package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// WhitespacePattern reads the whitespace before a token and records how many line breaks it
// held and the whitespace after the last one (js/src/core/whitespacepattern.js).
type WhitespacePattern struct {
	Pattern
	newlineRegexp *jsrt.Regexp

	// NewlineCount and WhitespaceBeforeToken describe the whitespace read by the last Read.
	NewlineCount          int
	WhitespaceBeforeToken jsrt.String
}

// NewWhitespacePattern returns the default pattern: spaces and tabs, with \n and \r as line
// breaks (\r\n counting once).
func NewWhitespacePattern(input *InputScanner) *WhitespacePattern {
	p := &WhitespacePattern{Pattern: Pattern{input: input}}
	p.setWhitespacePatterns("", "")
	return p
}

func (p *WhitespacePattern) setWhitespacePatterns(whitespaceChars, newlineChars jsrt.String) {
	whitespaceChars += `\t `
	newlineChars += `\n\r`

	p.matchPattern = regexpWithFlags("["+whitespaceChars+newlineChars+"]+", true)
	p.newlineRegexp = regexpWithFlags(`\r\n|[`+newlineChars+"]", false)
}

// Read reads the whitespace at the current position and returns it.
func (p *WhitespacePattern) Read() jsrt.String {
	p.NewlineCount = 0
	p.WhitespaceBeforeToken = ""

	resultingString := p.input.Read(p.matchPattern, nil, false)
	if resultingString == " " {
		p.WhitespaceBeforeToken = " "
	} else if resultingString != "" {
		matches := split(p.newlineRegexp, resultingString)
		p.NewlineCount = len(matches) - 1
		p.WhitespaceBeforeToken = matches[p.NewlineCount]
	}

	return resultingString
}

// Matching returns a copy that also treats whitespaceChars as whitespace and newlineChars as
// line breaks; both are regex character-class fragments such as `  `.
func (p *WhitespacePattern) Matching(whitespaceChars, newlineChars jsrt.String) *WhitespacePattern {
	r := WhitespacePattern{Pattern: p.Pattern}
	r.setWhitespacePatterns(whitespaceChars, newlineChars)
	return &r
}

// split is __split: the pieces of input between the matches of regexp, with a final "" when the
// input ends with a match.
func split(regexp *jsrt.Regexp, input jsrt.String) []jsrt.String {
	units := input.Units()
	start := 0
	var result []jsrt.String
	for m := regexp.ExecUnits(units, 0); m != nil; m = regexp.ExecUnits(units, m.End()) {
		result = append(result, jsrt.FromUnits(units[start:m.Index()]))
		start = m.End()
	}
	if start < len(units) {
		result = append(result, jsrt.FromUnits(units[start:]))
	} else {
		result = append(result, "")
	}
	return result
}
