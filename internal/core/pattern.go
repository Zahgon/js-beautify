package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Pattern reads a token shaped as "starting pattern, then matching pattern or text up to an
// until pattern" (js/src/core/pattern.js). Patterns are immutable: StartingWith, Matching,
// Until and UntilAfter return a modified copy, so a tokenizer builds its patterns once from a
// shared base.
type Pattern struct {
	input           *InputScanner
	startingPattern *jsrt.Regexp
	matchPattern    *jsrt.Regexp
	untilPattern    *jsrt.Regexp
	untilAfter      bool
}

// NewPattern returns an empty pattern reading from input.
func NewPattern(input *InputScanner) *Pattern {
	return &Pattern{input: input}
}

// Read reads the pattern at the current position: the starting pattern, then (if the starting
// pattern matched or there is none) the matching pattern and the text up to the until pattern.
func (p *Pattern) Read() jsrt.String {
	result := p.input.Read(p.startingPattern, nil, false)
	if p.startingPattern == nil || result != "" {
		result += p.input.Read(p.matchPattern, p.untilPattern, p.untilAfter)
	}
	return result
}

// ReadMatch returns the match of the matching pattern at the current position and moves past
// it, or nil (read_match).
func (p *Pattern) ReadMatch() *jsrt.Match {
	return p.input.Match(p.matchPattern)
}

// UntilAfter returns a copy that reads up to and including the next match of pattern.
func (p *Pattern) UntilAfter(pattern *jsrt.Regexp) *Pattern {
	r := *p
	r.setUntil(pattern, true)
	return &r
}

// Until returns a copy that reads up to the next match of pattern.
func (p *Pattern) Until(pattern *jsrt.Regexp) *Pattern {
	r := *p
	r.setUntil(pattern, false)
	return &r
}

// StartingWith returns a copy that requires pattern at the current position first.
func (p *Pattern) StartingWith(pattern *jsrt.Regexp) *Pattern {
	r := *p
	r.setStartingWith(pattern)
	return &r
}

// Matching returns a copy whose body is pattern, matched at the current position.
func (p *Pattern) Matching(pattern *jsrt.Regexp) *Pattern {
	r := *p
	r.setMatching(pattern)
	return &r
}

func (p *Pattern) setUntil(pattern *jsrt.Regexp, after bool) {
	p.untilAfter = after
	p.untilPattern = p.input.GetRegexp(pattern, false)
}

func (p *Pattern) setStartingWith(pattern *jsrt.Regexp) {
	p.startingPattern = p.input.GetRegexp(pattern, true)
}

func (p *Pattern) setMatching(pattern *jsrt.Regexp) {
	p.matchPattern = p.input.GetRegexp(pattern, true)
}
