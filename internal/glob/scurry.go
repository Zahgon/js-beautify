package glob

import (
	"io/fs"
	"os"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/nodefmt"
)

// This file ports the parts of path-scurry 2.0.2 (posix flavour) that glob's walk depends on:
// the tree of known path entries, with what has been learned about each of them, and the order
// in which a directory's children are kept, which decides the order of glob's results.

// Entry type bits, as path-scurry packs them.
const (
	typeUnknown   = 0
	typeFIFO      = 0b0001
	typeChar      = 0b0010
	typeDir       = 0b0100
	typeBlock     = 0b0110
	typeReg       = 0b1000
	typeLink      = 0b1010
	typeSocket    = 0b1100
	typeMask      = 0b1111
	readdirCalled = 0b0001_0000
	lstatCalled   = 0b0010_0000
	enotdir       = 0b0100_0000
	enoent        = 0b1000_0000
	enoreadlink   = 0b1_0000_0000
	enorealpath   = 0b10_0000_0000
	enochild      = enotdir | enoent | enorealpath
	typeBits      = 0b11_1111_1111
)

func modeToType(m fs.FileMode) int {
	switch {
	case m.IsRegular():
		return typeReg
	case m.IsDir():
		return typeDir
	case m&fs.ModeSymlink != 0:
		return typeLink
	case m&fs.ModeCharDevice != 0:
		return typeChar
	case m&fs.ModeDevice != 0:
		return typeBlock
	case m&fs.ModeSocket != 0:
		return typeSocket
	case m&fs.ModeNamedPipe != 0:
		return typeFIFO
	}
	return typeUnknown
}

// pathEntry is a path-scurry Path: one file system entry that may or may not exist.
type pathEntry struct {
	name     string
	typ      int
	parent   *pathEntry
	children []*pathEntry
	// provisional is the number of leading children that readdir confirmed; the others were
	// created by resolving names and may not exist.
	provisional int
	fullpath    string
	relative    *string
	depth       int
}

func newRoot() *pathEntry {
	return &pathEntry{name: "/", typ: typeDir}
}

func (p *pathEntry) newChild(name string, typ int, relative *string) *pathEntry {
	return &pathEntry{name: name, typ: typ & typeBits, parent: p, relative: relative, depth: p.depth + 1}
}

// resolve returns the entry for path, resolved from p.
func (p *pathEntry) resolve(path string) *pathEntry {
	if path == "" {
		return p
	}
	t := p
	if strings.HasPrefix(path, "/") {
		for t.parent != nil {
			t = t.parent
		}
		path = path[1:]
	}
	for _, part := range strings.Split(path, "/") {
		t = t.child(part, nil)
	}
	return t
}

// child returns the child named part, creating a provisional one if it is not known.
func (p *pathEntry) child(part string, relative *string) *pathEntry {
	switch part {
	case "", ".":
		return p
	case "..":
		if p.parent != nil {
			return p.parent
		}
		return p
	}
	for _, c := range p.children {
		if c.name == part {
			return c
		}
	}
	c := p.newChild(part, typeUnknown, relative)
	if !p.canReaddir() {
		c.typ |= enoent
	}
	p.children = append(p.children, c)
	return c
}

// relativePath returns the path from the walk's working directory.
func (p *pathEntry) relativePath() string {
	if p.relative != nil {
		return *p.relative
	}
	if p.parent == nil {
		return p.name
	}
	pv := p.parent.relativePath()
	if pv == "" || p.parent.parent == nil {
		return pv + p.name
	}
	return pv + "/" + p.name
}

func (p *pathEntry) fullPath() string {
	if p.fullpath != "" {
		return p.fullpath
	}
	if p.parent == nil {
		p.fullpath = p.name
	} else if p.parent.parent == nil {
		p.fullpath = p.parent.fullPath() + p.name
	} else {
		p.fullpath = p.parent.fullPath() + "/" + p.name
	}
	return p.fullpath
}

