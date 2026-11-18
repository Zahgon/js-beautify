package jsrt

import (
	"errors"
	"unicode/utf8"
)

var errBadJSONString = errors.New("jsrt: invalid JSON string literal")

// UnquoteJSON decodes a JSON string literal (with its quotes) the way JSON.parse does, keeping
// \uXXXX escapes of lone surrogates as code units where encoding/json writes U+FFFD.
func UnquoteJSON(literal []byte) (String, error) {
	if len(literal) < 2 || literal[0] != '"' || literal[len(literal)-1] != '"' {
		return "", errBadJSONString
	}
	body := literal[1 : len(literal)-1]
	units := make([]uint16, 0, len(body))
	for i := 0; i < len(body); {
		c := body[i]
		if c != '\\' {
			r, size := utf8.DecodeRune(body[i:])
			units = append(units, FromUTF8(string(r)).Units()...)
			i += size
			continue
		}
		if i+1 >= len(body) {
			return "", errBadJSONString
		}
		e := body[i+1]
		i += 2
		switch e {
		case '"', '\\', '/':
			units = append(units, uint16(e))
		case 'b':
			units = append(units, '\b')
		case 'f':
			units = append(units, '\f')
		case 'n':
			units = append(units, '\n')
		case 'r':
			units = append(units, '\r')
		case 't':
			units = append(units, '\t')
		case 'u':
			if i+4 > len(body) {
				return "", errBadJSONString
			}
			v := 0
			for _, h := range body[i : i+4] {
				d := digitValue(uint16(h))
				if d >= 16 {
					return "", errBadJSONString
				}
				v = v*16 + d
			}
			units = append(units, uint16(v))
			i += 4
		default:
			return "", errBadJSONString
		}
	}
	return FromUnits(units), nil
}
