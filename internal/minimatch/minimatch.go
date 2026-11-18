// Package minimatch ports minimatch 10.2.5 (with brace-expansion 5.0.6), the glob matcher that
// glob 13 and editorconfig 3 use: brace expansion, the per-portion translation of globs and
// extglobs into regular expressions, the pattern-set optimizations, and path matching with
// globstars. Regular expressions run on jsrt's JavaScript engine.
package minimatch

import (
	"regexp"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
)

// Options are the minimatch options that glob and editorconfig set.
type Options struct {
	Dot       bool
	MatchBase bool
	NoNegate  bool
	NoComment bool
	NoExt     bool
	// OptimizationLevel is minimatch's optimizationLevel; 0 means the default, 1.
	OptimizationLevel int
	// BraceExpandMax limits brace expansion; 0 means brace-expansion's default, 100000.
	BraceExpandMax int
}

func (o Options) level() int {
	if o.OptimizationLevel == 0 {
		return 1
	}
	return o.OptimizationLevel
}

// Part is one path portion of a compiled pattern: a literal string, a globstar ("**"), or a
// regular expression.
type Part struct {
	globstar bool
	literal  string
	re       *jsrt.Regexp
	fast     func(string) bool
}

// IsGlobstar reports whether p is "**".
func (p Part) IsGlobstar() bool { return p.globstar }

// IsRegexp reports whether p needs a regular expression (it has magic).
func (p Part) IsRegexp() bool { return p.re != nil }

// Literal returns the text of a part that is neither a globstar nor a regular expression.
func (p Part) Literal() string { return p.literal }

// Test reports whether the path portion f matches a regular expression part.
func (p Part) Test(f string) bool {
	if p.fast != nil {
		return p.fast(f)
	}
	return p.re.Test(jsrt.FromUTF8(f))
}

// Minimatch is a compiled pattern.
type Minimatch struct {
	options Options
	pattern string
	negate  bool
	comment bool
	empty   bool
	// Set holds, for every alternative of the pattern, its compiled path portions.
	Set [][]Part
	// GlobParts holds the path portions of every alternative as glob text.
	GlobParts [][]string
}

const maxPatternLength = 1024 * 64

// New compiles pattern. It fails as minimatch does for a pattern longer than 65536 code units.
func New(pattern string, options Options) (*Minimatch, error) {
	if jsrt.FromUTF8(pattern).Len() > maxPatternLength {
		return nil, nodefmt.NewError("TypeError", "pattern is too long")
	}
	m := &Minimatch{options: options, pattern: pattern}
	if err := m.make(); err != nil {
		return nil, err
	}
	return m, nil
}

// HasMagic reports whether any part of the pattern needs a regular expression or a globstar.
func (m *Minimatch) HasMagic() bool {
	for _, set := range m.Set {
		for _, p := range set {
			if p.globstar || p.re != nil {
				return true
			}
		}
	}
	return false
}

func (m *Minimatch) make() error {
	if !m.options.NoComment && strings.HasPrefix(m.pattern, "#") {
		m.comment = true
		return nil
	}
	if m.pattern == "" {
		m.empty = true
		return nil
	}
	m.parseNegate()
	var globSet []string
	seen := map[string]bool{}
	for _, s := range m.braceExpand() {
		if !seen[s] {
			seen[s] = true
			globSet = append(globSet, s)
		}
	}
	raw := make([][]string, len(globSet))
	for i, s := range globSet {
		raw[i] = slashSplit(s)
	}
	m.GlobParts = m.preprocess(raw)
	for _, parts := range m.GlobParts {
		set := make([]Part, len(parts))
		for i, ss := range parts {
			p, err := m.parse(ss)
			if err != nil {
				return err
			}
			set[i] = p
		}
		m.Set = append(m.Set, set)
	}
	return nil
}

func (m *Minimatch) parseNegate() {
	if m.options.NoNegate {
		return
	}
	n := 0
	for n < len(m.pattern) && m.pattern[n] == '!' {
		n++
	}
	m.negate = n%2 == 1
	m.pattern = m.pattern[n:]
}

