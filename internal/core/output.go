package core

import (
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// OutputLine is one line of output: its items and the bookkeeping for indentation and wrapping
// (OutputLine in js/src/core/output.js). Character counts are in UTF-16 code units.
type OutputLine struct {
	parent         *Output
	characterCount int
	// indentCount is -1 on lines whose indentation is preserved from the input.
	indentCount             int
	alignmentCount          int
	wrapPointIndex          int
	wrapPointCharacterCount int
	wrapPointIndentCount    int
	wrapPointAlignmentCount int

	items []jsrt.String
}

func newOutputLine(parent *Output) *OutputLine {
	return &OutputLine{parent: parent, indentCount: -1, wrapPointIndentCount: -1}
}

// CloneEmpty returns an empty line with the same indentation.
func (l *OutputLine) CloneEmpty() *OutputLine {
	line := newOutputLine(l.parent)
	line.SetIndent(l.indentCount, l.alignmentCount)
	return line
}

// Item returns the item at index, counting from the end when index is negative, or "" when
// there is none (where the original yields undefined).
func (l *OutputLine) Item(index int) jsrt.String {
	if index < 0 {
		index += len(l.items)
	}
	if index < 0 || index >= len(l.items) {
		return ""
	}
	return l.items[index]
}

// HasMatch reports whether any item matches pattern.
func (l *OutputLine) HasMatch(pattern *jsrt.Regexp) bool {
	for i := len(l.items) - 1; i >= 0; i-- {
		if pattern.Test(l.items[i]) {
			return true
		}
	}
	return false
}

// SetIndent sets the indentation and alignment of an empty line.
func (l *OutputLine) SetIndent(indent, alignment int) {
	if l.IsEmpty() {
		l.indentCount = indent
		l.alignmentCount = alignment
		l.characterCount = l.parent.GetIndentSize(l.indentCount, l.alignmentCount)
	}
}

func (l *OutputLine) setWrapPoint() {
	if l.parent.wrapLineLength != 0 {
		l.wrapPointIndex = len(l.items)
		l.wrapPointCharacterCount = l.characterCount
		l.wrapPointIndentCount = l.parent.NextLine.indentCount
		l.wrapPointAlignmentCount = l.parent.NextLine.alignmentCount
	}
}

func (l *OutputLine) shouldWrap() bool {
	return l.wrapPointIndex != 0 &&
		l.characterCount > l.parent.wrapLineLength &&
		l.wrapPointCharacterCount > l.parent.NextLine.characterCount
}

func (l *OutputLine) allowWrap() bool {
	if !l.shouldWrap() {
		return false
	}
	l.parent.AddNewLine(false)
	next := l.parent.CurrentLine
	next.SetIndent(l.wrapPointIndentCount, l.wrapPointAlignmentCount)
	// In raw mode next is this same line. The wrap point may lie past the items left after a
	// trim; the slices clamp as in JavaScript.
	next.items = append([]jsrt.String(nil), l.items[min(l.wrapPointIndex, len(l.items)):]...)
	l.items = l.items[:min(l.wrapPointIndex, len(l.items))]

	next.characterCount += l.characterCount - l.wrapPointCharacterCount
	l.characterCount = l.wrapPointCharacterCount

	if len(next.items) > 0 && next.items[0] == " " {
		next.items = next.items[1:]
		next.characterCount--
	}
	return true
}

// IsEmpty reports whether the line has no items.
func (l *OutputLine) IsEmpty() bool {
	return len(l.items) == 0
}

// Last returns the last item, or "" when the line is empty.
func (l *OutputLine) Last() jsrt.String {
	if l.IsEmpty() {
		return ""
	}
	return l.items[len(l.items)-1]
}

// Push appends an item. An item containing a line break restarts the character count after it.
func (l *OutputLine) Push(item jsrt.String) {
	l.items = append(l.items, item)
	if lastNewline := item.LastIndexOf("\n"); lastNewline != -1 {
		l.characterCount = item.Len() - lastNewline
	} else {
		l.characterCount += item.Len()
	}
}

// Pop removes and returns the last item, or "" when the line is empty.
func (l *OutputLine) Pop() jsrt.String {
	if l.IsEmpty() {
		return ""
	}
	item := l.items[len(l.items)-1]
	l.items = l.items[:len(l.items)-1]
	l.characterCount -= item.Len()
	return item
}

func (l *OutputLine) removeIndent() {
	if l.indentCount > 0 {
		l.indentCount--
		l.characterCount -= l.parent.indentSize
	}
}

func (l *OutputLine) removeWrapIndent() {
	if l.wrapPointIndentCount > 0 {
		l.wrapPointIndentCount--
	}
}

// Trim removes trailing single-space items.
func (l *OutputLine) Trim() {
	for l.Last() == " " {
		l.items = l.items[:len(l.items)-1]
		l.characterCount--
	}
}

// String returns the line's text with its indentation.
func (l *OutputLine) String() jsrt.String {
	if l.IsEmpty() {
		if l.parent.indentEmptyLines {
			return l.parent.GetIndentString(l.indentCount, 0)
		}
		return ""
	}
	var b strings.Builder
	b.WriteString(string(l.parent.GetIndentString(l.indentCount, l.alignmentCount)))
	for _, item := range l.items {
		b.WriteString(string(item))
	}
	return jsrt.String(b.String())
}

// indentStringCache builds indentation strings (IndentStringCache in js/src/core/output.js).
type indentStringCache struct {
	cache        []jsrt.String
	indentSize   int
	indentString jsrt.String
	baseString   jsrt.String
	// err is the RangeError the original throws while building an indentation string.
	err error
}

func newIndentStringCache(options *Options, baseIndentString jsrt.String) (*indentStringCache, error) {
	c := &indentStringCache{cache: []jsrt.String{""}, indentSize: options.IndentSize, indentString: options.IndentChar}
	if !options.IndentWithTabs {
		s, err := jsrt.ArrayJoinRepeat(options.IndentSize+1, options.IndentChar)
		if err != nil {
			return nil, err
		}
		c.indentString = s
	}

	if options.IndentLevel > 0 {
		s, err := jsrt.ArrayJoinRepeat(options.IndentLevel+1, c.indentString)
		if err != nil {
			return nil, err
		}
		baseIndentString = s
	}

	c.baseString = baseIndentString
	return c, nil
}

func (c *indentStringCache) getIndentSize(indent, column int) int {
	result := c.baseString.Len()
	if indent < 0 {
		result = 0
	}
	result += indent * c.indentSize
	result += column
	return result
}

func (c *indentStringCache) getIndentString(indentLevel, column int) jsrt.String {
	result := c.baseString
	if indentLevel < 0 {
		indentLevel = 0
		result = ""
	}
	column += indentLevel * c.indentSize
	if column < 0 {
		// The original reads its cache array at a negative index and appends undefined.
		return result + "undefined"
	}
	return result + c.column(column)
}

// column returns the indentation for a column, filling the cache column by column as the
// original's __ensure_cache does.
func (c *indentStringCache) column(column int) jsrt.String {
	for column >= len(c.cache) && c.err == nil {
		c.addColumn()
	}
	if column >= len(c.cache) {
		return ""
	}
	return c.cache[column]
}

// addColumn is __add_column: as many indent strings as fit in the next column, then spaces.
func (c *indentStringCache) addColumn() {
	column := len(c.cache)
	var result jsrt.String
	if c.indentSize != 0 && column >= c.indentSize {
		indent := floorDiv(column, c.indentSize)
		column -= indent * c.indentSize
		s, err := jsrt.ArrayJoinRepeat(indent+1, c.indentString)
		if err != nil {
			c.setErr(err)
			return
		}
		result = s
	}
	if column != 0 {
		s, err := jsrt.ArrayJoinRepeat(column+1, " ")
		if err != nil {
			c.setErr(err)
			return
		}
		result += s
	}
	c.cache = append(c.cache, result)
}

func (c *indentStringCache) setErr(err error) {
	if c.err == nil {
		c.err = err
	}
}

// floorDiv is Math.floor(a / b) for b != 0.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Output collects beautified text line by line (Output in js/src/core/output.js).
//
// An indentation the original fails to build (RangeError with a negative indent_size and an
// alignment) is reported by GetCode, which every beautifier calls last.
type Output struct {
	indentCache *indentStringCache
	// Raw suppresses new lines from AddNewLine while keeping its result.
	Raw              bool
	endWithNewline   bool
	indentSize       int
	wrapLineLength   int
	indentEmptyLines bool
	lines            []*OutputLine

	PreviousLine *OutputLine
	CurrentLine  *OutputLine
	// NextLine holds the indentation the next new line gets.
	NextLine *OutputLine

	SpaceBeforeToken     bool
	NonBreakingSpace     bool
	PreviousTokenWrapped bool
}

// NewOutput returns an empty output. baseIndentString prefixes every line unless
// options.IndentLevel replaces it. The error is the RangeError the original throws for an
// indent_size or indent_level it cannot build an indentation string for.
func NewOutput(options *Options, baseIndentString jsrt.String) (*Output, error) {
	cache, err := newIndentStringCache(options, baseIndentString)
	if err != nil {
		return nil, err
	}
	o := &Output{
		indentCache:      cache,
		endWithNewline:   options.EndWithNewline,
		indentSize:       options.IndentSize,
		wrapLineLength:   options.WrapLineLength,
		indentEmptyLines: options.IndentEmptyLines,
	}
	o.NextLine = newOutputLine(o)
	o.addOutputLine()
	return o, nil
}

func (o *Output) addOutputLine() {
	o.PreviousLine = o.CurrentLine
	o.CurrentLine = o.NextLine.CloneEmpty()
	o.lines = append(o.lines, o.CurrentLine)
}

// GetLineNumber returns the number of lines.
func (o *Output) GetLineNumber() int {
	return len(o.lines)
}

// GetIndentString returns the indentation for an indent level plus a column of alignment.
func (o *Output) GetIndentString(indent, column int) jsrt.String {
	return o.indentCache.getIndentString(indent, column)
}

// GetIndentSize returns the width of that indentation.
func (o *Output) GetIndentSize(indent, column int) int {
	return o.indentCache.getIndentSize(indent, column)
}

// IsEmpty reports whether nothing has been output.
func (o *Output) IsEmpty() bool {
	return o.PreviousLine == nil && o.CurrentLine.IsEmpty()
}

// AddNewLine starts a new line, unless the output is empty or (when not forced) the current
// line is already empty. It reports whether a line break was due, even in Raw mode.
func (o *Output) AddNewLine(forceNewline bool) bool {
	// Never a newline at the start of the output; otherwise only if forced or after content.
	if o.IsEmpty() || (!forceNewline && o.JustAddedNewline()) {
		return false
	}
	if !o.Raw {
		o.addOutputLine()
	}
	return true
}

// GetCode returns the text with lines joined by eol: trailing spaces and empty lines removed,
// line breaks at the end of the last item dropped, and a final line break when
// end_with_newline is set.
func (o *Output) GetCode(eol jsrt.String) (jsrt.String, error) {
	o.Trim(true)

	// The last token's text may end with line breaks.
	if lastItem := o.CurrentLine.Pop(); lastItem != "" {
		if strings.HasSuffix(string(lastItem), "\n") {
			lastItem = jsrt.String(strings.TrimRight(string(lastItem), "\n"))
		}
		o.CurrentLine.Push(lastItem)
	}

	if o.endWithNewline {
		o.addOutputLine()
	}

	var b strings.Builder
	for i, line := range o.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(line.String()))
	}
	if o.indentCache.err != nil {
		return "", o.indentCache.err
	}
	sweetCode := b.String()
	if eol != "\n" {
		sweetCode = strings.ReplaceAll(sweetCode, "\n", string(eol))
	}
	return jsrt.String(sweetCode), nil
}

