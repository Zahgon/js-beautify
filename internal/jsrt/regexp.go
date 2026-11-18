package jsrt

import (
	"errors"
	"strings"
)

// Regexp is a compiled JavaScript regular expression. It is immutable and safe for concurrent
// use: where JavaScript keeps lastIndex on the RegExp object, the methods here take it as an
// argument and the caller keeps it.
type Regexp struct {
	source     String
	flags      string
	global     bool
	sticky     bool
	ignoreCase bool
	root       *node
	ncap       int
	names      map[String]int
	prefix     int // first code unit of every match, or -1 when unknown
}

func (e syntaxError) Error() string { return string(e) }

// flagOrder is the order in which V8 writes flags (RegExp.prototype.flags).
const flagOrder = "dgimsuvy"

// Compile parses a pattern as new RegExp(source, flags) does. Supported flags are d, g, i, m, s
// and y; the u and v flags, which js-beautify never uses, are rejected. Errors carry V8's
// message, e.g. "Invalid regular expression: /(/g: Unterminated group".
func Compile(source String, flags string) (*Regexp, error) {
	re := &Regexp{source: source, prefix: -1}
	var f regexpFlags
	seen := map[rune]bool{}
	for _, c := range flags {
		if seen[c] || !strings.ContainsRune(flagOrder, c) || c == 'u' || c == 'v' {
			return nil, errors.New("Invalid flags supplied to RegExp constructor '" + flags + "'")
		}
		seen[c] = true
		switch c {
		case 'g':
			re.global = true
		case 'y':
			re.sticky = true
		case 'i':
			f.icase = true
		case 'm':
			f.multiline = true
		case 's':
			f.dotAll = true
		}
	}
	for _, c := range flagOrder {
		if seen[c] {
			re.flags += string(c)
		}
	}
	re.ignoreCase = f.icase
	root, ncap, names, err := parsePattern(source.Units(), f)
	if err != nil {
		return nil, errors.New("Invalid regular expression: /" + re.Source().UTF8() + "/" + re.flags + ": " + err.Error())
	}
	re.root, re.ncap, re.names = root, ncap, names
	if c, ok := firstUnit(root); ok {
		re.prefix = int(c)
	}
	return re, nil
}

// MustCompile is Compile for patterns written in the program; it panics on an invalid pattern.
func MustCompile(source String, flags string) *Regexp {
	re, err := Compile(source, flags)
	if err != nil {
		panic(err)
	}
	return re
}

// firstUnit returns the code unit every match must start with, when the pattern says so plainly.
func firstUnit(n *node) (uint16, bool) {
	switch n.kind {
	case nLiteral:
		if !n.icase {
			return n.lit[0], true
		}
	case nSeq:
		return firstUnit(n.subs[0])
	case nGroup:
		return firstUnit(n.sub)
	case nRepeat:
		if n.min > 0 {
			return firstUnit(n.sub)
		}
	}
	return 0, false
}

// Source returns the pattern text (RegExp.prototype.source, "(?:)" for an empty pattern), for
// building new patterns from existing ones.
func (re *Regexp) Source() String {
	if re.source == "" {
		return "(?:)"
	}
	return re.source
}

// Flags returns the flags in canonical order (RegExp.prototype.flags).
func (re *Regexp) Flags() string { return re.flags }

// Global reports the g flag.
func (re *Regexp) Global() bool { return re.global }

// Sticky reports the y flag.
func (re *Regexp) Sticky() bool { return re.sticky }

// WithFlags returns new RegExp(re, flags): the same pattern with other flags.
func (re *Regexp) WithFlags(flags string) (*Regexp, error) {
	return Compile(re.source, flags)
}

// String returns the pattern as a regex literal, /source/flags.
func (re *Regexp) String() string {
	return "/" + re.Source().UTF8() + "/" + re.flags
}

// Match is the result of a successful match (the array RegExp.prototype.exec returns).
type Match struct {
	input []uint16
	caps  []int
	names map[String]int
}