func (p *pathEntry) isUnknown() bool      { return p.typ&typeMask == typeUnknown }
func (p *pathEntry) isSymbolicLink() bool { return p.typ&typeLink == typeLink }
func (p *pathEntry) isENOENT() bool       { return p.typ&enoent != 0 }
func (p *pathEntry) calledReaddir() bool  { return p.typ&readdirCalled != 0 }

func (p *pathEntry) canReaddir() bool {
	if p.typ&enochild != 0 {
		return false
	}
	ifmt := p.typ & typeMask
	return ifmt == typeUnknown || ifmt == typeDir || ifmt == typeLink
}

func (p *pathEntry) markENOENT() {
	if p.typ&enoent != 0 {
		return
	}
	p.typ = (p.typ | enoent) &^ typeMask
	p.markChildrenENOENT()
}

func (p *pathEntry) markChildrenENOENT() {
	p.provisional = 0
	for _, c := range p.children {
		c.markENOENT()
	}
}

func (p *pathEntry) markENOTDIR() {
	if p.typ&enotdir != 0 {
		return
	}
	t := p.typ
	if t&typeMask == typeDir {
		t &^= typeMask
	}
	p.typ = t | enotdir
	p.markChildrenENOENT()
}

// lstat updates the entry from lstat(2), and returns nil when it does not exist.
func (p *pathEntry) lstat() *pathEntry {
	if p.typ&enoent != 0 {
		return nil
	}
	st, err := os.Lstat(p.fullPath())
	if err != nil {
		switch nodefmt.Code(err) {
		case "ENOTDIR":
			p.parent.markENOTDIR()
		case "ENOENT":
			p.markENOENT()
		}
		return nil
	}
	ifmt := modeToType(st.Mode())
	p.typ = (p.typ &^ typeMask) | ifmt | lstatCalled
	if ifmt != typeUnknown && ifmt != typeDir && ifmt != typeLink {
		p.typ |= enotdir
	}
	return p
}

// readdir returns the entry's children, reading the directory the first time. Like Node's
// fs.readdirSync, os.ReadDir lists the names in byte order; path-scurry puts each new name in
// front of the list.
func (p *pathEntry) readdir() []*pathEntry {
	if !p.canReaddir() {
		return nil
	}
	if p.calledReaddir() {
		return p.children[:p.provisional]
	}
	entries, err := os.ReadDir(p.fullPath())
	if err != nil {
		switch nodefmt.Code(err) {
		case "ENOTDIR", "EPERM":
			p.markENOTDIR()
		case "ENOENT":
			p.markENOENT()
		}
		p.provisional = 0
		return p.children[:0]
	}
	for _, e := range entries {
		p.addChild(e.Name(), modeToType(e.Type()))
	}
	p.typ |= readdirCalled
	for _, c := range p.children[p.provisional:] {
		c.markENOENT()
	}
	return p.children[:p.provisional]
}

func (p *pathEntry) addChild(name string, typ int) {
	for i := p.provisional; i < len(p.children); i++ {
		c := p.children[i]
		if c.name != name {
			continue
		}
		// Promote the provisional child: it exists.
		c.typ = (c.typ &^ typeMask) | typ
		if i != p.provisional {
			rest := append(p.children[:i:i], p.children[i+1:]...)
			p.children = append([]*pathEntry{c}, rest...)
		}
		p.provisional++
		return
	}
	c := p.newChild(name, typ, nil)
	if typ != typeDir && typ != typeLink && typ != typeUnknown {
		c.typ |= enotdir
	}
	p.children = append([]*pathEntry{c}, p.children...)
	p.provisional++
}

// newCwd builds the entries from the root down to the working directory cwd (an absolute
// path), giving each the relative path path-scurry gives it.
func newCwd(cwd string) *pathEntry {
	t := newRoot()
	var parts []string
	if trimmed := strings.TrimPrefix(cwd, "/"); trimmed != "" {
		parts = strings.Split(trimmed, "/")
	}
	for i, part := range parts {
		rel := strings.TrimSuffix(strings.Repeat("../", len(parts)-1-i), "/")
		t = t.child(part, &rel)
	}
	return t
}