// SetWrapPoint marks the current position as a place to wrap the line.
func (o *Output) SetWrapPoint() {
	o.CurrentLine.setWrapPoint()
}

// SetIndent sets the indentation of the next line and of the current one if it is empty. It
// reports false (and leaves the first line unindented) at the start of the output.
func (o *Output) SetIndent(indent, alignment int) bool {
	// The next line stores the alignment values.
	o.NextLine.SetIndent(indent, alignment)

	// Never indent the first line of the output.
	if len(o.lines) > 1 {
		o.CurrentLine.SetIndent(indent, alignment)
		return true
	}

	o.CurrentLine.SetIndent(0, 0)
	return false
}

// AddRawToken outputs a token as it was in the input: its line breaks, its whitespace and its
// text, with the indentation preserved.
func (o *Output) AddRawToken(token *Token) {
	for x := 0; x < token.Newlines; x++ {
		o.addOutputLine()
	}
	o.CurrentLine.SetIndent(-1, 0)
	o.CurrentLine.Push(token.WhitespaceBefore)
	o.CurrentLine.Push(token.Text)
	o.SpaceBeforeToken = false
	o.NonBreakingSpace = false
	o.PreviousTokenWrapped = false
}

// AddToken outputs printableToken, preceded by a space when SpaceBeforeToken is set, and wraps
// the line if it got too long.
func (o *Output) AddToken(printableToken jsrt.String) {
	o.addSpaceBeforeToken()
	o.CurrentLine.Push(printableToken)
	o.SpaceBeforeToken = false
	o.NonBreakingSpace = false
	o.PreviousTokenWrapped = o.CurrentLine.allowWrap()
}