// Index returns the code unit index where the match starts.
func (m *Match) Index() int { return m.caps[0] }

// End returns the code unit index just after the match (the next lastIndex of a global regex).
func (m *Match) End() int { return m.caps[1] }

// Text returns the matched text, m[0].
func (m *Match) Text() String { return FromUnits(m.input[m.caps[0]:m.caps[1]]) }

// NumGroups returns the number of capturing groups of the pattern.
func (m *Match) NumGroups() int { return len(m.caps)/2 - 1 }

// Group returns m[i]; ok is false when the group did not participate (undefined in JavaScript).
func (m *Match) Group(i int) (s String, ok bool) {
	if i < 0 || 2*i+1 >= len(m.caps) || m.caps[2*i] < 0 {
		return "", false
	}
	return FromUnits(m.input[m.caps[2*i]:m.caps[2*i+1]]), true
}

// GroupText returns m[i], or "" when the group did not participate.
func (m *Match) GroupText(i int) String {
	s, _ := m.Group(i)
	return s
}

// NamedGroup returns m.groups[name].
func (m *Match) NamedGroup(name String) (String, bool) {
	i, ok := m.names[name]
	if !ok {
		return "", false
	}
	return m.Group(i)
}

func (re *Regexp) result(in []uint16, caps []int) *Match {
	if caps == nil {
		return nil
	}
	return &Match{input: in, caps: caps, names: re.names}
}

// ExecUnits is RegExp.prototype.exec on code units with the given lastIndex: a global or sticky
// regex starts at lastIndex (a sticky one only tries there) and fails when lastIndex is past
// the end; any other regex searches from 0. The caller updates lastIndex from Match.End.
func (re *Regexp) ExecUnits(in []uint16, lastIndex int) *Match {
	if !re.global && !re.sticky {
		lastIndex = 0
	}
	if lastIndex < 0 {
		lastIndex = 0
	}
	if lastIndex > len(in) {
		return nil
	}
	if re.sticky {
		return re.result(in, re.matchAt(in, lastIndex))
	}
	return re.result(in, re.search(in, lastIndex))
}

// MatchAt reports the match that starts exactly at index i, whatever the flags (the behaviour of
// a sticky regex with lastIndex i), or nil.
func (re *Regexp) MatchAt(in []uint16, i int) *Match {
	if i < 0 || i > len(in) {
		return nil
	}
	return re.result(in, re.matchAt(in, i))
}

// SearchFrom returns the leftmost match starting at or after index from, whatever the flags.
func (re *Regexp) SearchFrom(in []uint16, from int) *Match {
	if from < 0 {
		from = 0
	}
	if from > len(in) {
		return nil
	}
	return re.result(in, re.search(in, from))
}

// Exec returns re.exec(s) for a regex whose lastIndex is 0, which is also s.match(re) for a regex
// without the g flag.
func (re *Regexp) Exec(s String) *Match {
	return re.ExecUnits(s.Units(), 0)
}

// Test returns re.test(s) for a regex whose lastIndex is 0.
func (re *Regexp) Test(s String) bool {
	return re.Exec(s) != nil
}

// Search returns s.search(re): the index of the first match from the start, or -1.
func (re *Regexp) Search(s String) int {
	if m := re.SearchFrom(s.Units(), 0); m != nil {
		return m.Index()
	}
	return -1
}

// FindAll returns s.match(re) for a regex with the g flag: the text of every match, or nil when
// there is none. An empty match advances the search by one code unit.
func (re *Regexp) FindAll(s String) []String {
	in := s.Units()
	var out []String
	for last := 0; ; {
		m := re.ExecUnits(in, last)
		if m == nil {
			break
		}
		out = append(out, m.Text())
		last = m.End()
		if m.End() == m.Index() {
			last++
		}
		if !re.global {
			break
		}
	}
	return out
}

