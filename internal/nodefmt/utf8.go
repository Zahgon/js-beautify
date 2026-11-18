package nodefmt

import (
	"strings"
	"unicode/utf8"
)

// DecodeUTF8 returns the string Node makes of bytes read as UTF-8 (buffer.toString('utf8'),
// fs.readFileSync(path, 'utf8') and a utf8-decoding stdin): each maximal ill-formed subsequence
// becomes one U+FFFD, as the WHATWG decoder specifies. A byte order mark is kept.
func DecodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
	out.Grow(len(b) + 8)
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			out.WriteByte(c)
			i++
			continue
		}
		need, lower, upper := 0, byte(0x80), byte(0xBF)
		switch {
		case c >= 0xC2 && c <= 0xDF:
			need = 1
		case c == 0xE0:
			need, lower = 2, 0xA0
		case c == 0xED:
			need, upper = 2, 0x9F
		case c >= 0xE1 && c <= 0xEF:
			need = 2
		case c == 0xF0:
			need, lower = 3, 0x90
		case c == 0xF4:
			need, upper = 3, 0x8F
		case c >= 0xF1 && c <= 0xF3:
			need = 3
		default:
			out.WriteRune(utf8.RuneError)
			i++
			continue
		}
		j := i + 1
		complete := true
		for k := 0; k < need; k++ {
			if j >= len(b) || b[j] < lower || b[j] > upper {
				complete = false
				break
			}
			lower, upper = 0x80, 0xBF
			j++
		}
		if complete {
			out.Write(b[i:j])
		} else {
			out.WriteRune(utf8.RuneError)
		}
		i = j
	}
	return out.String()
}
