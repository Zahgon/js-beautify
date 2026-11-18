package minimatch

import (
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// This file ports minimatch's AST class, which parses one path portion of a glob (with extglobs
// such as +(a|b)) and turns it into a regular expression source.

// Pieces of regular expression minimatch puts together.
const (
	qmark            = "[^/]"
	star             = qmark + "*?"
	starNoEmpty      = qmark + "+?"
	startNoTraversal = `(?!(?:^|/)\.\.?(?:$|/))`
	startNoDot       = `(?!\.)`
)

// adoption tables: which extglob types a parent of the given type can absorb.
var (
	adoptionMap          = map[byte]string{'!': "@", '?': "?@", '@': "@", '*': "*+?@", '+': "+@"}
	adoptionWithSpaceMap = map[byte]string{'!': "?", '@': "?", '+': "?*"}
	adoptionAnyMap       = map[byte]string{'!': "?@", '?': "?@", '@': "?@", '*': "*+?@", '+': "+@?*"}
	usurpMap             = map[byte]map[byte]byte{
		'!': {'!': '@'},
		'?': {'*': '*', '+': '*'},
		'@': {'!': '!', '?': '?', '@': '@', '*': '*', '+': '+'},
		'+': {'?': '*', '*': '*'},
	}
)

func isExtglobType(c uint16) bool {
	return c == '!' || c == '?' || c == '+' || c == '*' || c == '@'
}

// astOptions are the minimatch options the AST reads.
type astOptions struct {
	dot, noext bool
}

// tristate is a boolean that may be undefined.
type tristate int8

const (
	unset tristate = iota
	isFalse
	isTrue
)

func tri(b bool) tristate {
	if b {
		return isTrue
	}
	return isFalse
}

// astPart is a part of an AST: a string, or a nested AST when node is set.
type astPart struct {
	s    jsrt.String
	node *ast
}

type ast struct {
	typ         byte // 0 for a plain sequence, else the extglob type
	root        *ast
	hasMagic    tristate
	uflag       bool
	parts       []astPart
	parent      *ast
	parentIndex int
	negs        *[]*ast
	filledNegs  bool
	options     astOptions
	str         *jsrt.String
	emptyExt    bool
}

func newAST(typ byte, parent *ast, options astOptions) *ast {
	a := &ast{typ: typ, parent: parent}
	if typ != 0 {
		a.hasMagic = isTrue
	}
	if parent != nil {
		a.root = parent.root
		a.options = a.root.options
		a.negs = a.root.negs
		a.parentIndex = len(parent.parts)
	} else {
		a.root = a
		a.options = options
		a.negs = &[]*ast{}
	}
	if typ == '!' && !a.root.filledNegs {
		*a.negs = append(*a.negs, a)
	}
	return a
}

// String reconstructs the pattern.
func (a *ast) String() jsrt.String {
	if a.str != nil {
		return *a.str
	}
	parts := make([]jsrt.String, len(a.parts))
	for i, p := range a.parts {
		if p.node != nil {
			parts[i] = p.node.String()
		} else {
			parts[i] = p.s
		}
	}
	var s jsrt.String
	if a.typ == 0 {
		s = jsrt.Join(parts, "")
	} else {
		s = jsrt.String(a.typ) + "(" + jsrt.Join(parts, "|") + ")"
	}
	a.str = &s
	return s
}

func (a *ast) fillNegs() {
	a.String()
	a.filledNegs = true
	negs := *a.negs
	for len(negs) > 0 {
		n := negs[len(negs)-1]
		negs = negs[:len(negs)-1]
		*a.negs = negs
		if n.typ != '!' {
			continue
		}
		// Walk up the tree, appending everything that comes after parentIndex.
		p, pp := n, n.parent
		for pp != nil {
			for i := p.parentIndex + 1; pp.typ == 0 && i < len(pp.parts); i++ {
				for _, part := range n.parts {
					part.node.copyIn(pp.parts[i])
				}
			}
			p, pp = pp, pp.parent
		}
	}
}

func (a *ast) pushString(s jsrt.String) {
	if s != "" {
		a.parts = append(a.parts, astPart{s: s})
	}
}

func (a *ast) pushNode(n *ast) {
	a.parts = append(a.parts, astPart{node: n})
}

func (a *ast) copyIn(p astPart) {
	if p.node == nil {
		a.pushString(p.s)
	} else {
		a.pushNode(p.node.clone(a))
	}
}

func (a *ast) clone(parent *ast) *ast {
	c := newAST(a.typ, parent, astOptions{})
	for _, p := range a.parts {
		c.copyIn(p)
	}
	return c
}

func (a *ast) isStart() bool {
	if a.root == a {
		return true
	}
	if !a.parent.isStart() {
		return false
	}
	if a.parentIndex == 0 {
		return true
	}
	// If everything ahead of this is a negation, then it's still the start.
	for i := 0; i < a.parentIndex; i++ {
		pp := a.parent.parts[i]
		if pp.node == nil || pp.node.typ != '!' {
			return false
		}
	}
	return true
}

func (a *ast) isEnd() bool {
	if a.root == a {
		return true
	}
	if a.parent.typ == '!' {
		return true
	}
	if !a.parent.isEnd() {
		return false
	}
	if a.typ == 0 {
		return a.parent.isEnd()
	}
	return a.parentIndex == len(a.parent.parts)-1
}

// parseAST parses str from pos into a, as minimatch's #parseAST does, and returns the position
// after what it consumed.
func parseAST(str []uint16, a *ast, pos int, extDepth int) int {
	const maxDepth = 2
	escaping, inBrace, braceNeg := false, false, false
	braceStart := -1
	if a.typ == 0 {
		// Outside of an extglob, append until a start is found.
		i := pos
		var acc []uint16
		for i < len(str) {
			c := str[i]
			i++
			if escaping || c == '\\' {
				escaping = !escaping
				acc = append(acc, c)
				continue
			}
			if inBrace {
				if i == braceStart+1 {
					if c == '^' || c == '!' {
						braceNeg = true
					}
				} else if c == ']' && !(i == braceStart+2 && braceNeg) {
					inBrace = false
				}
				acc = append(acc, c)
				continue
			} else if c == '[' {
				inBrace = true
				braceStart = i
				braceNeg = false
				acc = append(acc, c)
				continue
			}
			if !a.options.noext && isExtglobType(c) && i < len(str) && str[i] == '(' && extDepth <= maxDepth {
				a.pushString(jsrt.FromUnits(acc))
				acc = acc[:0]
				ext := newAST(byte(c), a, astOptions{})
				i = parseAST(str, ext, i, extDepth+1)
				a.pushNode(ext)
				continue
			}
			acc = append(acc, c)
		}
		a.pushString(jsrt.FromUnits(acc))
		return i
	}

	// Some kind of extglob: pos is at the "(". Find the next | or ).
	i := pos + 1
	part := newAST(0, a, astOptions{})
	var parts []*ast
	var acc []uint16
	for i < len(str) {
		c := str[i]
		i++
		if escaping || c == '\\' {
			escaping = !escaping
			acc = append(acc, c)
			continue
		}
		if inBrace {
			if i == braceStart+1 {
				if c == '^' || c == '!' {
					braceNeg = true
				}
			} else if c == ']' && !(i == braceStart+2 && braceNeg) {
				inBrace = false
			}
			acc = append(acc, c)
			continue
		} else if c == '[' {
			inBrace = true
			braceStart = i
			braceNeg = false
			acc = append(acc, c)
			continue
		}
		if !a.options.noext && isExtglobType(c) && i < len(str) && str[i] == '(' &&
			(extDepth <= maxDepth || a.canAdoptType(byte(c), adoptionAnyMap)) {
			depthAdd := 1
			if a.canAdoptType(byte(c), adoptionAnyMap) {
				depthAdd = 0
			}
			part.pushString(jsrt.FromUnits(acc))
			acc = acc[:0]
			ext := newAST(byte(c), part, astOptions{})
			part.pushNode(ext)
			i = parseAST(str, ext, i, extDepth+depthAdd)
			continue
		}
		if c == '|' {
			part.pushString(jsrt.FromUnits(acc))
			acc = acc[:0]
			parts = append(parts, part)
			part = newAST(0, a, astOptions{})
			continue
		}
		if c == ')' {
			if len(acc) == 0 && len(a.parts) == 0 {
				a.emptyExt = true
			}
			part.pushString(jsrt.FromUnits(acc))
			for _, p := range append(parts, part) {
				a.pushNode(p)
			}
			return i
		}
		acc = append(acc, c)
	}
	// An unfinished extglob is not an extglob, just the text it spans.
	a.typ = 0
	a.hasMagic = unset
	a.parts = []astPart{{s: jsrt.FromUnits(str[pos-1:])}}
	return i
}

func (a *ast) canAdoptType(c byte, m map[byte]string) bool {
	allowed, ok := m[a.typ]
	return ok && strings.IndexByte(allowed, c) >= 0
}

// onlyExtglobChild returns the extglob that is the single part of child, the single-part
// sequence it must be.
func onlyExtglobChild(child astPart) *ast {
	if child.node == nil || child.node.typ != 0 || len(child.node.parts) != 1 {
		return nil
	}
	gc := child.node.parts[0].node
	if gc == nil || gc.typ == 0 {
		return nil
	}
	return gc
}

func (a *ast) canAdopt(child astPart, m map[byte]string) bool {
	if a.typ == 0 {
		return false
	}
	gc := onlyExtglobChild(child)
	return gc != nil && a.canAdoptType(gc.typ, m)
}

func (a *ast) adoptWithSpace(child *ast, index int) {
	gc := child.parts[0].node
	blank := newAST(0, gc, astOptions{})
	blank.parts = append(blank.parts, astPart{s: ""})
	gc.pushNode(blank)
	a.adopt(child, index)
}

func (a *ast) adopt(child *ast, index int) {
	gc := child.parts[0].node
	parts := make([]astPart, 0, len(a.parts)+len(gc.parts))
	parts = append(parts, a.parts[:index]...)
	parts = append(parts, gc.parts...)
	parts = append(parts, a.parts[index+1:]...)
	a.parts = parts
	for _, p := range gc.parts {
		if p.node != nil {
			p.node.parent = a
		}
	}
	a.str = nil
}

func (a *ast) canUsurp(child astPart) bool {
	if a.typ == 0 || len(a.parts) != 1 {
		return false
	}
	gc := onlyExtglobChild(child)
	if gc == nil {
		return false
	}
	_, ok := usurpMap[a.typ][gc.typ]
	return ok
}

func (a *ast) usurp(child *ast) {
	gc := child.parts[0].node
	nt := usurpMap[a.typ][gc.typ]
	a.parts = gc.parts
	for _, p := range a.parts {
		if p.node != nil {
			p.node.parent = a
		}
	}
	a.typ = nt
	a.str = nil
	a.emptyExt = false
}

func (a *ast) flatten() {
	if a.typ == 0 {
		for _, p := range a.parts {
			if p.node != nil {
				p.node.flatten()
			}
		}
	} else {
		// Up to 10 passes to flatten as much as possible.
		for iterations := 0; ; {
			done := true
			for i := 0; i < len(a.parts); i++ {
				c := a.parts[i]
				if c.node == nil {
					continue
				}
				c.node.flatten()
				switch {
				case a.canAdopt(c, adoptionMap):
					done = false
					a.adopt(c.node, i)
				case a.canAdopt(c, adoptionWithSpaceMap):
					done = false
					a.adoptWithSpace(c.node, i)
				case a.canUsurp(c):
					done = false
					a.usurp(c.node)
				}
			}
			iterations++
			if done || iterations >= 10 {
				break
			}
		}
	}
	a.str = nil
}

// regexpSource is what toRegExpSource returns.
type regexpSource struct {
	re       jsrt.String
	body     jsrt.String
	hasMagic bool
	uflag    bool
}

func (a *ast) toRegExpSource(allowDot tristate) regexpSource {
	dot := allowDot == isTrue || (allowDot == unset && a.options.dot)
	if a.root == a {
		a.flatten()
		a.fillNegs()
	}
	if a.typ == 0 {
		noEmpty := a.isStart() && a.isEnd()
		for _, p := range a.parts {
			if p.node != nil {
				noEmpty = false
			}
		}
		var srcParts []jsrt.String
		for _, p := range a.parts {
			var r regexpSource
			if p.node == nil {
				r = parseGlob(p.s, a.hasMagic == isTrue, noEmpty)
			} else {
				r = p.node.toRegExpSource(allowDot)
			}
			if a.hasMagic != isTrue && r.hasMagic {
				a.hasMagic = isTrue
			}
			a.uflag = a.uflag || r.uflag
			srcParts = append(srcParts, r.re)
		}
		src := jsrt.Join(srcParts, "")
		start := jsrt.String("")
		if a.isStart() && len(a.parts) > 0 && a.parts[0].node == nil {
			// This string matches the start of the pattern: "." and ".." cannot match unless
			// the pattern is exactly that, even if it starts with "." or dot is set.
			first := a.parts[0].s
			dotTravAllowed := len(a.parts) == 1 && (first == "." || first == "..")
			if !dotTravAllowed {
				aps := func(i int) bool { c := src.CharAt(i); return c == "[" || c == "." }
				needNoTrav := (dot && aps(0)) ||
					(src.StartsWith(`\.`) && aps(2)) ||
					(src.StartsWith(`\.\.`) && aps(4))
				needNoDot := !dot && allowDot != isTrue && aps(0)
				switch {
				case needNoTrav:
					start = startNoTraversal
				case needNoDot:
					start = startNoDot
				}
			}
		}
		end := jsrt.String("")
		if a.isEnd() && a.root.filledNegs && a.parent != nil && a.parent.typ == '!' {
			end = `(?:$|\/)`
		}
		if a.hasMagic != isTrue {
			a.hasMagic = isFalse
		}
		return regexpSource{start + src + end, unescape(src), a.hasMagic == isTrue, a.uflag}
	}

	// The body is computed twice for a repeated pattern at the start, once in no-dot mode and
	// again in dot mode, so that a pattern like *(?) can match "x.y".
	repeated := a.typ == '*' || a.typ == '+'
	start := jsrt.String("(?:")
	if a.typ == '!' {
		start = "(?:(?!(?:"
	}
	body := a.partsToRegExp(dot)
	if a.isStart() && a.isEnd() && body == "" && a.typ != '!' {
		// An invalid extglob: it has to match something when it is the whole path portion.
		s := a.String()
		a.parts = []astPart{{s: s}}
		a.typ = 0
		a.hasMagic = unset
		return regexpSource{s, unescape(a.String()), false, false}
	}
	bodyDotAllowed := jsrt.String("")
	if repeated && allowDot != isTrue && !dot {
		bodyDotAllowed = a.partsToRegExp(true)
	}
	if bodyDotAllowed == body {
		bodyDotAllowed = ""
	}
	if bodyDotAllowed != "" {
		body = "(?:" + body + ")(?:" + bodyDotAllowed + ")*?"
	}
	var final jsrt.String
	if a.typ == '!' && a.emptyExt {
		if a.isStart() && !dot {
			final = startNoDot
		}
		final += starNoEmpty
	} else {
		var closing jsrt.String
		switch {
		case a.typ == '!':
			closing = "))"
			if a.isStart() && !dot && allowDot != isTrue {
				closing += startNoDot
			}
			closing += star + ")"
		case a.typ == '@':
			closing = ")"
		case a.typ == '?':
			closing = ")?"
		case a.typ == '+' && bodyDotAllowed != "":
			closing = ")"
		case a.typ == '*' && bodyDotAllowed != "":
			closing = ")?"
		default:
			closing = ")" + jsrt.String(a.typ)
		}
		final = start + body + closing
	}
	a.hasMagic = isTrue
	return regexpSource{final, unescape(body), true, a.uflag}
}

func (a *ast) partsToRegExp(dot bool) jsrt.String {
	var out []jsrt.String
	whole := a.isStart() && a.isEnd()
	for _, p := range a.parts {
		r := p.node.toRegExpSource(tri(dot))
		a.uflag = a.uflag || r.uflag
		if !whole || r.re != "" {
			out = append(out, r.re)
		}
	}
	return jsrt.Join(out, "|")
}

// parseGlob translates the glob text of a sequence into a regular expression source.
func parseGlob(glob jsrt.String, hasMagic, noEmpty bool) regexpSource {
	units := glob.Units()
	escaping, uflag, inStar := false, false, false
	var re strings.Builder
	allStars := len(units) > 0
	for _, u := range units {
		if u != '*' {
			allStars = false
		}
	}
	for i := 0; i < len(units); i++ {
		c := units[i]
		if escaping {
			escaping = false
			if strings.ContainsRune(`().*{}+?[]^$\!`, rune(c)) {
				re.WriteByte('\\')
			}
			re.WriteString(string(jsrt.FromUnit(c)))
			continue
		}
		if c == '*' {
			if inStar {
				continue
			}
			inStar = true
			if noEmpty && allStars {
				re.WriteString(starNoEmpty)
			} else {
				re.WriteString(star)
			}
			hasMagic = true
			continue
		}
		inStar = false
		if c == '\\' {
			if i == len(units)-1 {
				re.WriteString(`\\`)
			} else {
				escaping = true
			}
			continue
		}
		if c == '[' {
			src, needUflag, consumed, magic := parseClass(units, i)
			if consumed > 0 {
				re.WriteString(string(src))
				uflag = uflag || needUflag
				i += consumed - 1
				hasMagic = hasMagic || magic
				continue
			}
		}
		if c == '?' {
			re.WriteString(qmark)
			hasMagic = true
			continue
		}
		re.WriteString(string(regexpEscape(jsrt.FromUnit(c))))
	}
	return regexpSource{jsrt.String(re.String()), unescape(glob), hasMagic, uflag}
}

// fromGlob parses a path portion.
func fromGlob(pattern jsrt.String, options astOptions) *ast {
	a := newAST(0, nil, options)
	parseAST(pattern.Units(), a, 0, 0)
	return a
}
