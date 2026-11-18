package jsrt

import (
	"strings"
	"unicode/utf8"
)

// String is a JavaScript string value: a sequence of UTF-16 code units stored in a Go string with
// every code unit encoded on its own (CESU-8). Code units below U+0800 use their UTF-8 bytes and
// every other unit, surrogates included, takes three bytes, so BMP text is plain UTF-8 and two
// Strings are equal exactly when their code units are.
//
// Untyped Go string constants convert implicitly (s == "{" works). Convert Go text that may hold
// astral characters with FromUTF8, never with a String(...) conversion.
type String string

// FromUTF8 converts UTF-8 text into a String. Invalid UTF-8 bytes become U+FFFD.
func FromUTF8(s string) String {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return String(s)
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/2)
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			appendUnit(&b, uint16(0xD800+(r>>10)))
			appendUnit(&b, uint16(0xDC00+(r&0x3FF)))
		} else {
			appendUnit(&b, uint16(r))
		}
	}
	return String(b.String())
}

// FromUnits converts UTF-16 code units into a String.
func FromUnits(units []uint16) String {
	ascii := true
	for _, u := range units {
		if u >= 0x80 {
			ascii = false
			break
		}
	}
	var b strings.Builder
	if ascii {
		b.Grow(len(units))
		for _, u := range units {
			b.WriteByte(byte(u))
		}
		return String(b.String())
	}
	b.Grow(len(units) * 2)
	for _, u := range units {
		appendUnit(&b, u)
	}
	return String(b.String())
}

// FromUnit returns the one-unit String holding u (String.fromCharCode(u)).
func FromUnit(u uint16) String {
	if u < 0x80 {
		return String(rune(u))
	}
	var b strings.Builder
	appendUnit(&b, u)
	return String(b.String())
}

func appendUnit(b *strings.Builder, u uint16) {
	switch {
	case u < 0x80:
		b.WriteByte(byte(u))
	case u < 0x800:
		b.WriteByte(byte(0xC0 | u>>6))
		b.WriteByte(byte(0x80 | u&0x3F))
	default:
		b.WriteByte(byte(0xE0 | u>>12))
		b.WriteByte(byte(0x80 | (u>>6)&0x3F))
		b.WriteByte(byte(0x80 | u&0x3F))
	}
}

// decodeUnit returns the code unit encoded at byte offset i and the number of units and bytes it
// takes. A four-byte UTF-8 sequence (only produced by a String(...) conversion of Go text) counts
// as its surrogate pair.
func decodeUnit(s String, i int) (units [2]uint16, n int, size int) {
	c := s[i]
	switch {
	case c < 0x80:
		return [2]uint16{uint16(c)}, 1, 1
	case c >= 0xF0 && i+3 < len(s):
		r := rune(c&0x07)<<18 | rune(s[i+1]&0x3F)<<12 | rune(s[i+2]&0x3F)<<6 | rune(s[i+3]&0x3F)
		r -= 0x10000
		return [2]uint16{uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))}, 2, 4
	case c >= 0xE0 && i+2 < len(s):
		return [2]uint16{uint16(c&0x0F)<<12 | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F)}, 1, 3
	case c >= 0xC0 && i+1 < len(s):
		return [2]uint16{uint16(c&0x1F)<<6 | uint16(s[i+1]&0x3F)}, 1, 2
	}
	return [2]uint16{0xFFFD}, 1, 1
}

// Units returns the UTF-16 code units of s.
func (s String) Units() []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		u, n, size := decodeUnit(s, i)
		out = append(out, u[:n]...)
		i += size
	}
	return out
}

// UTF8 converts s to UTF-8 as Node does when writing a string out: surrogate pairs become their
// astral character and every lone surrogate becomes U+FFFD.
func (s String) UTF8() string {
	if s.isASCII() {
		return string(s)
	}
	units := s.Units()
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			b.WriteRune(0x10000 + (rune(u)-0xD800)<<10 + rune(units[i+1]) - 0xDC00)
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteRune(rune(u))
		}
	}
	return b.String()
}

func (s String) isASCII() bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// Len returns the number of UTF-16 code units in s (JavaScript's length).
func (s String) Len() int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c < 0x80 || (c >= 0xC0 && c < 0xF0):
			n++
		case c >= 0xF0:
			n += 2
		}
	}
	return n
}

// byteOffset returns the byte offset of code unit index i (0 <= i <= Len). An index that falls
// inside a four-byte sequence rounds down to its start.
func (s String) byteOffset(i int) int {
	if i <= 0 {
		return 0
	}
	n := 0
	for b := 0; b < len(s); {
		if n >= i {
			return b
		}
		_, units, size := decodeUnit(s, b)
		n += units
		b += size
	}
	return len(s)
}

// unitIndex returns the code unit index of byte offset b.
func (s String) unitIndex(b int) int {
	return s[:b].Len()
}

// CharCodeAt returns the code unit at index i, or -1 when i is out of range (where JavaScript's
// charCodeAt returns NaN).
func (s String) CharCodeAt(i int) int {
	if i < 0 {
		return -1
	}
	n := 0
	for b := 0; b < len(s); {
		u, units, size := decodeUnit(s, b)
		if i < n+units {
			return int(u[i-n])
		}
		n += units
		b += size
	}
	return -1
}

