package core

import (
	"sync"
	"sync/atomic"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// InputScanner walks the source text one UTF-16 code unit at a time (js/src/core/inputscanner.js).
//
// Where the original returns null (Next and Peek past either end) these methods return "". Where
// the original concatenates that null into a string it gets the text "null"; a port of such code
// writes "null" explicitly.
type InputScanner struct {
	input    []uint16
	position int
}

// NewInputScanner returns a scanner at the start of input.
func NewInputScanner(input jsrt.String) *InputScanner {
	return &InputScanner{input: input.Units()}
}

// Restart moves back to the start of the input.
func (s *InputScanner) Restart() {
	s.position = 0
}

// Back moves back one code unit, unless at the start.
func (s *InputScanner) Back() {
	if s.position > 0 {
		s.position--
	}
}

// HasNext reports whether code units remain.
func (s *InputScanner) HasNext() bool {
	return s.position < len(s.input)
}

// Next returns the next code unit and advances, or "" at the end.
func (s *InputScanner) Next() jsrt.String {
	if !s.HasNext() {
		return ""
	}
	s.position++
	return jsrt.FromUnit(s.input[s.position-1])
}

// Peek returns the code unit index units ahead of the current one (peek(index); Peek(0) is
// peek()), or "" outside the input.
func (s *InputScanner) Peek(index int) jsrt.String {
	index += s.position
	if index >= 0 && index < len(s.input) {
		return jsrt.FromUnit(s.input[index])
	}
	return ""
}

// match returns the match of pattern starting exactly at index (__match). The original runs exec
// with lastIndex = index and rejects a match found elsewhere: for a global or sticky regex that
// is the same as trying index alone, while any other regex searches from the start of the input,
// so it only succeeds when its first match in the whole input starts at index.
func (s *InputScanner) match(pattern *jsrt.Regexp, index int) *jsrt.Match {
	if pattern.Global() || pattern.Sticky() {
		return pattern.MatchAt(s.input, index)
	}
	if m := pattern.ExecUnits(s.input, index); m != nil && m.Index() == index {
		return m
	}
	return nil
}

// Test reports whether pattern matches at index units ahead of the current position.
func (s *InputScanner) Test(pattern *jsrt.Regexp, index int) bool {
	index += s.position
	if index >= 0 && index < len(s.input) {
		return s.match(pattern, index) != nil
	}
	return false
}

// TestChar reports whether pattern matches the single code unit index units ahead (testChar).
func (s *InputScanner) TestChar(pattern *jsrt.Regexp, index int) bool {
	val := s.Peek(index)
	return val != "" && pattern.Test(val)
}

// Match returns the match of pattern at the current position and moves past it, or nil.
func (s *InputScanner) Match(pattern *jsrt.Regexp) *jsrt.Match {
	m := s.match(pattern, s.position)
	if m != nil {
		s.position += m.End() - m.Index()
	}
	return m
}

// Read returns read(startingPattern, untilPattern, untilAfter): the match of startingPattern at
// the current position (when given), then, if that matched or no startingPattern was given, the
// text up to untilPattern (or past it when untilAfter). Nil patterns are absent.
func (s *InputScanner) Read(startingPattern, untilPattern *jsrt.Regexp, untilAfter bool) jsrt.String {
	var val jsrt.String
	var m *jsrt.Match
	if startingPattern != nil {
		m = s.Match(startingPattern)
		if m != nil {
			val += m.Text()
		}
	}
	if untilPattern != nil && (m != nil || startingPattern == nil) {
		val += s.ReadUntil(untilPattern, untilAfter)
	}
	return val
}

// ReadUntil returns the text from the current position up to the next match of pattern, or up
// to its end when untilAfter, or the rest of the input when there is no match. The search is
// exec with lastIndex at the current position: a sticky pattern must match right here, and a
// pattern that is neither global nor sticky searches from the start of the input; when its match
// lies behind the position, the original's substring returns the text in between and the
// position moves back to the match.
func (s *InputScanner) ReadUntil(pattern *jsrt.Regexp, untilAfter bool) jsrt.String {
	matchIndex := len(s.input)
	if m := pattern.ExecUnits(s.input, s.position); m != nil {
		matchIndex = m.Index()
		if untilAfter {
			matchIndex = m.End()
		}
	}
	val := jsrt.FromUnits(s.input[min(s.position, matchIndex):max(s.position, matchIndex)])
	s.position = matchIndex
	return val
}

// ReadUntilAfter is ReadUntil(pattern, true).
func (s *InputScanner) ReadUntilAfter(pattern *jsrt.Regexp) jsrt.String {
	return s.ReadUntil(pattern, true)
}

// GetRegexp returns get_regexp(pattern, matchFrom): the pattern with the y flag when matchFrom
// (patterns matched where the scanner stands) or the g flag otherwise (patterns searched for).
// A nil pattern gives nil.
func (s *InputScanner) GetRegexp(pattern *jsrt.Regexp, matchFrom bool) *jsrt.Regexp {
	if pattern == nil {
		return nil
	}
	return regexpWithFlags(pattern.Source(), matchFrom)
}

// regexpWithFlags compiles a pattern built by the program with the y or g flag.
func regexpWithFlags(source jsrt.String, matchFrom bool) *jsrt.Regexp {
	flags := "g"
	if matchFrom {
		flags = "y"
	}
	return compileCached(source, flags)
}

// compiledPatterns caches the patterns every tokenizer rebuilds over its own input: compiling
// them (the JavaScript identifier classes especially) would otherwise dominate a beautifier run.
// A *jsrt.Regexp is immutable, so one compiled pattern serves every scanner. Some sources come
// from option values (HTML's unformatted_content_delimiter), so the cache stops growing at
// maxCompiledPatterns entries.
var (
	compiledPatterns      sync.Map
	compiledPatternsCount atomic.Int64
)

const maxCompiledPatterns = 1024

type patternKey struct {
	source jsrt.String
	flags  string
}

// compileCached is jsrt.MustCompile, reusing an earlier compilation of the same pattern.
func compileCached(source jsrt.String, flags string) *jsrt.Regexp {
	key := patternKey{source, flags}
	if re, ok := compiledPatterns.Load(key); ok {
		return re.(*jsrt.Regexp)
	}
	re := jsrt.MustCompile(source, flags)
	if compiledPatternsCount.Load() < maxCompiledPatterns {
		if _, loaded := compiledPatterns.LoadOrStore(key, re); !loaded {
			compiledPatternsCount.Add(1)
		}
	}
	return re
}

var literalRegexpEscape = jsrt.MustCompile(`[-\/\\^$*+?.()|[\]{}]`, "g")

// GetLiteralRegexp returns get_literal_regexp: a regex (without flags) matching literal.
func (s *InputScanner) GetLiteralRegexp(literal jsrt.String) *jsrt.Regexp {
	return jsrt.MustCompile(literalRegexpEscape.Replace(literal, `\$&`), "")
}

// PeekUntilAfter returns what ReadUntilAfter(pattern) would read, without moving.
func (s *InputScanner) PeekUntilAfter(pattern *jsrt.Regexp) jsrt.String {
	start := s.position
	val := s.ReadUntilAfter(pattern)
	s.position = start
	return val
}

// LookBack reports whether the testVal.length code units before the last one read are testVal,
// compared case-insensitively (the text is lowercased).
func (s *InputScanner) LookBack(testVal jsrt.String) bool {
	start := s.position - 1
	n := testVal.Len()
	return start >= n && jsrt.FromUnits(s.input[start-n:start]).ToLowerCase() == testVal
}
