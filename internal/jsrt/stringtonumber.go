package jsrt

import (
	"math"
	"math/big"
	"strconv"
)

// StringToNumber returns Number(s), the ToNumber conversion of a string: surrounding whitespace
// and line terminators are ignored, the empty string is 0, "0x", "0o" and "0b" prefixes select
// radix 16, 8 and 2 (without a sign), "Infinity" may carry a sign, and anything else must be a
// complete decimal literal or the result is NaN.
func StringToNumber(s String) float64 {
	t := string(s.Trim())
	if t == "" {
		return 0
	}
	if len(t) > 2 && t[0] == '0' {
		radix := 0
		switch t[1] {
		case 'x', 'X':
			radix = 16
		case 'o', 'O':
			radix = 8
		case 'b', 'B':
			radix = 2
		}
		if radix != 0 {
			return radixToNumber(t[2:], radix)
		}
	}
	body := t
	sign := 1.0
	if body[0] == '+' || body[0] == '-' {
		if body[0] == '-' {
			sign = -1
		}
		body = body[1:]
	}
	if body == "Infinity" {
		return sign * math.Inf(1)
	}
	if !isDecimalLiteral(body) {
		return math.NaN()
	}
	// The literal is well formed, so ParseFloat can only report a range error, for which it
	// returns the same ±Inf or 0 that JavaScript gives.
	v, _ := strconv.ParseFloat(body, 64)
	return sign * v
}

// isDecimalLiteral reports whether s is a StrUnsignedDecimalLiteral without "Infinity": digits
// with an optional fraction (at least one digit overall) and an optional exponent.
func isDecimalLiteral(s string) bool {
	i, digits := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

// radixToNumber converts the digits of a 0x, 0o or 0b literal, rounding to the nearest double as
// JavaScript does; any character that is not a digit of the radix makes the result NaN.
func radixToNumber(digits string, radix int) float64 {
	n, ok := new(big.Int).SetString(digits, radix)
	if !ok || digits[0] == '+' || digits[0] == '-' || digits[0] == '_' {
		return math.NaN()
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] == '_' {
			return math.NaN()
		}
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}
