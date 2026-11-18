package jsrt

import (
	"sort"
	"unicode"
)

// The parser follows the ECMAScript pattern grammar without the u flag, including the Annex B
// extensions every JavaScript engine implements: a lone ] { or } is a literal, \c not followed by
// a letter is a backslash, unmatched decimal escapes are legacy octal escapes, identity escapes
// accept any character, and lookaheads may be quantified. Errors carry V8's messages.

type nodeKind uint8

const (
	nEmpty        nodeKind = iota
	nLiteral               // the code units in lit
	nSet                   // one code unit in set
	nSeq                   // subs in order
	nAlt                   // the first of subs that leads to a match
	nGroup                 // capturing group number n around sub
	nLook                  // lookahead (or lookbehind when behind) around sub
	nBackref               // the text captured by group n
	nLineStart             // ^
	nLineEnd               // $
	nWordBoundary          // \b, or \B when negate
	nRepeat                // sub repeated min..max times (max < 0: unbounded)
)

type node struct {
	kind   nodeKind
	lit    []uint16
	set    *charSet
	subs   []*node
	sub    *node
	n      int
	min    int
	max    int
	greedy bool
	negate bool
	behind bool
	icase  bool // case-insensitive comparison (nLiteral, nSet, nBackref)
	multi  bool // multiline (nLineStart, nLineEnd)
	// capLo and capHi delimit the capture groups inside a repeated or looked-around sub: the
	// groups numbered capLo+1 .. capHi.
	capLo, capHi int
	// name is the group name of a \k<name> reference until it is resolved.
	name String
	// single is set on an nRepeat whose sub-pattern matches exactly one code unit and captures
	// nothing; it tests that code unit.
	single func(uint16) bool
}

// charSet is a set of code units: sorted, non-overlapping ranges, optionally complemented.
type charSet struct {
	ranges []unitRange
	negate bool
}

type unitRange struct{ lo, hi uint16 }

// inRanges reports whether u lies in one of the ranges, ignoring negate.
func (s *charSet) inRanges(u uint16) bool {
	i := sort.Search(len(s.ranges), func(i int) bool { return s.ranges[i].hi >= u })
	return i < len(s.ranges) && s.ranges[i].lo <= u
}

// matches reports whether u matches the set. With icase, a unit matches when some member of
// the ranges has the same canonical form; a negated set then rejects it.
func (s *charSet) matches(u uint16, icase bool) bool {
	found := s.inRanges(u)
	if !found && icase {
		cu := canonicalize(u)
		found = caseVariants(u, func(v uint16) bool {
			return canonicalize(v) == cu && s.inRanges(v)
		})
	}
	return found != s.negate
}