func (o *Output) addSpaceBeforeToken() {
	if o.SpaceBeforeToken && !o.JustAddedNewline() {
		if !o.NonBreakingSpace {
			o.SetWrapPoint()
		}
		o.CurrentLine.Push(" ")
	}
}

// RemoveIndent removes one level of indentation from the lines starting at index.
func (o *Output) RemoveIndent(index int) {
	for ; index < len(o.lines); index++ {
		o.lines[index].removeIndent()
	}
	o.CurrentLine.removeWrapIndent()
}

// Trim removes trailing spaces from the current line and, when eatNewlines, the empty lines at
// the end of the output.
func (o *Output) Trim(eatNewlines bool) {
	o.CurrentLine.Trim()

	for eatNewlines && len(o.lines) > 1 && o.CurrentLine.IsEmpty() {
		o.lines = o.lines[:len(o.lines)-1]
		o.CurrentLine = o.lines[len(o.lines)-1]
		o.CurrentLine.Trim()
	}

	o.PreviousLine = nil
	if len(o.lines) > 1 {
		o.PreviousLine = o.lines[len(o.lines)-2]
	}
}

// JustAddedNewline reports whether the current line is empty.
func (o *Output) JustAddedNewline() bool {
	return o.CurrentLine.IsEmpty()
}

// JustAddedBlankline reports whether the output is empty or ends with an empty line.
func (o *Output) JustAddedBlankline() bool {
	return o.IsEmpty() || (o.CurrentLine.IsEmpty() && o.PreviousLine.IsEmpty())
}

// EnsureEmptyLineAbove inserts an empty line above the current one, unless one is already there
// above the lines that start with startsWith or end with endsWith.
func (o *Output) EnsureEmptyLineAbove(startsWith, endsWith jsrt.String) {
	for index := len(o.lines) - 2; index >= 0; index-- {
		potentialEmptyLine := o.lines[index]
		if potentialEmptyLine.IsEmpty() {
			break
		} else if potentialEmptyLine.Item(0).IndexOf(startsWith) != 0 && potentialEmptyLine.Item(-1) != endsWith {
			o.lines = append(o.lines[:index+1], append([]*OutputLine{newOutputLine(o)}, o.lines[index+1:]...)...)
			o.PreviousLine = o.lines[len(o.lines)-2]
			break
		}
	}
}
