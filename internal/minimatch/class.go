package minimatch

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// posixClass is a POSIX bracket class: the character class body minimatch translates it to,
// whether the class is negated, and whether the body needs the u flag.
type posixClass struct {
	name    string
	body    string
	negated bool
	uflag   bool
}

// posixClasses lists the POSIX classes in minimatch's order, with minimatch's translations.
var posixClasses = []posixClass{
	{"[:alnum:]", `\p{L}\p{Nl}\p{Nd}`, false, true},
	{"[:alpha:]", `\p{L}\p{Nl}`, false, true},
	{"[:ascii:]", `\x00-\x7f`, false, false},
	{"[:blank:]", `\p{Zs}\t`, false, true},
	{"[:cntrl:]", `\p{Cc}`, false, true},
	{"[:digit:]", `\p{Nd}`, false, true},
	{"[:graph:]", `\p{Z}\p{C}`, true, true},
	{"[:lower:]", `\p{Ll}`, false, true},
	{"[:print:]", `\p{C}`, false, true},
	{"[:punct:]", `\p{P}`, false, true},
	{"[:space:]", `\p{Z}\t\r\n\v\f`, false, true},
	{"[:upper:]", `\p{Lu}`, false, true},
	{"[:word:]", `\p{L}\p{Nl}\p{Nd}\p{Pc}`, false, true},
	{"[:xdigit:]", "A-Fa-f0-9", false, false},
}

// propertyRanges spells the Unicode property escapes of posixClasses as ranges of code units of
// the Basic Multilingual Plane, so that the expressions compile without the u flag.
var propertyRanges = strings.NewReplacer(
	`\p{L}`, bmpRanges(unicode.L), `\p{Nl}`, bmpRanges(unicode.Nl), `\p{Nd}`, bmpRanges(unicode.Nd),
	`\p{Zs}`, bmpRanges(unicode.Zs), `\p{Cc}`, bmpRanges(unicode.Cc), `\p{Z}`, bmpRanges(unicode.Z),
	`\p{C}`, bmpRanges(unicode.C), `\p{Ll}`, bmpRanges(unicode.Ll), `\p{P}`, bmpRanges(unicode.P),
	`\p{Lu}`, bmpRanges(unicode.Lu), `\p{Pc}`, bmpRanges(unicode.Pc),
)

// bmpRanges writes the code points below U+10000 of a table (with the surrogates, which the C
// category holds) as a character class body of \uXXXX escapes.
func bmpRanges(t *unicode.RangeTable) string {
	var b strings.Builder
	write := func(lo, hi rune) {
		if lo == hi {
			fmt.Fprintf(&b, `\u%04X`, lo)
		} else {
			fmt.Fprintf(&b, `\u%04X-\u%04X`, lo, hi)
		}
	}
	for _, r := range t.R16 {
		if r.Stride == 1 {
			write(rune(r.Lo), rune(r.Hi))
			continue
		}
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			write(c, c)
		}
	}
	if t == unicode.C {
		write(0xD800, 0xDFFF)
	}
	return b.String()
}

// checkUnicodeEscapes rejects, as V8 does under the u flag, the identity escapes that only
// non-unicode expressions allow: outside a class only syntax characters and "/" may be
// escaped, inside one "-" too.
func checkUnicodeEscapes(src []uint16) bool {
	inClass := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src):
			i++
			e := src[i]
			if strings.ContainsRune(`^$\.*+?()[]{}|/fnrtvpuxdDwWsSbB0`, rune(e)) || (inClass && e == '-') {
				continue
			}
			return false
		case c == '[' && !inClass:
			inClass = true
		case c == ']' && inClass:
			inClass = false
		}
	}
	return true
}

// braceEscape escapes the characters that are special inside a class: [ \ ] -
func braceEscape(s jsrt.String) jsrt.String {
	var b strings.Builder
	for _, u := range s.Units() {
		if u == '[' || u == ']' || u == '\\' || u == '-' {
			b.WriteByte('\\')
		}
		b.WriteString(string(jsrt.FromUnit(u)))
	}
	return jsrt.String(b.String())
}

