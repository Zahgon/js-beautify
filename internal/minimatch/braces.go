package minimatch

import (
	"math"
	"regexp"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// This file ports brace-expansion 5.0.6 and the balanced-match 4.0.4 function it uses. All the
// characters they look at are ASCII, so they work on the UTF-8 bytes of the pattern.

// The placeholders brace-expansion substitutes for escaped characters while it expands. The
// original builds them from Math.random(); NUL bytes keep them out of real patterns.
const (
	escSlash  = "\x00SLASH\x01\x00"
	escOpen   = "\x00OPEN\x01\x00"
	escClose  = "\x00CLOSE\x01\x00"
	escComma  = "\x00COMMA\x01\x00"
	escPeriod = "\x00PERIOD\x01\x00"
)

// expansionMax is brace-expansion's default limit on the number of expansions.
const expansionMax = 100_000

var (
	numericSequence = regexp.MustCompile(`^-?\d+\.\.-?\d+(?:\.\.-?\d+)?$`)
	alphaSequence   = regexp.MustCompile(`^[a-zA-Z]\.\.[a-zA-Z](?:\.\.-?\d+)?$`)
	paddedNumber    = regexp.MustCompile(`^-?0\d`)
)

// braceRange is balanced-match's range('{', '}', str): the indexes of the first balanced pair.
func braceRange(str string) (int, int, bool) {
	ai := strings.IndexByte(str, '{')
	bi := indexByteFrom(str, '}', ai+1)
	i := ai
	if ai < 0 || bi <= 0 {
		return 0, 0, false
	}
	var begs []int
	left, right, haveRight := len(str), 0, false
	result, found := [2]int{}, false
	for i >= 0 && !found {
		switch {
		case i == ai:
			begs = append(begs, i)
			ai = indexByteFrom(str, '{', i+1)
		case len(begs) == 1:
			result, found = [2]int{begs[0], bi}, true
			begs = begs[:0]
		default:
			if len(begs) > 0 {
				beg := begs[len(begs)-1]
				begs = begs[:len(begs)-1]
				if beg < left {
					left, right, haveRight = beg, bi, true
				}
			}
			bi = indexByteFrom(str, '}', i+1)
		}
		if ai < bi && ai >= 0 {
			i = ai
		} else {
			i = bi
		}
	}
	if !found && len(begs) > 0 && haveRight {
		return left, right, true
	}
	return result[0], result[1], found
}

func indexByteFrom(s string, c byte, from int) int {
	if from < 0 {
		from = 0
	}
	if from > len(s) {
		return -1
	}
	i := strings.IndexByte(s[from:], c)
	if i < 0 {
		return -1
	}
	return i + from
}

type balancedMatch struct {
	pre, body, post string
}

// balanced is balanced-match's balanced('{', '}', str).
func balanced(str string) (balancedMatch, bool) {
	start, end, ok := braceRange(str)
	if !ok {
		return balancedMatch{}, false
	}
	return balancedMatch{pre: str[:start], body: str[start+1 : end], post: str[end+1:]}, true
}

var (
	escaper   = strings.NewReplacer(`\\`, escSlash, `\{`, escOpen, `\}`, escClose, `\,`, escComma, `\.`, escPeriod)
	unescaper = strings.NewReplacer(escSlash, `\`, escOpen, "{", escClose, "}", escComma, ",", escPeriod, ".")
)

// parseCommaParts splits str on the commas that are not inside a nested brace set.
func parseCommaParts(str string) []string {
	if str == "" {
		return []string{""}
	}
	m, ok := balanced(str)
	if !ok {
		return strings.Split(str, ",")
	}
	p := strings.Split(m.pre, ",")
	p[len(p)-1] += "{" + m.body + "}"
	postParts := parseCommaParts(m.post)
	if m.post != "" {
		p[len(p)-1] += postParts[0]
		p = append(p, postParts[1:]...)
	}
	return p
}

// braceExpand is brace-expansion's expand(str, {max}).
func braceExpand(str string, max int) []string {
	if str == "" {
		return []string{}
	}
	if strings.HasPrefix(str, "{}") {
		str = `\{\}` + str[2:]
	}
	out := expandBraces(escaper.Replace(str), max, true)
	for i, s := range out {
		out[i] = unescaper.Replace(s)
	}
	return out
}

// numeric is brace-expansion's numeric(): parseInt of a number, or the first character's code.
func numeric(s string) float64 {
	if !math.IsNaN(jsrt.StringToNumber(jsrt.FromUTF8(s))) {
		return jsrt.ParseInt(jsrt.FromUTF8(s), 10)
	}
	if s == "" {
		return math.NaN()
	}
	return float64(s[0])
}

func expandBraces(str string, max int, isTop bool) []string {
	var expansions []string
	m, ok := balanced(str)
	if !ok {
		return []string{str}
	}
	pre := m.pre
	post := []string{""}
	if m.post != "" {
		post = expandBraces(m.post, max, false)
	}
	if strings.HasSuffix(m.pre, "$") {
		for k := 0; k < len(post) && k < max; k++ {
			expansions = append(expansions, pre+"{"+m.body+"}"+post[k])
		}
		return expansions
	}
	isNumericSeq := numericSequence.MatchString(m.body)
	isAlphaSeq := alphaSequence.MatchString(m.body)
	isSequence := isNumericSeq || isAlphaSeq
	isOptions := strings.Contains(m.body, ",")
	if !isSequence && !isOptions {
		// {a},b}
		if hasLoneCommaBeforeClose(m.post) {
			return expandBraces(m.pre+"{"+m.body+escClose+m.post, max, true)
		}
		return []string{str}
	}
	var n []string
	if isSequence {
		n = strings.Split(m.body, "..")
	} else {
		n = parseCommaParts(m.body)
		if len(n) == 1 {
			// x{{a,b}}y ==> x{a}y x{b}y
			inner := expandBraces(n[0], max, false)
			n = make([]string, len(inner))
			for i, s := range inner {
				n[i] = "{" + s + "}"
			}
			if len(n) == 1 {
				out := make([]string, len(post))
				for i, p := range post {
					out[i] = m.pre + n[0] + p
				}
				return out
			}
		}
	}
	var items []string
	if isSequence {
		x, y := numeric(n[0]), numeric(n[1])
		width := len(n[0])
		if len(n[1]) > width {
			width = len(n[1])
		}
		incr := 1.0
		if len(n) == 3 {
			incr = math.Max(math.Abs(numeric(n[2])), 1)
		}
		test := func(i, y float64) bool { return i <= y }
		if y < x {
			incr = -incr
			test = func(i, y float64) bool { return i >= y }
		}
		pad := false
		for _, s := range n {
			pad = pad || paddedNumber.MatchString(s)
		}
		for i := x; test(i, y) && len(items) < max; i += incr {
			var c string
			if isAlphaSeq {
				c = string(rune(int(i)))
				if c == `\` {
					c = ""
				}
			} else {
				c = string(jsrt.NumberToString(i))
				if pad {
					if need := width - len(c); need > 0 {
						z := strings.Repeat("0", need)
						if i < 0 {
							c = "-" + z + c[1:]
						} else {
							c = z + c
						}
					}
				}
			}
			items = append(items, c)
		}
	} else {
		for _, part := range n {
			items = append(items, expandBraces(part, max, false)...)
		}
	}
	for _, item := range items {
		for k := 0; k < len(post) && len(expansions) < max; k++ {
			expansion := pre + item + post[k]
			if !isTop || isSequence || expansion != "" {
				expansions = append(expansions, expansion)
			}
		}
	}
	return expansions
}

// hasLoneCommaBeforeClose tests /,(?!,).*\}/: a comma not followed by another, then a "}" on
// the same line.
func hasLoneCommaBeforeClose(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ',' || (i+1 < len(s) && s[i+1] == ',') {
			continue
		}
		rest := s[i+1:]
		if j := strings.IndexAny(rest, "\n\r\u2028\u2029"); j >= 0 {
			rest = rest[:j]
		}
		if strings.Contains(rest, "}") {
			return true
		}
	}
	return false
}