// hasBraceSet tests /\{(?:(?!\{).)*\}/: a "{" closed by a "}" with no "{" or line terminator
// between them.
func hasBraceSet(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		rest := s[i+1:]
		if j := strings.IndexAny(rest, "{}\n\r  "); j >= 0 && rest[j] == '}' {
			return true
		}
	}
	return false
}

func (m *Minimatch) braceExpand() []string {
	if !hasBraceSet(m.pattern) {
		return []string{m.pattern}
	}
	max := m.options.BraceExpandMax
	if max == 0 {
		max = expansionMax
	}
	return braceExpand(m.pattern, max)
}

var multiSlash = regexp.MustCompile(`/+`)

func slashSplit(p string) []string {
	return multiSlash.Split(p, -1)
}

func (m *Minimatch) preprocess(globParts [][]string) [][]string {
	if m.options.level() >= 2 {
		globParts = m.firstPhasePreProcess(globParts)
		return m.secondPhasePreProcess(globParts)
	}
	return levelOneOptimize(globParts)
}

// levelOneOptimize removes adjacent "**" portions and resolves ".." portions.
func levelOneOptimize(globParts [][]string) [][]string {
	out := make([][]string, len(globParts))
	for i, parts := range globParts {
		var set []string
		for _, part := range parts {
			prev := ""
			if len(set) > 0 {
				prev = set[len(set)-1]
			}
			if part == "**" && prev == "**" {
				continue
			}
			if part == ".." && prev != "" && prev != ".." && prev != "." && prev != "**" {
				set = set[:len(set)-1]
				continue
			}
			set = append(set, part)
		}
		if len(set) == 0 {
			set = []string{""}
		}
		out[i] = set
	}
	return out
}

func spliceStrings(s []string, i, n int, items ...string) []string {
	out := make([]string, 0, len(s)-n+len(items))
	out = append(out, s[:i]...)
	out = append(out, items...)
	return append(out, s[i+n:]...)
}

func indexFrom(s []string, v string, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == v {
			return i
		}
	}
	return -1
}

func at(s []string, i int) (string, bool) {
	if i < 0 || i >= len(s) {
		return "", false
	}
	return s[i], true
}

// squeezeEmpty removes "." and "" portions inside parts (<pre>/<e>/<rest> -> <pre>/<rest>).
func squeezeEmpty(parts []string) ([]string, bool) {
	did := false
	for i := 1; i < len(parts)-1; i++ {
		p := parts[i]
		if i == 1 && p == "" && parts[0] == "" {
			continue
		}
		if p == "." || p == "" {
			did = true
			parts = spliceStrings(parts, i, 1)
			i--
		}
	}
	if len(parts) == 2 && parts[0] == "." && (parts[1] == "." || parts[1] == "") {
		did = true
		parts = parts[:1]
	}
	return parts, did
}