// Replace returns s.replace(re, replacement): every match when re is global, the first one
// otherwise, replaced by replacement with $$, $&, $`, $', $n, $nn and $<name> expanded.
func (re *Regexp) Replace(s String, replacement String) String {
	return re.replace(s, func(in []uint16, caps []int) String {
		return expandReplacement(replacement, in, caps, re.names)
	})
}

// ReplaceFunc returns s.replace(re, fn): fn computes the replacement of each match.
func (re *Regexp) ReplaceFunc(s String, fn func(*Match) String) String {
	return re.replace(s, func(in []uint16, caps []int) String {
		return fn(re.result(in, caps))
	})
}

func (re *Regexp) replace(s String, repl func([]uint16, []int) String) String {
	in := s.Units()
	var b strings.Builder
	next, last, matched := 0, 0, false
	for {
		m := re.ExecUnits(in, last)
		if m == nil {
			break
		}
		matched = true
		b.WriteString(string(FromUnits(in[next:m.Index()])))
		b.WriteString(string(repl(in, m.caps)))
		next = m.End()
		if !re.global {
			break
		}
		last = m.End()
		if m.End() == m.Index() {
			last++
		}
	}
	if !matched {
		return s
	}
	b.WriteString(string(FromUnits(in[next:])))
	return String(b.String())
}

// Split returns s.split(re): the pieces between matches, with the text of capturing groups
// spliced in ("" for a group that did not participate, where JavaScript inserts undefined).
func (re *Regexp) Split(s String) []String {
	in := s.Units()
	if len(in) == 0 {
		if re.matchAt(in, 0) != nil {
			return []String{}
		}
		return []String{s}
	}
	var out []String
	p := 0
	for q := p; q < len(in); {
		caps := re.matchAt(in, q)
		if caps == nil {
			q++
			continue
		}
		e := caps[1]
		if e > len(in) {
			e = len(in)
		}
		if e == p {
			q++
			continue
		}
		out = append(out, FromUnits(in[p:q]))
		m := re.result(in, caps)
		for g := 1; g <= m.NumGroups(); g++ {
			out = append(out, m.GroupText(g))
		}
		p = e
		q = p
	}
	return append(out, FromUnits(in[p:]))
}

// expandReplacement is GetSubstitution: it expands the $ patterns of a replacement string for
// the match whose group bounds are caps (caps[0], caps[1] being the match) within in.
func expandReplacement(repl String, in []uint16, caps []int, names map[String]int) String {
	if !strings.Contains(string(repl), "$") {
		return repl
	}
	r := repl.Units()
	ncap := len(caps)/2 - 1
	group := func(i int) []uint16 {
		if caps[2*i] < 0 {
			return nil
		}
		return in[caps[2*i]:caps[2*i+1]]
	}
	var out []uint16
	for i := 0; i < len(r); i++ {
		if r[i] != '$' || i+1 >= len(r) {
			out = append(out, r[i])
			continue
		}
		c := r[i+1]
		switch {
		case c == '$':
			out = append(out, '$')
			i++
		case c == '&':
			out = append(out, in[caps[0]:caps[1]]...)
			i++
		case c == '`':
			out = append(out, in[:caps[0]]...)
			i++
		case c == '\'':
			out = append(out, in[caps[1]:]...)
			i++
		case IsDigit(c):
			d := int(c - '0')
			if i+2 < len(r) && IsDigit(r[i+2]) {
				if dd := d*10 + int(r[i+2]-'0'); dd >= 1 && dd <= ncap {
					out = append(out, group(dd)...)
					i += 2
					continue
				}
			}
			if d >= 1 && d <= ncap {
				out = append(out, group(d)...)
				i++
				continue
			}
			out = append(out, '$')
		case c == '<' && names != nil:
			end := -1
			for j := i + 2; j < len(r); j++ {
				if r[j] == '>' {
					end = j
					break
				}
			}
			if end < 0 {
				out = append(out, '$')
				continue
			}
			if g, ok := names[FromUnits(r[i+2:end])]; ok {
				out = append(out, group(g)...)
			}
			i = end
		default:
			out = append(out, '$')
		}
	}
	return FromUnits(out)
}
