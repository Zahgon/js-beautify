package jsrt

import (
	"strings"
	"unicode"
)

// IsWhitespace reports whether u is in the JavaScript \s class (WhiteSpace and LineTerminator),
// which is also the set String.prototype.trim and parseInt skip.
func IsWhitespace(u uint16) bool {
	switch {
	case u >= 0x09 && u <= 0x0D, u == 0x20, u == 0xA0, u == 0x1680:
		return true
	case u < 0x2000:
		return false
	case u <= 0x200A, u == 0x2028, u == 0x2029, u == 0x202F, u == 0x205F, u == 0x3000, u == 0xFEFF:
		return true
	}
	return false
}

// IsLineTerminator reports whether u is a JavaScript LineTerminator (\n, \r, U+2028, U+2029):
// the code units that . does not match and that ^ and $ see in multiline mode.
func IsLineTerminator(u uint16) bool {
	return u == '\n' || u == '\r' || u == 0x2028 || u == 0x2029
}

// IsWordChar reports whether u is in the regex \w class ([A-Za-z0-9_]).
func IsWordChar(u uint16) bool {
	return u == '_' || IsDigit(u) || (u|0x20 >= 'a' && u|0x20 <= 'z')
}

// IsDigit reports whether u is in the regex \d class ([0-9]).
func IsDigit(u uint16) bool {
	return u >= '0' && u <= '9'
}

func isSurrogate(u uint16) bool {
	return u >= 0xD800 && u <= 0xDFFF
}

// multiUnitUpper lists the BMP characters whose full uppercase mapping (Unicode SpecialCasing)
// is longer than one code unit, e.g. ß -> SS. Their regex canonical form is themselves.
var multiUnitUpper = [][2]uint16{
	{0x00DF, 0x00DF}, {0x0149, 0x0149}, {0x01F0, 0x01F0}, {0x0390, 0x0390}, {0x03B0, 0x03B0},
	{0x0587, 0x0587}, {0x1E96, 0x1E9A}, {0x1F50, 0x1F50}, {0x1F52, 0x1F52}, {0x1F54, 0x1F54},
	{0x1F56, 0x1F56}, {0x1F80, 0x1FAF}, {0x1FB2, 0x1FB4}, {0x1FB6, 0x1FB7}, {0x1FBC, 0x1FBC},
	{0x1FC2, 0x1FC4}, {0x1FC6, 0x1FC7}, {0x1FCC, 0x1FCC}, {0x1FD2, 0x1FD3}, {0x1FD6, 0x1FD7},
	{0x1FE2, 0x1FE4}, {0x1FE6, 0x1FE7}, {0x1FF2, 0x1FF4}, {0x1FF6, 0x1FF7}, {0x1FFC, 0x1FFC},
	{0xFB00, 0xFB06}, {0xFB13, 0xFB17},
}

// canonicalize is the Canonicalize operation of non-unicode case-insensitive regexes: the code
// unit's uppercase when that is a single code unit and does not turn a non-ASCII unit into an
// ASCII one, otherwise the unit itself.
func canonicalize(u uint16) uint16 {
	if u < 0x80 {
		if u >= 'a' && u <= 'z' {
			return u - 0x20
		}
		return u
	}
	if isSurrogate(u) {
		return u
	}
	for _, r := range multiUnitUpper {
		if u >= r[0] && u <= r[1] {
			return u
		}
	}
	up := unicode.ToUpper(rune(u))
	if up > 0xFFFF || up < 0x80 {
		return u
	}
	return uint16(up)
}

// caseVariants calls f with every BMP code unit in the simple case-folding orbit of u (u
// included) until f returns true.
func caseVariants(u uint16, f func(uint16) bool) bool {
	if f(u) {
		return true
	}
	if isSurrogate(u) {
		return false
	}
	for r := unicode.SimpleFold(rune(u)); r != rune(u); r = unicode.SimpleFold(r) {
		if r <= 0xFFFF && f(uint16(r)) {
			return true
		}
	}
	return false
}

// ToLowerCase returns s.toLowerCase() with JavaScript's full, locale-independent mapping:
// U+0130 becomes "i̇" and a capital sigma at the end of a word becomes a final sigma.
func (s String) ToLowerCase() String {
	if s.isASCII() {
		return String(strings.ToLower(string(s)))
	}
	runes := codePoints(s.Units())
	var b strings.Builder
	for i, r := range runes {
		switch {
		case r == 0x130:
			b.WriteString("i̇")
		case r == 0x3A3 && isFinalSigma(runes, i):
			b.WriteString(string(FromUTF8("ς")))
		case r >= 0xD800 && r <= 0xDFFF:
			appendUnit(&b, uint16(r))
		default:
			b.WriteString(string(FromUTF8(string(unicode.ToLower(r)))))
		}
	}
	return String(b.String())
}

// codePoints decodes units into code points, keeping lone surrogates as they are.
func codePoints(units []uint16) []rune {
	out := make([]rune, 0, len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
			out = append(out, 0x10000+(rune(u)-0xD800)<<10+rune(units[i+1])-0xDC00)
			i++
			continue
		}
		out = append(out, rune(u))
	}
	return out
}

// isFinalSigma implements the Final_Sigma casing context: the sigma at i follows a cased letter
// (skipping case-ignorable characters) and is not followed by one.
func isFinalSigma(runes []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0; j-- {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		before = isCased(runes[j])
		break
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(runes); j++ {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		return !isCased(runes[j])
	}
	return true
}

func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r)
}

func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0xB7, 0x387, 0x5F4, 0x2018, 0x2019, 0x2024, 0x2027, 0xFE13,
		0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