func normalizeRanges(rs []unitRange) []unitRange {
	sort.Slice(rs, func(i, j int) bool { return rs[i].lo < rs[j].lo })
	out := rs[:0]
	for _, r := range rs {
		if n := len(out); n > 0 && uint32(r.lo) <= uint32(out[n-1].hi)+1 {
			if r.hi > out[n-1].hi {
				out[n-1].hi = r.hi
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func complementRanges(rs []unitRange) []unitRange {
	var out []unitRange
	next := uint32(0)
	for _, r := range rs {
		if uint32(r.lo) > next {
			out = append(out, unitRange{uint16(next), r.lo - 1})
		}
		next = uint32(r.hi) + 1
	}
	if next <= 0xFFFF {
		out = append(out, unitRange{uint16(next), 0xFFFF})
	}
	return out
}

var (
	digitRanges      = []unitRange{{'0', '9'}}
	wordRanges       = []unitRange{{'0', '9'}, {'A', 'Z'}, {'_', '_'}, {'a', 'z'}}
	whitespaceRanges = []unitRange{{0x09, 0x0D}, {0x20, 0x20}, {0xA0, 0xA0}, {0x1680, 0x1680},
		{0x2000, 0x200A}, {0x2028, 0x2029}, {0x202F, 0x202F}, {0x205F, 0x205F}, {0x3000, 0x3000},
		{0xFEFF, 0xFEFF}}
	lineTerminatorRanges = []unitRange{{'\n', '\n'}, {'\r', '\r'}, {0x2028, 0x2029}}
)

// classEscapeRanges returns the ranges of \d \D \s \S \w \W.
func classEscapeRanges(c uint16) ([]unitRange, bool) {
	var rs []unitRange
	switch c | 0x20 {
	case 'd':
		rs = digitRanges
	case 's':
		rs = whitespaceRanges
	case 'w':
		rs = wordRanges
	default:
		return nil, false
	}
	if c < 'a' {
		return complementRanges(rs), true
	}
	return append([]unitRange(nil), rs...), true
}

type regexpFlags struct {
	icase, multiline, dotAll bool
}

type parser struct {
	src       []uint16
	pos       int
	flags     regexpFlags
	ncap      int // capture groups opened so far
	totalCaps int // capture groups in the whole pattern
	hasNames  bool
	names     map[String]int
	backrefs  []*node
}

// syntaxError is a pattern error; its message is completed with the source and flags.
type syntaxError string

func parsePattern(src []uint16, flags regexpFlags) (*node, int, map[String]int, error) {
	p := &parser{src: src, flags: flags}
	p.totalCaps, p.hasNames = countGroups(src)
	root, err := p.parseDisjunction()
	if err != nil {
		return nil, 0, nil, err
	}
	if p.pos < len(p.src) {
		// parseDisjunction only stops early at a ')'.
		return nil, 0, nil, syntaxError("Unmatched ')'")
	}
	for _, b := range p.backrefs {
		n, ok := p.names[b.name]
		if !ok {
			return nil, 0, nil, syntaxError("Invalid named capture referenced")
		}
		b.n = n
	}
	return root, p.ncap, p.names, nil
}

// countGroups counts the capturing groups of a pattern and reports whether any is named.
func countGroups(src []uint16) (int, bool) {
	n, named, inClass := 0, false, false
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if inClass {
				continue
			}
			if i+1 < len(src) && src[i+1] == '?' {
				if i+3 < len(src) && src[i+2] == '<' && src[i+3] != '=' && src[i+3] != '!' {
					n++
					named = true
				}
				continue
			}
			n++
		}
	}
	return n, named
}

func (p *parser) more() bool { return p.pos < len(p.src) }

func (p *parser) peekIs(c uint16) bool { return p.pos < len(p.src) && p.src[p.pos] == c }

func (p *parser) peekAt(off int) (uint16, bool) {
	if p.pos+off < len(p.src) {
		return p.src[p.pos+off], true
	}
	return 0, false
}

func (p *parser) parseDisjunction() (*node, error) {
	var alts []*node
	for {
		alt, err := p.parseAlternative()
		if err != nil {
			return nil, err
		}
		alts = append(alts, alt)
		if !p.peekIs('|') {
			break
		}
		p.pos++
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &node{kind: nAlt, subs: alts}, nil
}

func (p *parser) parseAlternative() (*node, error) {
	var terms []*node
	for p.more() && !p.peekIs('|') && !p.peekIs(')') {
		t, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		// Merge adjacent literals with the same case sensitivity.
		if n := len(terms); n > 0 && t.kind == nLiteral && terms[n-1].kind == nLiteral && terms[n-1].icase == t.icase {
			terms[n-1].lit = append(terms[n-1].lit, t.lit...)
			continue
		}
		terms = append(terms, t)
	}
	switch len(terms) {
	case 0:
		return &node{kind: nEmpty}, nil
	case 1:
		return terms[0], nil
	}
	return &node{kind: nSeq, subs: terms}, nil
}

func (p *parser) parseTerm() (*node, error) {
	c := p.src[p.pos]
	capBefore := p.ncap
	var atom *node
	quantifiable := true
	switch c {
	case '^':
		p.pos++
		atom, quantifiable = &node{kind: nLineStart, multi: p.flags.multiline}, false
	case '$':
		p.pos++
		atom, quantifiable = &node{kind: nLineEnd, multi: p.flags.multiline}, false
	case '\\':
		if next, ok := p.peekAt(1); ok && (next == 'b' || next == 'B') {
			p.pos += 2
			atom, quantifiable = &node{kind: nWordBoundary, negate: next == 'B'}, false
			break
		}
		a, err := p.parseAtomEscape()
		if err != nil {
			return nil, err
		}
		atom = a
	case '(':
		a, behind, err := p.parseGroup()
		if err != nil {
			return nil, err
		}
		atom = a
		if behind {
			quantifiable = false
		}
	case '.':
		p.pos++
		rs := lineTerminatorRanges
		if p.flags.dotAll {
			rs = nil
		}
		atom = &node{kind: nSet, set: &charSet{ranges: rs, negate: true}}
	case '[':
		a, err := p.parseClass()
		if err != nil {
			return nil, err
		}
		atom = a
	case '*', '+', '?':
		return nil, syntaxError("Nothing to repeat")
	case '{':
		if _, _, _, ok := p.braceQuantifier(); ok {
			return nil, syntaxError("Nothing to repeat")
		}
		p.pos++
		atom = p.literal(c)
	default:
		p.pos++
		atom = p.literal(c)
	}
	min, max, ok, err := p.parseQuantifier()
	if err != nil {
		return nil, err
	}
	if !ok {
		return atom, nil
	}
	if !quantifiable {
		if atom.kind == nLook {
			return nil, syntaxError("Invalid quantifier")
		}
		return nil, syntaxError("Nothing to repeat")
	}
	greedy := true
	if p.peekIs('?') {
		p.pos++
		greedy = false
	}
	return &node{kind: nRepeat, sub: atom, min: min, max: max, greedy: greedy, capLo: capBefore, capHi: p.ncap, single: singleUnit(atom)}, nil
}

func (p *parser) literal(c uint16) *node {
	return &node{kind: nLiteral, lit: []uint16{c}, icase: p.flags.icase}
}

// parseQuantifier reads a quantifier after an atom, if there is one.
func (p *parser) parseQuantifier() (min, max int, ok bool, err error) {
	if !p.more() {
		return 0, 0, false, nil
	}
	switch p.src[p.pos] {
	case '*':
		p.pos++
		return 0, -1, true, nil
	case '+':
		p.pos++
		return 1, -1, true, nil
	case '?':
		p.pos++
		return 0, 1, true, nil
	case '{':
		min, max, end, ok := p.braceQuantifier()
		if !ok {
			return 0, 0, false, nil
		}
		if max >= 0 && min > max {
			return 0, 0, false, syntaxError("numbers out of order in {} quantifier")
		}
		p.pos = end
		return min, max, true, nil
	}
	return 0, 0, false, nil
}

// braceQuantifier recognises {n}, {n,} or {n,m} at the current position.
func (p *parser) braceQuantifier() (min, max, end int, ok bool) {
	i := p.pos + 1
	readInt := func() (int, bool) {
		start, v := i, 0
		for i < len(p.src) && IsDigit(p.src[i]) {
			if v < 1<<30 {
				v = v*10 + int(p.src[i]-'0')
			}
			i++
		}
		return v, i > start
	}
	min, ok = readInt()
	if !ok {
		return 0, 0, 0, false
	}
	max = min
	if i < len(p.src) && p.src[i] == ',' {
		i++
		if m, has := readInt(); has {
			max = m
		} else {
			max = -1
		}
	}
	if i >= len(p.src) || p.src[i] != '}' {
		return 0, 0, 0, false
	}
	return min, max, i + 1, true
}

// parseGroup parses a parenthesised group; behind reports a lookbehind.
func (p *parser) parseGroup() (n *node, behind bool, err error) {
	p.pos++ // (
	kind, capture, negate := nGroup, false, false
	var name String
	modifiers := p.flags
	if p.peekIs('?') {
		next, _ := p.peekAt(1)
		switch next {
		case ':':
			p.pos += 2
			kind = nSeq
		case '=', '!':
			p.pos += 2
			kind, negate = nLook, next == '!'
		case '<':
			after, _ := p.peekAt(2)
			if after == '=' || after == '!' {
				p.pos += 3
				kind, negate, behind = nLook, after == '!', true
				break
			}
			p.pos += 2
			nm, err := p.parseGroupName()
			if err != nil {
				return nil, false, err
			}
			name, capture = nm, true
		default:
			f, err := p.parseModifiers()
			if err != nil {
				return nil, false, err
			}
			modifiers, kind = f, nSeq
		}
	} else {
		capture = true
	}
	index := 0
	if capture {
		p.ncap++
		index = p.ncap
		if name != "" {
			if p.names == nil {
				p.names = map[String]int{}
			}
			if _, dup := p.names[name]; dup {
				return nil, false, syntaxError("Duplicate capture group name")
			}
			p.names[name] = index
		}
	}
	capBefore := p.ncap
	saved := p.flags
	p.flags = modifiers
	sub, err := p.parseDisjunction()
	p.flags = saved
	if err != nil {
		return nil, false, err
	}
	if !p.peekIs(')') {
		return nil, false, syntaxError("Unterminated group")
	}
	p.pos++
	switch kind {
	case nGroup:
		return &node{kind: nGroup, n: index, sub: sub}, false, nil
	case nLook:
		return &node{kind: nLook, sub: sub, negate: negate, behind: behind, capLo: capBefore, capHi: p.ncap}, behind, nil
	}
	return sub, false, nil
}

// parseModifiers parses the (?ims-ims: prefix of a modifiers group, after "(?".
func (p *parser) parseModifiers() (regexpFlags, error) {
	f := p.flags
	i := p.pos + 1
	seen := map[uint16]bool{}
	removing, hasFlag := false, false
	for ; i < len(p.src); i++ {
		c := p.src[i]
		switch c {
		case 'i', 'm', 's':
			if seen[c] {
				return f, syntaxError("Repeated flag in flag group")
			}
			seen[c] = true
			hasFlag = true
			switch c {
			case 'i':
				f.icase = !removing
			case 'm':
				f.multiline = !removing
			case 's':
				f.dotAll = !removing
			}
			continue
		case '-':
			if removing {
				return f, syntaxError("Invalid group")
			}
			removing = true
			continue
		case ':':
			if !hasFlag {
				return f, syntaxError("Invalid group")
			}
			p.pos = i + 1
			return f, nil
		}
		break
	}
	return f, syntaxError("Invalid group")
}

// parseGroupName reads "name>" after "(?<" or "\k<".
func (p *parser) parseGroupName() (String, error) {
	start := p.pos
	for p.more() && p.src[p.pos] != '>' {
		c := rune(p.src[p.pos])
		first := p.pos == start
		ok := c == '$' || c == '_' || unicode.IsLetter(c) || (!first && (unicode.IsDigit(c) || unicode.In(c, unicode.Mn, unicode.Mc, unicode.Pc)))
		if !ok {
			return "", syntaxError("Invalid capture group name")
		}
		p.pos++
	}
	if !p.more() || p.pos == start {
		return "", syntaxError("Invalid capture group name")
	}
	name := FromUnits(p.src[start:p.pos])
	p.pos++ // >
	return name, nil
}

// parseAtomEscape parses an escape outside a character class (not \b or \B).
func (p *parser) parseAtomEscape() (*node, error) {
	p.pos++ // backslash
	if !p.more() {
		return nil, syntaxError("\\ at end of pattern")
	}
	c := p.src[p.pos]
	if rs, ok := classEscapeRanges(c); ok {
		p.pos++
		return &node{kind: nSet, set: &charSet{ranges: rs}, icase: p.flags.icase}, nil
	}
	if c >= '1' && c <= '9' {
		start, v := p.pos, 0
		for p.more() && IsDigit(p.src[p.pos]) {
			if v < 1<<20 {
				v = v*10 + int(p.src[p.pos]-'0')
			}
			p.pos++
		}
		if v <= p.totalCaps {
			return &node{kind: nBackref, n: v, icase: p.flags.icase}, nil
		}
		p.pos = start
	}
	if c == 'k' && p.hasNames {
		p.pos++
		if !p.peekIs('<') {
			return nil, syntaxError("Invalid named reference")
		}
		p.pos++
		name, err := p.parseGroupName()
		if err != nil {
			return nil, syntaxError("Invalid named reference")
		}
		ref := &node{kind: nBackref, name: name, icase: p.flags.icase}
		p.backrefs = append(p.backrefs, ref)
		return ref, nil
	}
	if c == 'c' {
		if next, ok := p.peekAt(1); ok && next|0x20 >= 'a' && next|0x20 <= 'z' {
			p.pos += 2
			return p.literal(next % 32), nil
		}
		// \c without a control letter is a literal backslash; the c is read next.
		return p.literal('\\'), nil
	}
	return p.literal(p.characterEscape()), nil
}

// characterEscape reads the escape at p.pos (after the backslash) that denotes one code unit:
// control escapes, legacy octal, \x and \u hex escapes, and identity escapes.
func (p *parser) characterEscape() uint16 {
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	case 'x':
		if v, ok := p.hexDigits(2); ok {
			return v
		}
	case 'u':
		if v, ok := p.hexDigits(4); ok {
			return v
		}
	case '0', '1', '2', '3', '4', '5', '6', '7':
		// Legacy octal: up to three digits with a value of at most 0377.
		v := int(c - '0')
		maxDigits := 2
		if c >= '4' {
			maxDigits = 1
		}
		for i := 0; i < maxDigits && p.more() && p.src[p.pos] >= '0' && p.src[p.pos] <= '7'; i++ {
			v = v*8 + int(p.src[p.pos]-'0')
			p.pos++
		}
		return uint16(v)
	}
	return c
}

func (p *parser) hexDigits(n int) (uint16, bool) {
	if p.pos+n > len(p.src) {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		d := digitValue(p.src[p.pos+i])
		if d >= 16 {
			return 0, false
		}
		v = v*16 + d
	}
	p.pos += n
	return uint16(v), true
}

// parseClass parses a character class [...] or [^...].
func (p *parser) parseClass() (*node, error) {
	p.pos++ // [
	set := &charSet{}
	if p.peekIs('^') {
		p.pos++
		set.negate = true
	}
	var ranges []unitRange
	for {
		if !p.more() {
			return nil, syntaxError("Unterminated character class")
		}
		if p.src[p.pos] == ']' {
			p.pos++
			break
		}
		lo, loSet, err := p.parseClassAtom()
		if err != nil {
			return nil, err
		}
		if p.peekIs('-') {
			if next, ok := p.peekAt(1); ok && next != ']' {
				p.pos++ // -
				hi, hiSet, err := p.parseClassAtom()
				if err != nil {
					return nil, err
				}
				if loSet != nil || hiSet != nil {
					// Annex B: a range with a class escape at either end is a union with '-'.
					ranges = append(ranges, classAtomRanges(lo, loSet)...)
					ranges = append(ranges, unitRange{'-', '-'})
					ranges = append(ranges, classAtomRanges(hi, hiSet)...)
					continue
				}
				if lo > hi {
					return nil, syntaxError("Range out of order in character class")
				}
				ranges = append(ranges, unitRange{lo, hi})
				continue
			}
		}
		ranges = append(ranges, classAtomRanges(lo, loSet)...)
	}
	set.ranges = normalizeRanges(ranges)
	return &node{kind: nSet, set: set, icase: p.flags.icase}, nil
}

func classAtomRanges(u uint16, rs []unitRange) []unitRange {
	if rs != nil {
		return rs
	}
	return []unitRange{{u, u}}
}

// parseClassAtom reads one class atom: a code unit, or the ranges of a class escape.
func (p *parser) parseClassAtom() (uint16, []unitRange, error) {
	c := p.src[p.pos]
	if c != '\\' {
		p.pos++
		return c, nil, nil
	}
	p.pos++
	if !p.more() {
		return 0, nil, syntaxError("\\ at end of pattern")
	}
	e := p.src[p.pos]
	if rs, ok := classEscapeRanges(e); ok {
		p.pos++
		return 0, rs, nil
	}
	switch e {
	case 'b':
		p.pos++
		return '\b', nil, nil
	case '-':
		p.pos++
		return '-', nil, nil
	case 'c':
		if next, ok := p.peekAt(1); ok && (next|0x20 >= 'a' && next|0x20 <= 'z' || IsDigit(next) || next == '_') {
			p.pos += 2
			return next % 32, nil, nil
		}
		// A literal backslash; the c is read as the next atom.
		return '\\', nil, nil
	}
	return p.characterEscape(), nil, nil
}
