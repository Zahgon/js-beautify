package html

import (
	"slices"

	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// printer writes tokens to the output at the current indentation (Printer in
// js/src/html/beautifier.js).
type printer struct {
	indentLevel         int
	alignmentSize       int
	maxPreserveNewlines int
	preserveNewlines    bool
	output              *core.Output
}

func newPrinter(options *Options, baseIndentString jsrt.String) (*printer, error) {
	output, err := core.NewOutput(options.Options, baseIndentString)
	if err != nil {
		return nil, err
	}
	return &printer{
		maxPreserveNewlines: options.MaxPreserveNewlines,
		preserveNewlines:    options.PreserveNewlines,
		output:              output,
	}, nil
}

func (p *printer) currentLineHasMatch(pattern *jsrt.Regexp) bool {
	return p.output.CurrentLine.HasMatch(pattern)
}

func (p *printer) setSpaceBeforeToken(value, nonBreaking bool) {
	p.output.SpaceBeforeToken = value
	p.output.NonBreakingSpace = nonBreaking
}

func (p *printer) setWrapPoint() {
	p.output.SetIndent(p.indentLevel, p.alignmentSize)
	p.output.SetWrapPoint()
}

func (p *printer) addRawToken(token *core.Token) {
	p.output.AddRawToken(token)
}

// printPreservedNewlines prints the line breaks before rawToken that are kept: up to
// max_preserve_newlines + 1 when newlines are preserved, otherwise one between two tokens that
// are not text. It reports whether it printed any.
func (p *printer) printPreservedNewlines(rawToken *core.Token) bool {
	newlines := 0
	if rawToken.Type != tokenText && rawToken.Previous.Type != tokenText && rawToken.Newlines != 0 {
		newlines = 1
	}
	if p.preserveNewlines {
		newlines = min(rawToken.Newlines, p.maxPreserveNewlines+1)
	}
	for n := 0; n < newlines; n++ {
		p.printNewline(n > 0)
	}
	return newlines != 0
}

// traverseWhitespace turns the whitespace before rawToken into line breaks or a space. It
// reports whether there was any.
func (p *printer) traverseWhitespace(rawToken *core.Token) bool {
	if rawToken.WhitespaceBefore != "" || rawToken.Newlines != 0 {
		if !p.printPreservedNewlines(rawToken) {
			p.output.SpaceBeforeToken = true
		}
		return true
	}
	return false
}

func (p *printer) previousTokenWrapped() bool {
	return p.output.PreviousTokenWrapped
}

func (p *printer) printNewline(force bool) {
	p.output.AddNewLine(force)
}

func (p *printer) printToken(token *core.Token) {
	if token.Text != "" {
		p.output.SetIndent(p.indentLevel, p.alignmentSize)
		p.output.AddToken(token.Text)
	}
}

func (p *printer) indent() {
	p.indentLevel++
}

func (p *printer) deindent() {
	if p.indentLevel > 0 {
		p.indentLevel--
		p.output.SetIndent(p.indentLevel, p.alignmentSize)
	}
}

// getFullIndent returns the indentation string of the current level plus level.
func (p *printer) getFullIndent(level int) jsrt.String {
	level += p.indentLevel
	if level < 1 {
		return ""
	}
	return p.output.GetIndentString(level, 0)
}

// tagFrame is an open element on the tag stack, with the indentation it was opened at.
type tagFrame struct {
	parent      *tagFrame
	tag         jsrt.String
	indentLevel int
	parserToken *tagToken
}

// tagStack is the stack of open elements (TagStack in js/src/html/beautifier.js). Popping an
// element restores the printer's indentation to where the element started.
type tagStack struct {
	printer      *printer
	currentFrame *tagFrame
}

func (s *tagStack) getParserToken() *tagToken {
	if s.currentFrame == nil {
		return nil
	}
	return s.currentFrame.parserToken
}

func (s *tagStack) recordTag(parserToken *tagToken) {
	s.currentFrame = &tagFrame{
		parent:      s.currentFrame,
		tag:         parserToken.tagName,
		indentLevel: s.printer.indentLevel,
		parserToken: parserToken,
	}
}

func (s *tagStack) tryPopFrame(frame *tagFrame) *tagToken {
	if frame == nil {
		return nil
	}
	s.printer.indentLevel = frame.indentLevel
	s.currentFrame = frame.parent
	return frame.parserToken
}

// getFrame returns the innermost frame whose tag is in tagList, or nil when there is none or a
// tag of stopList comes first.
func (s *tagStack) getFrame(tagList, stopList []jsrt.String) *tagFrame {
	for frame := s.currentFrame; frame != nil; frame = frame.parent {
		if slices.Contains(tagList, frame.tag) {
			return frame
		}
		if slices.Contains(stopList, frame.tag) {
			return nil
		}
	}
	return nil
}

// tryPop closes the innermost open tag (and everything inside it) unless a tag of stopList is
// open inside it, and returns its parser token.
func (s *tagStack) tryPop(tag jsrt.String, stopList ...jsrt.String) *tagToken {
	return s.tryPopFrame(s.getFrame([]jsrt.String{tag}, stopList))
}

// indentToTag sets the indentation to that of the innermost open tag of tagList.
func (s *tagStack) indentToTag(tagList ...jsrt.String) {
	if frame := s.getFrame(tagList, nil); frame != nil {
		s.printer.indentLevel = frame.indentLevel
	}
}