// CharAt returns the one-unit String at index i, or "" when i is out of range (s.charAt(i), and
// s[i] where the original indexes a string).
func (s String) CharAt(i int) String {
	c := s.CharCodeAt(i)
	if c < 0 {
		return ""
	}
	return FromUnit(uint16(c))
}

// Substring returns s.substring(start, end): both indexes are clamped to [0, Len] and swapped
// when start > end.
func (s String) Substring(start, end int) String {
	n := s.Len()
	start, end = clamp(start, 0, n), clamp(end, 0, n)
	if start > end {
		start, end = end, start
	}
	return s.units(start, end)
}

// SubstringFrom returns s.substring(start).
func (s String) SubstringFrom(start int) String {
	return s.Substring(start, s.Len())
}

// Slice returns s.slice(start, end): negative indexes count from the end and an empty String
// results when start >= end.
func (s String) Slice(start, end int) String {
	n := s.Len()
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	start, end = clamp(start, 0, n), clamp(end, 0, n)
	if start >= end {
		return ""
	}
	return s.units(start, end)
}

// units returns code units [start, end) of s, with 0 <= start <= end <= Len.
func (s String) units(start, end int) String {
	if s.isASCII() {
		return s[start:end]
	}
	u := s.Units()
	return FromUnits(u[start:end])
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// IndexOf returns s.indexOf(search): the code unit index of the first occurrence, or -1.
func (s String) IndexOf(search String) int {
	return s.IndexOfFrom(search, 0)
}

// IndexOfFrom returns s.indexOf(search, from).
func (s String) IndexOfFrom(search String, from int) int {
	from = clamp(from, 0, s.Len())
	b := s.byteOffset(from)
	i := strings.Index(string(s[b:]), string(search))
	if i < 0 {
		return -1
	}
	return s.unitIndex(b + i)
}

// LastIndexOf returns s.lastIndexOf(search), or -1.
func (s String) LastIndexOf(search String) int {
	i := strings.LastIndex(string(s), string(search))
	if i < 0 {
		return -1
	}
	return s.unitIndex(i)
}

// Includes reports s.includes(search).
func (s String) Includes(search String) bool {
	return strings.Contains(string(s), string(search))
}

// StartsWith reports s.startsWith(prefix).
func (s String) StartsWith(prefix String) bool {
	return strings.HasPrefix(string(s), string(prefix))
}

// EndsWith reports s.endsWith(suffix).
func (s String) EndsWith(suffix String) bool {
	return strings.HasSuffix(string(s), string(suffix))
}

// Split returns s.split(separator) for a string separator: an empty separator splits into code
// units, and splitting the empty String by a non-empty separator gives [""].
func (s String) Split(separator String) []String {
	if separator == "" {
		u := s.Units()
		out := make([]String, len(u))
		for i, c := range u {
			out[i] = FromUnit(c)
		}
		return out
	}
	parts := strings.Split(string(s), string(separator))
	out := make([]String, len(parts))
	for i, p := range parts {
		out[i] = String(p)
	}
	return out
}

// Replace returns s.replace(search, replacement) for a string search: the first occurrence is
// replaced, with the $$, $&, $` and $' patterns of replacement expanded.
func (s String) Replace(search, replacement String) String {
	i := s.IndexOf(search)
	if i < 0 {
		return s
	}
	u := s.Units()
	j := i + search.Len()
	return FromUnits(u[:i]) + expandReplacement(replacement, u, []int{i, j}, nil) + FromUnits(u[j:])
}

// ReplaceAll returns s.replaceAll(search, replacement) for a string search.
func (s String) ReplaceAll(search, replacement String) String {
	u := s.Units()
	n := search.Len()
	var b strings.Builder
	last := 0
	for i := s.IndexOf(search); i >= 0; {
		b.WriteString(string(FromUnits(u[last:i])))
		b.WriteString(string(expandReplacement(replacement, u, []int{i, i + n}, nil)))
		last = i + n
		if n == 0 {
			// An empty search matches before every code unit and at the end.
			if i >= len(u) {
				break
			}
			b.WriteString(string(FromUnit(u[i])))
			last = i + 1
			i++
			continue
		}
		i = s.IndexOfFrom(search, last)
	}
	if last <= len(u) {
		b.WriteString(string(FromUnits(u[last:])))
	}
	return String(b.String())
}

// Trim returns s.trim(): leading and trailing JavaScript whitespace and line terminators removed.
func (s String) Trim() String {
	return s.TrimStart().TrimEnd()
}

// TrimStart returns s.trimStart().
func (s String) TrimStart() String {
	u := s.Units()
	i := 0
	for i < len(u) && IsWhitespace(u[i]) {
		i++
	}
	if i == 0 {
		return s
	}
	return FromUnits(u[i:])
}

// TrimEnd returns s.trimEnd().
func (s String) TrimEnd() String {
	u := s.Units()
	i := len(u)
	for i > 0 && IsWhitespace(u[i-1]) {
		i--
	}
	if i == len(u) {
		return s
	}
	return FromUnits(u[:i])
}

// Join returns parts.join(separator).
func Join(parts []String, separator String) String {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(string(separator))
		}
		b.WriteString(string(p))
	}
	return String(b.String())
}