// regexpEscape escapes every character a regular expression treats specially, and whitespace.
func regexpEscape(s jsrt.String) jsrt.String {
	var b strings.Builder
	for _, u := range s.Units() {
		if strings.ContainsRune(`-[]{}()*+?.,\^$|#`, rune(u)) || jsrt.IsWhitespace(u) {
			b.WriteByte('\\')
		}
		b.WriteString(string(jsrt.FromUnit(u)))
	}
	return jsrt.String(b.String())
}

// parseClass translates the bracket expression of glob at pos into a regular expression: the
// source, whether it needs the u flag, the number of code units consumed (0 when the expression
// is not closed and must be read literally), and whether it has magic. Out-of-order ranges are
// dropped; a class that can match nothing becomes "$.".
func parseClass(glob []uint16, pos int) (jsrt.String, bool, int, bool) {
	var ranges, negs []jsrt.String
	i := pos + 1
	sawStart, uflag, escaping, negate := false, false, false, false
	endPos := pos
	rangeStart := jsrt.String("")
	startsWith := func(s string, at int) bool {
		if at+len(s) > len(glob) {
			return false
		}
		for k := 0; k < len(s); k++ {
			if glob[at+k] != uint16(s[k]) {
				return false
			}
		}
		return true
	}
outer:
	for i < len(glob) {
		c := glob[i]
		if (c == '!' || c == '^') && i == pos+1 {
			negate = true
			i++
			continue
		}
		if c == ']' && sawStart && !escaping {
			endPos = i + 1
			break
		}
		sawStart = true
		if c == '\\' && !escaping {
			escaping = true
			i++
			continue
		}
		if c == '[' && !escaping {
			for _, pc := range posixClasses {
				if startsWith(pc.name, i) {
					if rangeStart != "" {
						return "$.", false, len(glob) - pos, true
					}
					i += len(pc.name)
					if pc.negated {
						negs = append(negs, jsrt.String(pc.body))
					} else {
						ranges = append(ranges, jsrt.String(pc.body))
					}
					uflag = uflag || pc.uflag
					continue outer
				}
			}
		}
		escaping = false
		cs := jsrt.FromUnit(c)
		if rangeStart != "" {
			start := rangeStart.Units()[0]
			if c > start {
				ranges = append(ranges, braceEscape(rangeStart)+"-"+braceEscape(cs))
			} else if c == start {
				ranges = append(ranges, braceEscape(cs))
			}
			rangeStart = ""
			i++
			continue
		}
		if startsWith("-]", i+1) {
			ranges = append(ranges, braceEscape(cs+"-"))
			i += 2
			continue
		}
		if startsWith("-", i+1) {
			rangeStart = cs
			i += 2
			continue
		}
		ranges = append(ranges, braceEscape(cs))
		i++
	}
	if endPos < i {
		return "", false, 0, false
	}
	if len(ranges) == 0 && len(negs) == 0 {
		return "$.", false, len(glob) - pos, true
	}
	if len(negs) == 0 && len(ranges) == 1 && !negate {
		// A single character (/^\\?.$/, where "." is not a line terminator) is a literal.
		r := ranges[0].Units()
		if (len(r) == 1 || (len(r) == 2 && r[0] == '\\')) && !jsrt.IsLineTerminator(r[len(r)-1]) {
			return regexpEscape(jsrt.FromUnit(r[len(r)-1])), false, endPos - pos, false
		}
	}
	neg := ""
	if negate {
		neg = "^"
	}
	notNeg := "^"
	if negate {
		notNeg = ""
	}
	sranges := "[" + jsrt.String(neg) + jsrt.Join(ranges, "") + "]"
	snegs := "[" + jsrt.String(notNeg) + jsrt.Join(negs, "") + "]"
	switch {
	case len(ranges) > 0 && len(negs) > 0:
		return "(" + sranges + "|" + snegs + ")", uflag, endPos - pos, true
	case len(ranges) > 0:
		return sranges, uflag, endPos - pos, true
	}
	return snegs, uflag, endPos - pos, true
}
