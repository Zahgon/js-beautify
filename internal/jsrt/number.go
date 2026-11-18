package jsrt

import (
	"math"
	"strconv"
	"strings"
)

// NumberToString returns String(x) for a JavaScript number (Number::toString with radix 10):
// the shortest round-tripping digits, in plain notation for 1e-7 <= |x| < 1e21 and in
// exponential notation ("1e+21", "1.5e-7") otherwise.
func NumberToString(x float64) String {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case x == 0:
		return "0"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x < 0:
		return "-" + NumberToString(-x)
	}
	// Shortest digits and decimal exponent: x = d1.d2...dk × 10^ex.
	e := strconv.FormatFloat(x, 'e', -1, 64)
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	ex := 0
	for _, c := range exp[1:] {
		ex = ex*10 + int(c-'0')
	}
	if exp[0] == '-' {
		ex = -ex
	}
	k, n := len(digits), ex+1
	switch {
	case k <= n && n <= 21:
		return String(digits + strings.Repeat("0", n-k))
	case 0 < n && n <= 21:
		return String(digits[:n] + "." + digits[n:])
	case -6 < n && n <= 0:
		return String("0." + strings.Repeat("0", -n) + digits)
	}
	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	abs := n - 1
	if abs < 0 {
		abs = -abs
	}
	if k == 1 {
		return String(digits + "e" + sign + strconv.Itoa(abs))
	}
	return String(digits[:1] + "." + digits[1:] + "e" + sign + strconv.Itoa(abs))
}

// ParseInt returns parseInt(s, radix): leading whitespace is skipped, an optional sign and (for
// radix 16 or 0) a 0x prefix are read, then the longest run of digits valid in the radix. The
// result is NaN when there is no digit or the radix is outside 2..36 (0 means 10, or 16 with a
// 0x prefix).
func ParseInt(s String, radix int) float64 {
	u := s.Units()
	i := 0
	for i < len(u) && IsWhitespace(u[i]) {
		i++
	}
	sign := 1.0
	if i < len(u) && (u[i] == '-' || u[i] == '+') {
		if u[i] == '-' {
			sign = -1
		}
		i++
	}
	stripPrefix := true
	if radix != 0 {
		if radix < 2 || radix > 36 {
			return math.NaN()
		}
		if radix != 16 {
			stripPrefix = false
		}
	} else {
		radix = 10
	}
	if stripPrefix && i+1 < len(u) && u[i] == '0' && (u[i+1] == 'x' || u[i+1] == 'X') {
		i += 2
		radix = 16
	}
	start := i
	for i < len(u) && digitValue(u[i]) < radix {
		i++
	}
	if i == start {
		return math.NaN()
	}
	digits := string(FromUnits(u[start:i]))
	if radix == 10 {
		// Decimal digits are converted with correct rounding, as V8 does.
		v, err := strconv.ParseFloat(digits, 64)
		if err != nil {
			// Only overflow is possible here; V8 yields Infinity.
			v = math.Inf(1)
		}
		return sign * v
	}
	v := 0.0
	for _, c := range []byte(digits) {
		v = v*float64(radix) + float64(digitValue(uint16(c)))
	}
	return sign * v
}

func digitValue(u uint16) int {
	switch {
	case u >= '0' && u <= '9':
		return int(u - '0')
	case u >= 'a' && u <= 'z':
		return int(u-'a') + 10
	case u >= 'A' && u <= 'Z':
		return int(u-'A') + 10
	}
	return 36
}