// levelTwoFileOptimize resolves "." and ".." portions of a path being matched.
func levelTwoFileOptimize(parts []string) []string {
	parts = append([]string(nil), parts...)
	for {
		var did bool
		parts, did = squeezeEmpty(parts)
		for dd := indexFrom(parts, "..", 1); dd != -1; dd = indexFrom(parts, "..", dd+1) {
			p, _ := at(parts, dd-1)
			if p != "" && p != "." && p != ".." && p != "**" {
				did = true
				parts = spliceStrings(parts, dd-1, 2)
				dd -= 2
			}
		}
		if !did {
			break
		}
	}
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

// firstPhasePreProcess simplifies each pattern for a file system walk: **/.. expansions, empty
// and "." portions, <p>/.. pairs and repeated **.
func (m *Minimatch) firstPhasePreProcess(globParts [][]string) [][]string {
	for {
		did := false
		for gi := 0; gi < len(globParts); gi++ {
			parts := globParts[gi]
			for gs := indexFrom(parts, "**", 0); gs != -1; gs = indexFrom(parts, "**", gs+1) {
				gss := gs
				for gss+1 < len(parts) && parts[gss+1] == "**" {
					gss++
				}
				if gss > gs {
					parts = spliceStrings(parts, gs+1, gss-gs)
				}
				next, _ := at(parts, gs+1)
				p, _ := at(parts, gs+2)
				p2, _ := at(parts, gs+3)
				if next != ".." {
					continue
				}
				if p == "" || p == "." || p == ".." || p2 == "" || p2 == "." || p2 == ".." {
					continue
				}
				did = true
				parts = spliceStrings(parts, gs, 1)
				other := append([]string(nil), parts...)
				other[gs] = "**"
				globParts = append(globParts, other)
				gs--
			}
			var squeezed bool
			parts, squeezed = squeezeEmpty(parts)
			did = did || squeezed
			for dd := indexFrom(parts, "..", 1); dd != -1; dd = indexFrom(parts, "..", dd+1) {
				p, _ := at(parts, dd-1)
				if p != "" && p != "." && p != ".." && p != "**" {
					did = true
					var splin []string
					if dd == 1 {
						if n, _ := at(parts, dd+1); n == "**" {
							splin = []string{"."}
						}
					}
					parts = spliceStrings(parts, dd-1, 2, splin...)
					if len(parts) == 0 {
						parts = append(parts, "")
					}
					dd -= 2
				}
			}
			globParts[gi] = parts
		}
		if !did {
			return globParts
		}
	}
}

// secondPhasePreProcess removes patterns that another one covers.
func (m *Minimatch) secondPhasePreProcess(globParts [][]string) [][]string {
	for i := 0; i < len(globParts)-1; i++ {
		for j := i + 1; j < len(globParts); j++ {
			if matched, ok := m.partsMatch(globParts[i], globParts[j], true); ok {
				globParts[i] = nil
				globParts[j] = matched
				break
			}
		}
	}
	var out [][]string
	for _, gs := range globParts {
		if len(gs) > 0 {
			out = append(out, gs)
		}
	}
	return out
}

func (m *Minimatch) partsMatch(a, b []string, emptyGSMatch bool) ([]string, bool) {
	ai, bi := 0, 0
	var result []string
	which := byte(0)
	for ai < len(a) && bi < len(b) {
		aNext, _ := at(a, ai+1)
		bNext, _ := at(b, bi+1)
		switch {
		case a[ai] == b[bi]:
			if which == 'b' {
				result = append(result, b[bi])
			} else {
				result = append(result, a[ai])
			}
			ai++
			bi++
		case emptyGSMatch && a[ai] == "**" && ai+1 < len(a) && b[bi] == aNext:
			result = append(result, a[ai])
			ai++
		case emptyGSMatch && b[bi] == "**" && bi+1 < len(b) && a[ai] == bNext:
			result = append(result, b[bi])
			bi++
		case a[ai] == "*" && b[bi] != "" && (m.options.Dot || !strings.HasPrefix(b[bi], ".")) && b[bi] != "**":
			if which == 'b' {
				return nil, false
			}
			which = 'a'
			result = append(result, a[ai])
			ai++
			bi++
		case b[bi] == "*" && a[ai] != "" && (m.options.Dot || !strings.HasPrefix(a[ai], ".")) && a[ai] != "**":
			if which == 'a' {
				return nil, false
			}
			which = 'b'
			result = append(result, b[bi])
			ai++
			bi++
		default:
			return nil, false
		}
	}
	if len(a) != len(b) {
		return nil, false
	}
	return result, true
}

var (
	starRE        = regexp.MustCompile(`^\*+$`)
	starDotExtRE  = regexp.MustCompile(`^\*+([^+@!?*[(]*)$`)
	qmarksRE      = regexp.MustCompile(`^\?+([^+@!?*[(]*)?$`)
	starDotStarRE = regexp.MustCompile(`^\*+\.\*+$`)
	dotStarRE     = regexp.MustCompile(`^\.\*+$`)
)

func jsLen(s string) int { return jsrt.FromUTF8(s).Len() }

// fastTest returns minimatch's shortcut test for the most common portion patterns (*, *.ext,
// ??? and the like), which replaces the regular expression's test.
func fastTest(pattern string, dot bool) func(string) bool {
	if starRE.MatchString(pattern) {
		if dot {
			return func(f string) bool { return f != "" && f != "." && f != ".." }
		}
		return func(f string) bool { return f != "" && !strings.HasPrefix(f, ".") }
	}
	if m := starDotExtRE.FindStringSubmatch(pattern); m != nil {
		ext := m[1]
		if dot {
			return func(f string) bool { return strings.HasSuffix(f, ext) }
		}
		return func(f string) bool { return !strings.HasPrefix(f, ".") && strings.HasSuffix(f, ext) }
	}
	if m := qmarksRE.FindStringSubmatch(pattern); m != nil {
		n, ext := jsLen(m[0]), m[1]
		noext := func(f string) bool { return jsLen(f) == n && !strings.HasPrefix(f, ".") }
		if dot {
			noext = func(f string) bool { return jsLen(f) == n && f != "." && f != ".." }
		}
		if ext == "" {
			return noext
		}
		return func(f string) bool { return noext(f) && strings.HasSuffix(f, ext) }
	}
	if starDotStarRE.MatchString(pattern) {
		if dot {
			return func(f string) bool { return f != "." && f != ".." && strings.Contains(f, ".") }
		}
		return func(f string) bool { return !strings.HasPrefix(f, ".") && strings.Contains(f, ".") }
	}
	if dotStarRE.MatchString(pattern) {
		return func(f string) bool { return f != "." && f != ".." && strings.HasPrefix(f, ".") }
	}
	return nil
}

// parse compiles one path portion.
func (m *Minimatch) parse(pattern string) (Part, error) {
	if jsLen(pattern) > maxPatternLength {
		return Part{}, nodefmt.NewError("TypeError", "pattern is too long")
	}
	if pattern == "**" {
		return Part{globstar: true}, nil
	}
	if pattern == "" {
		return Part{}, nil
	}
	a := fromGlob(jsrt.FromUTF8(pattern), astOptions{dot: m.options.Dot, noext: m.options.NoExt})
	src := a.toRegExpSource(unset)
	if !src.hasMagic && a.hasMagic != isTrue {
		return Part{literal: src.body.UTF8()}, nil
	}
	source := "^" + src.re + "$"
	if src.uflag || a.uflag {
		if !checkUnicodeEscapes(source.Units()) {
			return Part{}, nodefmt.NewError("SyntaxError", "Invalid regular expression: /"+source.UTF8()+"/u: Invalid escape")
		}
		source = jsrt.String(propertyRanges.Replace(string(source)))
	}
	re, err := jsrt.Compile(source, "")
	if err != nil {
		return Part{}, nodefmt.NewError("SyntaxError", err.Error())
	}
	return Part{re: re, fast: fastTest(pattern, m.options.Dot)}, nil
}

// Match reports whether the path f matches the pattern.
func (m *Minimatch) Match(f string) bool {
	if m.comment {
		return false
	}
	if m.empty {
		return f == ""
	}
	ff := slashSplit(f)
	filename := ff[len(ff)-1]
	for i := len(ff) - 2; filename == "" && i >= 0; i-- {
		filename = ff[i]
	}
	for _, pattern := range m.Set {
		file := ff
		if m.options.MatchBase && len(pattern) == 1 {
			file = []string{filename}
		}
		if m.matchOne(file, pattern) {
			return !m.negate
		}
	}
	return m.negate
}

func (m *Minimatch) matchOne(file []string, pattern []Part) bool {
	if m.options.level() >= 2 {
		file = levelTwoFileOptimize(file)
	}
	for _, p := range pattern {
		if p.globstar {
			return m.matchGlobstar(file, pattern)
		}
	}
	return m.matchParts(file, pattern, 0, 0)
}

// maxGlobstarRecursion is minimatch's default limit on nested globstar body sections.
const maxGlobstarRecursion = 200

func (m *Minimatch) badDot(f string) bool {
	return f == "." || f == ".." || (!m.options.Dot && strings.HasPrefix(f, "."))
}

func (m *Minimatch) matchGlobstar(file []string, pattern []Part) bool {
	firstgs, lastgs := -1, -1
	for i, p := range pattern {
		if p.globstar {
			if firstgs < 0 {
				firstgs = i
			}
			lastgs = i
		}
	}
	head, tail := pattern[:firstgs], pattern[lastgs+1:]
	var body []Part
	if lastgs > firstgs {
		body = pattern[firstgs+1 : lastgs]
	}
	fileIndex := 0
	if len(head) > 0 {
		end := len(head)
		if end > len(file) {
			end = len(file)
		}
		if !m.matchParts(file[:end], head, 0, 0) {
			return false
		}
		fileIndex += len(head)
	}
	fileTailMatch := 0
	if len(tail) > 0 {
		if len(tail)+fileIndex > len(file) {
			return false
		}
		tailStart := len(file) - len(tail)
		if m.matchParts(file, tail, tailStart, 0) {
			fileTailMatch = len(tail)
		} else {
			// Affordance for a/**/* matching a/b/: try again without the trailing "".
			if file[len(file)-1] != "" || fileIndex+len(tail) == len(file) {
				return false
			}
			tailStart--
			if !m.matchParts(file, tail, tailStart, 0) {
				return false
			}
			fileTailMatch = len(tail) + 1
		}
	}
	if len(body) == 0 {
		sawSome := fileTailMatch != 0
		for i := fileIndex; i < len(file)-fileTailMatch; i++ {
			sawSome = true
			if m.badDot(file[i]) {
				return false
			}
		}
		return sawSome
	}
	type section struct {
		parts []Part
		after int
	}
	sections := []*section{{}}
	current := sections[0]
	nonGsParts := 0
	nonGsPartsSums := []int{0}
	for _, b := range body {
		if b.globstar {
			nonGsPartsSums = append(nonGsPartsSums, nonGsParts)
			current = &section{}
			sections = append(sections, current)
		} else {
			current.parts = append(current.parts, b)
			nonGsParts++
		}
	}
	fileLength := len(file) - fileTailMatch
	i := len(sections) - 1
	for _, s := range sections {
		s.after = fileLength - (nonGsPartsSums[i] + len(s.parts))
		i--
	}
	var walk func(fileIndex, bodyIndex, depth int, sawTail bool) (bool, bool)
	// walk returns (matched, keepTrying).
	walk = func(fileIndex, bodyIndex, depth int, sawTail bool) (bool, bool) {
		if bodyIndex >= len(sections) {
			for i := fileIndex; i < len(file); i++ {
				sawTail = true
				if m.badDot(file[i]) {
					return false, true
				}
			}
			return sawTail, true
		}
		s := sections[bodyIndex]
		for fileIndex <= s.after {
			end := fileIndex + len(s.parts)
			if end > len(file) {
				end = len(file)
			}
			if m.matchParts(file[:end], s.parts, fileIndex, 0) && depth < maxGlobstarRecursion {
				if ok, more := walk(fileIndex+len(s.parts), bodyIndex+1, depth+1, sawTail); ok || !more {
					return ok, more
				}
			}
			if fileIndex < len(file) && m.badDot(file[fileIndex]) {
				return false, true
			}
			fileIndex++
		}
		return false, false
	}
	ok, _ := walk(fileIndex, 0, 0, fileTailMatch != 0)
	return ok
}

// matchParts is minimatch's #matchOne without globstars: file portions from fileIndex against
// pattern portions from patternIndex.
func (m *Minimatch) matchParts(file []string, pattern []Part, fileIndex, patternIndex int) bool {
	fi, pi := fileIndex, patternIndex
	fl, pl := len(file), len(pattern)
	for ; fi < fl && pi < pl; fi, pi = fi+1, pi+1 {
		p, f := pattern[pi], file[fi]
		if p.globstar {
			return false
		}
		var hit bool
		if p.re != nil {
			hit = p.Test(f)
		} else {
			hit = f == p.literal
		}
		if !hit {
			return false
		}
	}
	switch {
	case fi == fl && pi == pl:
		return true
	case fi == fl:
		return false
	}
	return fi == fl-1 && file[fi] == ""
}

// Escape escapes the magic characters of s by wrapping them in brackets, as minimatch's escape
// does with windowsPathsNoEscape.
func Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte("?*()[]", c) >= 0 {
			b.WriteByte('[')
			b.WriteByte(c)
			b.WriteByte(']')
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

var (
	unescapeClass  = jsrt.MustCompile(`((?!\\).|^)\[([^/\\])\]`, "g")
	unescapeEscape = jsrt.MustCompile(`\\([^/])`, "g")
)

// unescape removes the bracket and backslash escapes of a glob.
func unescape(s jsrt.String) jsrt.String {
	return unescapeEscape.Replace(unescapeClass.Replace(s, "$1$2"), "$1")
}

// LiteralPart returns a part that matches the path portion s exactly.
func LiteralPart(s string) Part {
	return Part{literal: s}
}
