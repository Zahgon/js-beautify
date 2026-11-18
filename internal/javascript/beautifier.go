// Package javascript is a port of js-beautify's JavaScript beautifier (js/src/javascript).
package javascript

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// mode is the kind of construct a frame of the beautifier's stack formats.
type mode string

const (
	modeBlockStatement mode = "BlockStatement"
	modeStatement      mode = "Statement"
	modeObjectLiteral  mode = "ObjectLiteral"
	modeArrayLiteral   mode = "ArrayLiteral"
	modeForInitializer mode = "ForInitializer"
	modeConditional    mode = "Conditional"
	modeExpression     mode = "Expression"
)

// specialWords are the words after which a statement continues on the same line.
var specialWords = []jsrt.String{"case", "return", "do", "if", "throw", "else", "await", "break", "continue", "async"}

// newlineRestrictedTokens never get a line break between them and the expression after them.
var newlineRestrictedTokens = []jsrt.String{"async", "break", "continue", "return", "throw", "yield"}

var (
	leadingWhitespace = jsrt.MustCompile(`^\s+`, "g")
	leadingIndent     = jsrt.MustCompile(`^[\t ]*`, "")
)

func inList(s jsrt.String, list ...jsrt.String) bool {
	for _, x := range list {
		if s == x {
			return true
		}
	}
	return false
}

func inTypes(typ string, types ...string) bool {
	for _, x := range types {
		if typ == x {
			return true
		}
	}
	return false
}

func reservedWord(token *core.Token, word jsrt.String) bool {
	return token != nil && token.Type == tkReserved && token.Text == word
}

func reservedArray(token *core.Token, words ...jsrt.String) bool {
	return token != nil && token.Type == tkReserved && inList(token.Text, words...)
}

func isArray(m mode) bool {
	return m == modeArrayLiteral
}

func isExpression(m mode) bool {
	return m == modeExpression || m == modeForInitializer || m == modeConditional
}

// isPositionable reports whether s is one of the operators operator_position applies to.
func isPositionable(s jsrt.String) bool {
	return inList(s, positionableOperators...)
}

// isDigits is s.match('^[0-9]+$').
func isDigits(s jsrt.String) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// splitLinebreaks splits s at its line breaks; a final empty piece is dropped.
func splitLinebreaks(s jsrt.String) []jsrt.String {
	s = allLineBreaks.Replace(s, "\n")
	var out []jsrt.String
	start := 0
	for idx := s.IndexOfFrom("\n", start); idx != -1; idx = s.IndexOfFrom("\n", start) {
		out = append(out, s.Substring(start, idx))
		start = idx + 1
	}
	if start < s.Len() {
		out = append(out, s.SubstringFrom(start))
	}
	return out
}

func allLinesStartWith(lines []jsrt.String, c jsrt.String) bool {
	for _, line := range lines {
		if line.Trim().CharAt(0) != c {
			return false
		}
	}
	return true
}

func eachLineMatchesIndent(lines []jsrt.String, indent jsrt.String) bool {
	for _, line := range lines {
		// Empty lines pass.
		if line != "" && line.IndexOf(indent) != 0 {
			return false
		}
	}
	return true
}

// flags is one frame of the beautifier's stack of open constructs.
type flags struct {
	mode     mode
	parent   *flags
	lastWord jsrt.String
	// lastToken is the last token handled in this frame.
	lastToken *core.Token

	declarationStatement  bool
	declarationAssignment bool
	multilineFrame        bool
	inlineFrame           bool
	ifBlock               bool
	elseBlock             bool
	classStartBlock       bool // class A { INSIDE HERE } or class B extends C { INSIDE HERE }
	doBlock               bool
	doWhile               bool
	importBlock           bool
	inCaseStatement       bool // switch(..){ INSIDE HERE }
	inCase                bool // on the line of "case 0:"
	caseBody              bool // the indented case-action block
	caseBlock             bool // the case-action block is wrapped in {}

	indentationLevel int
	alignment        int
	lineIndentLevel  int
	startLineIndex   int
	ternaryDepth     int
}

// Beautifier formats one JavaScript source (Beautifier in js/src/javascript/beautifier.js).
//
// It reads the token stream forward only, keeping a stack of frames (flags) for the open
// blocks, expressions and statements. A '{' starts a BlockStatement frame that a later ':' can
// turn into an ObjectLiteral, and statements close when their container does, which makes it
// tolerant of syntax errors.
type Beautifier struct {
	sourceText jsrt.String
	options    *Options

	output        *core.Output
	tokens        *core.TokenStream
	lastLastText  jsrt.String
	flags         *flags
	previousFlags *flags
	flagStore     []*flags
}

// NewBeautifier returns a beautifier for sourceText; options are read and validated here.
func NewBeautifier(sourceText jsrt.String, options map[string]any) (*Beautifier, error) {
	o, err := NewOptions(options)
	if err != nil {
		return nil, err
	}
	return &Beautifier{sourceText: sourceText, options: o}, nil
}

func (b *Beautifier) createFlags(flagsBase *flags, m mode) *flags {
	nextIndentLevel := 0
	if flagsBase != nil {
		nextIndentLevel = flagsBase.indentationLevel
		if !b.output.JustAddedNewline() && flagsBase.lineIndentLevel > nextIndentLevel {
			nextIndentLevel = flagsBase.lineIndentLevel
		}
	}

	next := &flags{
		mode:             m,
		parent:           flagsBase,
		indentationLevel: nextIndentLevel,
		lineIndentLevel:  nextIndentLevel,
		startLineIndex:   b.output.GetLineNumber(),
	}
	if flagsBase != nil {
		next.lastToken = flagsBase.lastToken
		next.lastWord = flagsBase.lastWord
		next.lineIndentLevel = flagsBase.lineIndentLevel
	} else {
		next.lastToken = core.NewToken(tkStartBlock, "", 0, "")
	}
	return next
}

func (b *Beautifier) reset(sourceText jsrt.String) error {
	baseIndentString := leadingIndent.Exec(sourceText).Text()

	b.lastLastText = ""
	output, err := core.NewOutput(b.options.Options, baseIndentString)
	if err != nil {
		return err
	}
	b.output = output

	// Testing the preserve directive starts with raw output.
	b.output.Raw = b.options.TestOutputRaw

	b.flagStore = nil
	b.setMode(modeBlockStatement)
	b.tokens = NewTokenizer(sourceText, b.options).Tokenize()
	return nil
}

// Beautify returns the formatted source. The error is the RangeError the original throws for
// an indentation it cannot build (a negative indent_size, for instance).
func (b *Beautifier) Beautify() (jsrt.String, error) {
	// When disabled, the input comes back unchanged.
	if b.options.Disabled {
		return b.sourceText, nil
	}

	if err := b.reset(b.sourceText); err != nil {
		return "", err
	}

	eol := b.options.EOL
	if eol == "auto" {
		eol = "\n"
		if m := lineBreak.Exec(b.sourceText); m != nil {
			eol = m.Text()
		}
	}

	for currentToken := b.tokens.Next(); currentToken != nil; currentToken = b.tokens.Next() {
		b.handleToken(currentToken, false)

		b.lastLastText = b.flags.lastToken.Text
		b.flags.lastToken = currentToken
	}

	return b.output.GetCode(eol)
}

func (b *Beautifier) handleToken(currentToken *core.Token, preserveStatementFlags bool) {
	switch currentToken.Type {
	case tkStartExpr:
		b.handleStartExpr(currentToken)
	case tkEndExpr:
		b.handleEndExpr(currentToken)
	case tkStartBlock:
		b.handleStartBlock(currentToken)
	case tkEndBlock:
		b.handleEndBlock(currentToken)
	case tkWord, tkReserved:
		b.handleWord(currentToken)
	case tkSemicolon:
		b.handleSemicolon(currentToken)
	case tkString:
		b.handleString(currentToken)
	case tkEquals:
		b.handleEquals(currentToken)
	case tkOperator:
		b.handleOperator(currentToken)
	case tkComma:
		b.handleComma(currentToken)
	case tkBlockComment:
		b.handleBlockComment(currentToken, preserveStatementFlags)
	case tkComment:
		b.handleComment(currentToken, preserveStatementFlags)
	case tkDot:
		b.handleDot(currentToken)
	case tkEOF:
		b.handleEOF(currentToken)
	default:
		b.handleUnknown(currentToken, preserveStatementFlags)
	}
}

func (b *Beautifier) handleWhitespaceAndComments(currentToken *core.Token, preserveStatementFlags bool) {
	newlines := currentToken.Newlines
	keepWhitespace := b.options.KeepArrayIndentation && isArray(b.flags.mode)

	if currentToken.CommentsBefore != nil {
		// Inline comments are formatted as though they were not there, and so are unknown
		// tokens.
		for commentToken := currentToken.CommentsBefore.Next(); commentToken != nil; commentToken = currentToken.CommentsBefore.Next() {
			b.handleWhitespaceAndComments(commentToken, preserveStatementFlags)
			b.handleToken(commentToken, preserveStatementFlags)
		}
	}

	if keepWhitespace {
		for i := 0; i < newlines; i++ {
			b.printNewline(i > 0, preserveStatementFlags)
		}
		return
	}

	if b.options.MaxPreserveNewlines != 0 && newlines > b.options.MaxPreserveNewlines {
		newlines = b.options.MaxPreserveNewlines
	}
	if b.options.PreserveNewlines && newlines > 1 {
		b.printNewline(false, preserveStatementFlags)
		for j := 1; j < newlines; j++ {
			b.printNewline(true, preserveStatementFlags)
		}
	}
}

func (b *Beautifier) allowWrapOrPreservedNewline(currentToken *core.Token, forceLinewrap bool) {
	// Never wrap the first token on a line.
	if b.output.JustAddedNewline() {
		return
	}

	shouldPreserveOrForce := (b.options.PreserveNewlines && currentToken.Newlines != 0) || forceLinewrap
	lastIsPositionable := isPositionable(b.flags.lastToken.Text)
	currentIsPositionable := isPositionable(currentToken.Text)

	if lastIsPositionable || currentIsPositionable {
		shouldPrintOperatorNewline := (lastIsPositionable &&
			(b.options.OperatorPosition == operatorBeforeNewline || b.options.OperatorPosition == operatorPreserveNewline)) ||
			currentIsPositionable
		shouldPreserveOrForce = shouldPreserveOrForce && shouldPrintOperatorNewline
	}

	if shouldPreserveOrForce {
		b.printNewline(false, true)
	} else if b.options.WrapLineLength != 0 {
		if reservedArray(b.flags.lastToken, newlineRestrictedTokens...) {
			// No line break between these words and the expression after them.
			return
		}
		b.output.SetWrapPoint()
	}
}

func (b *Beautifier) printNewline(forceNewline, preserveStatementFlags bool) {
	if !preserveStatementFlags {
		last := b.flags.lastToken
		if last.Text != ";" && last.Text != "," && last.Text != "=" &&
			(last.Type != tkOperator || last.Text == "--" || last.Text == "++") {
			nextToken := b.tokens.Peek(0)
			for b.flags.mode == modeStatement &&
				!(b.flags.ifBlock && reservedWord(nextToken, "else")) &&
				!b.flags.doBlock {
				b.restoreMode()
			}
		}
	}

	if b.output.AddNewLine(forceNewline) {
		b.flags.multilineFrame = true
	}
}

func (b *Beautifier) printTokenLineIndentation(currentToken *core.Token) {
	if !b.output.JustAddedNewline() {
		return
	}
	if b.options.KeepArrayIndentation && currentToken.Newlines != 0 &&
		(currentToken.Text == "[" || isArray(b.flags.mode)) {
		b.output.CurrentLine.SetIndent(-1, 0)
		b.output.CurrentLine.Push(currentToken.WhitespaceBefore)
		b.output.SpaceBeforeToken = false
	} else if b.output.SetIndent(b.flags.indentationLevel, b.flags.alignment) {
		b.flags.lineIndentLevel = b.flags.indentationLevel
	}
}

func (b *Beautifier) printToken(currentToken *core.Token) {
	if b.output.Raw {
		b.output.AddRawToken(currentToken)
		return
	}

	if b.options.CommaFirst && currentToken.Previous != nil && currentToken.Previous.Type == tkComma &&
		b.output.JustAddedNewline() {
		if b.output.PreviousLine.Last() == "," {
			popped := b.output.PreviousLine.Pop()
			// A comma that was already at the start of its line is pulled back onto that line
			// and the indentation is printed again.
			if b.output.PreviousLine.IsEmpty() {
				b.output.PreviousLine.Push(popped)
				b.output.Trim(true)
				b.output.CurrentLine.Pop()
				b.output.Trim(false)
			}

			// The comma goes in front of the next token.
			b.printTokenLineIndentation(currentToken)
			b.output.AddToken(",")
			b.output.SpaceBeforeToken = true
		}
	}

	b.printTokenLineIndentation(currentToken)
	b.output.NonBreakingSpace = true
	b.output.AddToken(currentToken.Text)
	if b.output.PreviousTokenWrapped {
		b.flags.multilineFrame = true
	}
}

func (b *Beautifier) indent() {
	b.flags.indentationLevel++
	b.output.SetIndent(b.flags.indentationLevel, b.flags.alignment)
}

func (b *Beautifier) deindent() {
	if b.flags.indentationLevel > 0 &&
		(b.flags.parent == nil || b.flags.indentationLevel > b.flags.parent.indentationLevel) {
		b.flags.indentationLevel--
		b.output.SetIndent(b.flags.indentationLevel, b.flags.alignment)
	}
}

func (b *Beautifier) setMode(m mode) {
	if b.flags != nil {
		b.flagStore = append(b.flagStore, b.flags)
		b.previousFlags = b.flags
	} else {
		b.previousFlags = b.createFlags(nil, m)
	}

	b.flags = b.createFlags(b.previousFlags, m)
	b.output.SetIndent(b.flags.indentationLevel, b.flags.alignment)
}

func (b *Beautifier) restoreMode() {
	if n := len(b.flagStore); n > 0 {
		b.previousFlags = b.flags
		b.flags = b.flagStore[n-1]
		b.flagStore = b.flagStore[:n-1]
		if b.previousFlags.mode == modeStatement {
			b.removeRedundantIndentation(b.previousFlags)
		}
		b.output.SetIndent(b.flags.indentationLevel, b.flags.alignment)
	}
}

// removeRedundantIndentation removes one indent from each line of a frame that did not span
// several lines. It can make lines wrap too soon, as wrap points were computed with the indent,
// which is minor compared to ugly indentation.
func (b *Beautifier) removeRedundantIndentation(frame *flags) {
	if frame.multilineFrame || frame.mode == modeForInitializer || frame.mode == modeConditional {
		return
	}
	b.output.RemoveIndent(frame.startLineIndex)
}

func (b *Beautifier) startOfObjectProperty() bool {
	return b.flags.parent.mode == modeObjectLiteral && b.flags.mode == modeStatement &&
		((b.flags.lastToken.Text == ":" && b.flags.ternaryDepth == 0) || reservedArray(b.flags.lastToken, "get", "set"))
}

func (b *Beautifier) startOfStatement(currentToken *core.Token) bool {
	last := b.flags.lastToken
	start := reservedArray(last, "var", "let", "const") && currentToken.Type == tkWord ||
		reservedWord(last, "do") ||
		(!(b.flags.parent.mode == modeObjectLiteral && b.flags.mode == modeStatement)) &&
			reservedArray(last, newlineRestrictedTokens...) && currentToken.Newlines == 0 ||
		reservedWord(last, "else") && !(reservedWord(currentToken, "if") && currentToken.CommentsBefore == nil) ||
		(last.Type == tkEndExpr && (b.previousFlags.mode == modeForInitializer || b.previousFlags.mode == modeConditional)) ||
		(last.Type == tkWord && b.flags.mode == modeBlockStatement &&
			!b.flags.inCase &&
			!(currentToken.Text == "--" || currentToken.Text == "++") &&
			b.lastLastText != "function" &&
			currentToken.Type != tkWord && currentToken.Type != tkReserved) ||
		(b.flags.mode == modeObjectLiteral &&
			((last.Text == ":" && b.flags.ternaryDepth == 0) || reservedArray(last, "get", "set")))

	if !start {
		return false
	}

	b.setMode(modeStatement)
	b.indent()

	b.handleWhitespaceAndComments(currentToken, true)

	// Issue #276: a statement starting with if, for, while or do goes to a new line:
	// if (a) if (b) if(c) d(); else e(); else f();
	if !b.startOfObjectProperty() {
		b.allowWrapOrPreservedNewline(currentToken, reservedArray(currentToken, "do", "for", "if", "while"))
	}
	return true
}

func (b *Beautifier) handleStartExpr(currentToken *core.Token) {
	// The conditional starts the statement if appropriate.
	if !b.startOfStatement(currentToken) {
		b.handleWhitespaceAndComments(currentToken, false)
	}

	nextMode := modeExpression
	if currentToken.Text == "[" {
		if b.flags.lastToken.Type == tkWord || b.flags.lastToken.Text == ")" {
			// An index: a[x], fn()[x].
			if reservedArray(b.flags.lastToken, lineStarters...) {
				b.output.SpaceBeforeToken = true
			}
			b.printToken(currentToken)
			b.setMode(nextMode)
			b.indent()
			if b.options.SpaceInParen {
				b.output.SpaceBeforeToken = true
			}
			return
		}

		nextMode = modeArrayLiteral
		if isArray(b.flags.mode) {
			if b.flags.lastToken.Text == "[" ||
				(b.flags.lastToken.Text == "," && (b.lastLastText == "]" || b.lastLastText == "}")) {
				// ], [ and }, [ go to a new line.
				if !b.options.KeepArrayIndentation {
					b.printNewline(false, false)
				}
			}
		}

		if !inTypes(b.flags.lastToken.Type, tkStartExpr, tkEndExpr, tkWord, tkOperator, tkDot) {
			b.output.SpaceBeforeToken = true
		}
	} else {
		switch b.flags.lastToken.Type {
		case tkReserved:
			switch {
			case b.flags.lastToken.Text == "for":
				b.output.SpaceBeforeToken = b.options.SpaceBeforeConditional
				nextMode = modeForInitializer
			case inList(b.flags.lastToken.Text, "if", "while", "switch"):
				b.output.SpaceBeforeToken = b.options.SpaceBeforeConditional
				nextMode = modeConditional
			case inList(b.flags.lastWord, "await", "async"):
				// A space between await and an IIFE, or async and an arrow function.
				b.output.SpaceBeforeToken = true
			case b.flags.lastToken.Text == "import" && currentToken.WhitespaceBefore == "":
				b.output.SpaceBeforeToken = false
			case inList(b.flags.lastToken.Text, lineStarters...) || b.flags.lastToken.Text == "catch":
				b.output.SpaceBeforeToken = true
			}
		case tkEquals, tkOperator:
			// Keeps line breaks like
			// a = (b &&
			//     (c || d));
			if !b.startOfObjectProperty() {
				b.allowWrapOrPreservedNewline(currentToken, false)
			}
		case tkWord:
			b.output.SpaceBeforeToken = false

			// function name() vs function name (), and the same for function*, async and the
			// method shorthand of objects and classes: { a() {} } vs { a () {} }.
			peekBackTwo := b.tokens.Peek(-3)
			if b.options.SpaceAfterNamedFunction && peekBackTwo != nil {
				// The stream is past the current token, so Peek(-1) is the current token.
				peekBackThree := b.tokens.Peek(-4)
				if reservedArray(peekBackTwo, "async", "function") ||
					(peekBackTwo.Text == "*" && reservedArray(peekBackThree, "async", "function")) {
					b.output.SpaceBeforeToken = true
				} else if b.flags.mode == modeObjectLiteral {
					if (peekBackTwo.Text == "{" || peekBackTwo.Text == ",") ||
						(peekBackTwo.Text == "*" && (peekBackThree.Text == "{" || peekBackThree.Text == ",")) {
						b.output.SpaceBeforeToken = true
					}
				} else if b.flags.parent != nil && b.flags.parent.classStartBlock {
					b.output.SpaceBeforeToken = true
				}
			}
		default:
			// Keeps wrapped arrow functions:
			// a.b('c',
			//     () => d.e
			// )
			b.allowWrapOrPreservedNewline(currentToken, false)
		}

		// function() vs function (), yield*() vs yield* (), function*() vs function* ()
		if (b.flags.lastToken.Type == tkReserved && (b.flags.lastWord == "function" || b.flags.lastWord == "typeof")) ||
			(b.flags.lastToken.Text == "*" &&
				(inList(b.lastLastText, "function", "yield") ||
					(b.flags.mode == modeObjectLiteral && inList(b.lastLastText, "{", ",")))) {
			b.output.SpaceBeforeToken = b.options.SpaceAfterAnonFunction
		}
	}

	if b.flags.lastToken.Text == ";" || b.flags.lastToken.Type == tkStartBlock {
		b.printNewline(false, false)
	} else if inTypes(b.flags.lastToken.Type, tkEndExpr, tkStartExpr, tkEndBlock, tkComma) || b.flags.lastToken.Text == "." {
		// (( )( ][ ]( .( keep their line breaks.
		b.allowWrapOrPreservedNewline(currentToken, currentToken.Newlines != 0)
	}

	b.printToken(currentToken)
	b.setMode(nextMode)
	if b.options.SpaceInParen {
		b.output.SpaceBeforeToken = true
	}

	// A line break inside an expression is always indented.
	b.indent()
}

func (b *Beautifier) handleEndExpr(currentToken *core.Token) {
	// Statements inside expressions are not valid syntax, but they all close with their
	// container.
	for b.flags.mode == modeStatement {
		b.restoreMode()
	}

	b.handleWhitespaceAndComments(currentToken, false)

	if b.flags.multilineFrame {
		b.allowWrapOrPreservedNewline(currentToken,
			currentToken.Text == "]" && isArray(b.flags.mode) && !b.options.KeepArrayIndentation)
	}

	if b.options.SpaceInParen {
		if b.flags.lastToken.Type == tkStartExpr && !b.options.SpaceInEmptyParen {
			// No space inside empty () and [] (#320).
			b.output.Trim(false)
			b.output.SpaceBeforeToken = false
		} else {
			b.output.SpaceBeforeToken = true
		}
	}
	b.deindent()
	b.printToken(currentToken)
	b.restoreMode()

	b.removeRedundantIndentation(b.previousFlags)

	// do {} while () needs no statement after it.
	if b.flags.doWhile && b.previousFlags.mode == modeConditional {
		b.previousFlags.mode = modeExpression
		b.flags.doBlock = false
		b.flags.doWhile = false
	}
}

func (b *Beautifier) handleStartBlock(currentToken *core.Token) {
	b.handleWhitespaceAndComments(currentToken, false)

	// Decide whether this is an object literal.
	nextToken := b.tokens.Peek(0)
	secondToken := b.tokens.Peek(1)
	switch {
	case b.flags.lastWord == "switch" && b.flags.lastToken.Type == tkEndExpr:
		b.setMode(modeBlockStatement)
		b.flags.inCaseStatement = true
	case b.flags.caseBody:
		b.setMode(modeBlockStatement)
	case secondToken != nil &&
		((inList(secondToken.Text, ":", ",") && inTypes(nextToken.Type, tkString, tkWord, tkReserved)) ||
			(inList(nextToken.Text, "get", "set", "...") && inTypes(secondToken.Type, tkWord, tkReserved))):
		// TypeScript is not supported, but class and interface bodies are kept working.
		if inList(b.lastLastText, "class", "interface") && !inList(secondToken.Text, ":", ",") {
			b.setMode(modeBlockStatement)
		} else {
			b.setMode(modeObjectLiteral)
		}
	case b.flags.lastToken.Type == tkOperator && b.flags.lastToken.Text == "=>":
		// The body of an arrow function.
		b.setMode(modeBlockStatement)
	case inTypes(b.flags.lastToken.Type, tkEquals, tkStartExpr, tkComma, tkOperator) ||
		reservedArray(b.flags.lastToken, "return", "throw", "import", "default"):
		// Shorthand methods are hard to detect by scanning forward, so the context decides:
		// a block that is returned, imported, exported as default, passed as an argument or
		// assigned is an object literal.
		b.setMode(modeObjectLiteral)
	default:
		b.setMode(modeBlockStatement)
	}

	if reservedArray(b.flags.lastToken.Previous, "class", "extends") {
		b.flags.classStartBlock = true
	}

	emptyBraces := nextToken.CommentsBefore == nil && nextToken.Text == "}"
	emptyAnonymousFunction := emptyBraces && b.flags.lastWord == "function" &&
		b.flags.lastToken.Type == tkEndExpr

	if b.options.BracePreserveInline {
		// The block stays inline unless a line break is wanted somewhere inside it.
		b.flags.inlineFrame = true
		for index := 0; ; {
			index++
			checkToken := b.tokens.Peek(index - 1)
			if checkToken.Newlines != 0 {
				b.flags.inlineFrame = false
				break
			}
			if checkToken.Type == tkEOF || (checkToken.Type == tkEndBlock && checkToken.Opened == currentToken) {
				break
			}
		}
	}

	if (b.options.BraceStyle == "expand" || (b.options.BraceStyle == "none" && currentToken.Newlines != 0)) &&
		!b.flags.inlineFrame {
		if b.flags.lastToken.Type != tkOperator &&
			(emptyAnonymousFunction ||
				b.flags.lastToken.Type == tkEquals ||
				(reservedArray(b.flags.lastToken, specialWords...) && b.flags.lastToken.Text != "else")) {
			b.output.SpaceBeforeToken = true
		} else {
			b.printNewline(false, true)
		}
	} else { // collapse, or an inline frame
		if isArray(b.previousFlags.mode) && (b.flags.lastToken.Type == tkStartExpr || b.flags.lastToken.Type == tkComma) {
			if b.flags.lastToken.Type == tkComma || b.options.SpaceInParen {
				b.output.SpaceBeforeToken = true
			}

			if b.flags.lastToken.Type == tkComma || (b.flags.lastToken.Type == tkStartExpr && b.flags.inlineFrame) {
				b.allowWrapOrPreservedNewline(currentToken, false)
				b.previousFlags.multilineFrame = b.previousFlags.multilineFrame || b.flags.multilineFrame
				b.flags.multilineFrame = false
			}
		}
		if b.flags.lastToken.Type != tkOperator && b.flags.lastToken.Type != tkStartExpr {
			if inTypes(b.flags.lastToken.Type, tkStartBlock, tkSemicolon) && !b.flags.inlineFrame {
				b.printNewline(false, false)
			} else {
				b.output.SpaceBeforeToken = true
			}
		}
	}
	b.printToken(currentToken)
	b.indent()

	// Except in these cases an opening brace is followed by a line break.
	if !emptyBraces && !(b.options.BracePreserveInline && b.flags.inlineFrame) {
		b.printNewline(false, false)
	}
}

func (b *Beautifier) handleEndBlock(currentToken *core.Token) {
	// Statements all close with their container.
	b.handleWhitespaceAndComments(currentToken, false)

	for b.flags.mode == modeStatement {
		b.restoreMode()
	}

	emptyBraces := b.flags.lastToken.Type == tkStartBlock

	switch {
	case b.flags.inlineFrame && !emptyBraces:
		// Only set with brace_style preserve-inline.
		b.output.SpaceBeforeToken = true
	case b.options.BraceStyle == "expand":
		if !emptyBraces {
			b.printNewline(false, false)
		}
	case !emptyBraces:
		// {} stays together.
		if isArray(b.flags.mode) && b.options.KeepArrayIndentation {
			// A line break is needed here, which printNewline would skip while
			// keep_array_indentation is on.
			b.options.KeepArrayIndentation = false
			b.printNewline(false, false)
			b.options.KeepArrayIndentation = true
		} else {
			b.printNewline(false, false)
		}
	}
	b.restoreMode()
	b.printToken(currentToken)
}

func (b *Beautifier) handleWord(currentToken *core.Token) {
	if currentToken.Type == tkReserved {
		switch {
		case inList(currentToken.Text, "set", "get") && b.flags.mode != modeObjectLiteral:
			currentToken.Type = tkWord
		case currentToken.Text == "import" && inList(b.tokens.Peek(0).Text, "(", "."):
			currentToken.Type = tkWord
		case inList(currentToken.Text, "as", "from") && !b.flags.importBlock:
			currentToken.Type = tkWord
		case b.flags.mode == modeObjectLiteral:
			if b.tokens.Peek(0).Text == ":" {
				currentToken.Type = tkWord
			}
		}
	}

	if b.startOfStatement(currentToken) {
		// The conditional starts the statement if appropriate.
		if reservedArray(b.flags.lastToken, "var", "let", "const") && currentToken.Type == tkWord {
			b.flags.declarationStatement = true
		}
	} else if currentToken.Newlines != 0 && !isExpression(b.flags.mode) &&
		(b.flags.lastToken.Type != tkOperator || (b.flags.lastToken.Text == "--" || b.flags.lastToken.Text == "++")) &&
		b.flags.lastToken.Type != tkEquals &&
		(b.options.PreserveNewlines || !reservedArray(b.flags.lastToken, "var", "let", "const", "set", "get")) {
		b.handleWhitespaceAndComments(currentToken, false)
		b.printNewline(false, false)
	} else {
		b.handleWhitespaceAndComments(currentToken, false)
	}

	if b.flags.doBlock && !b.flags.doWhile {
		if reservedWord(currentToken, "while") {
			// do {} ## while ()
			b.output.SpaceBeforeToken = true
			b.printToken(currentToken)
			b.output.SpaceBeforeToken = true
			b.flags.doWhile = true
			return
		}
		// do {} should be followed by while; recover when it is not.
		b.printNewline(false, false)
		b.flags.doBlock = false
	}

	// An if may or may not be followed by else; nested bare ifs unwind their modes here:
	// if (a) if (b) c(); else d(); else e();
	if b.flags.ifBlock {
		if !b.flags.elseBlock && reservedWord(currentToken, "else") {
			b.flags.elseBlock = true
		} else {
			for b.flags.mode == modeStatement {
				b.restoreMode()
			}
			b.flags.ifBlock = false
			b.flags.elseBlock = false
		}
	}

	if b.flags.inCaseStatement && reservedArray(currentToken, "case", "default") {
		b.printNewline(false, false)
		if !b.flags.caseBlock && (b.flags.caseBody || b.options.JslintHappy) {
			// Cases following one another.
			b.deindent()
		}
		b.flags.caseBody = false

		b.printToken(currentToken)
		b.flags.inCase = true
		return
	}

	if inTypes(b.flags.lastToken.Type, tkComma, tkStartExpr, tkEquals, tkOperator) {
		// A numeric property value with a +/- sign starts an object property too.
		if !b.startOfObjectProperty() && !(inList(b.flags.lastToken.Text, "+", "-") &&
			b.lastLastText == ":" && b.flags.parent.mode == modeObjectLiteral) {
			b.allowWrapOrPreservedNewline(currentToken, false)
		}
	}

	if reservedWord(currentToken, "function") {
		if inList(b.flags.lastToken.Text, "}", ";") ||
			(b.output.JustAddedNewline() && !(inList(b.flags.lastToken.Text, "(", "[", "{", ":", "=", ",") ||
				b.flags.lastToken.Type == tkOperator)) {
			// A function definition gets an empty line above it.
			if !b.output.JustAddedBlankline() && currentToken.CommentsBefore == nil {
				b.printNewline(false, false)
				b.printNewline(true, false)
			}
		}
		if b.flags.lastToken.Type == tkReserved || b.flags.lastToken.Type == tkWord {
			switch {
			case reservedArray(b.flags.lastToken, "get", "set", "new", "export") ||
				reservedArray(b.flags.lastToken, newlineRestrictedTokens...):
				b.output.SpaceBeforeToken = true
			case reservedWord(b.flags.lastToken, "default") && b.lastLastText == "export":
				b.output.SpaceBeforeToken = true
			case b.flags.lastToken.Text == "declare":
				// TypeScript's declare function.
				b.output.SpaceBeforeToken = true
			default:
				b.printNewline(false, false)
			}
		} else if b.flags.lastToken.Type == tkOperator || b.flags.lastToken.Text == "=" {
			// foo = function
			b.output.SpaceBeforeToken = true
		} else if !b.flags.multilineFrame && (isExpression(b.flags.mode) || isArray(b.flags.mode)) {
			// (function
		} else {
			b.printNewline(false, false)
		}

		b.printToken(currentToken)
		b.flags.lastWord = currentToken.Text
		return
	}

	const (
		prefixNone = iota
		prefixSpace
		prefixNewline
	)
	prefix := prefixNone

	switch {
	case b.flags.lastToken.Type == tkEndBlock:
		switch {
		case b.previousFlags.inlineFrame:
			prefix = prefixSpace
		case !reservedArray(currentToken, "else", "catch", "finally", "from"):
			prefix = prefixNewline
		case b.options.BraceStyle == "expand" || b.options.BraceStyle == "end-expand" ||
			(b.options.BraceStyle == "none" && currentToken.Newlines != 0):
			prefix = prefixNewline
		default:
			prefix = prefixSpace
			b.output.SpaceBeforeToken = true
		}
	case b.flags.lastToken.Type == tkSemicolon && b.flags.mode == modeBlockStatement:
		prefix = prefixNewline
	case b.flags.lastToken.Type == tkSemicolon && isExpression(b.flags.mode):
		prefix = prefixSpace
	case b.flags.lastToken.Type == tkString:
		prefix = prefixNewline
	case b.flags.lastToken.Type == tkReserved || b.flags.lastToken.Type == tkWord ||
		(b.flags.lastToken.Text == "*" &&
			(inList(b.lastLastText, "function", "yield") ||
				(b.flags.mode == modeObjectLiteral && inList(b.lastLastText, "{", ",")))):
		prefix = prefixSpace
	case b.flags.lastToken.Type == tkStartBlock:
		if b.flags.inlineFrame {
			prefix = prefixSpace
		} else {
			prefix = prefixNewline
		}
	case b.flags.lastToken.Type == tkEndExpr:
		b.output.SpaceBeforeToken = true
		prefix = prefixNewline
	}

	if reservedArray(currentToken, lineStarters...) && b.flags.lastToken.Text != ")" {
		if b.flags.inlineFrame || b.flags.lastToken.Text == "else" || b.flags.lastToken.Text == "export" {
			prefix = prefixSpace
		} else {
			prefix = prefixNewline
		}
	}

	switch {
	case reservedArray(currentToken, "else", "catch", "finally"):
		if (!(b.flags.lastToken.Type == tkEndBlock && b.previousFlags.mode == modeBlockStatement) ||
			b.options.BraceStyle == "expand" ||
			b.options.BraceStyle == "end-expand" ||
			(b.options.BraceStyle == "none" && currentToken.Newlines != 0)) &&
			!b.flags.inlineFrame {
			b.printNewline(false, false)
		} else {
			b.output.Trim(true)
			// After the trim, anything but a closing brace before us (a comment, as in
			// "} // comment") needs the line break back.
			if b.output.CurrentLine.Last() != "}" {
				b.printNewline(false, false)
			}
			b.output.SpaceBeforeToken = true
		}
	case prefix == prefixNewline:
		switch {
		case reservedArray(b.flags.lastToken, specialWords...):
			// No line break in 'return nnn'.
			b.output.SpaceBeforeToken = true
		case b.flags.lastToken.Text == "declare" && reservedArray(currentToken, "var", "let", "const"):
			// TypeScript's declare var.
			b.output.SpaceBeforeToken = true
		case b.flags.lastToken.Type != tkEndExpr:
			// No forced line break for var in for (var x = 0...).
			if (b.flags.lastToken.Type != tkStartExpr || !reservedArray(currentToken, "var", "let", "const")) &&
				b.flags.lastToken.Text != ":" {
				if reservedWord(currentToken, "if") && reservedWord(currentToken.Previous, "else") {
					// No line break in } else if {.
					b.output.SpaceBeforeToken = true
				} else {
					b.printNewline(false, false)
				}
			}
		case reservedArray(currentToken, lineStarters...) && b.flags.lastToken.Text != ")":
			b.printNewline(false, false)
		}
	case b.flags.multilineFrame && isArray(b.flags.mode) && b.flags.lastToken.Text == "," && b.lastLastText == "}":
		// }, in lists gets a line break.
		b.printNewline(false, false)
	case prefix == prefixSpace:
		b.output.SpaceBeforeToken = true
	}
	if currentToken.Previous != nil && (currentToken.Previous.Type == tkWord || currentToken.Previous.Type == tkReserved) {
		b.output.SpaceBeforeToken = true
	}
	b.printToken(currentToken)
	b.flags.lastWord = currentToken.Text

	if currentToken.Type == tkReserved {
		switch {
		case currentToken.Text == "do":
			b.flags.doBlock = true
		case currentToken.Text == "if":
			b.flags.ifBlock = true
		case currentToken.Text == "import":
			b.flags.importBlock = true
		case b.flags.importBlock && reservedWord(currentToken, "from"):
			b.flags.importBlock = false
		}
	}
}

func (b *Beautifier) handleSemicolon(currentToken *core.Token) {
	if b.startOfStatement(currentToken) {
		// A semicolon can start (and end) a statement.
		b.output.SpaceBeforeToken = false
	} else {
		b.handleWhitespaceAndComments(currentToken, false)
	}

	nextToken := b.tokens.Peek(0)
	for b.flags.mode == modeStatement &&
		!(b.flags.ifBlock && reservedWord(nextToken, "else")) &&
		!b.flags.doBlock {
		b.restoreMode()
	}

	// An import ends at the semicolon at the latest.
	b.flags.importBlock = false
	b.printToken(currentToken)
}

func (b *Beautifier) handleString(currentToken *core.Token) {
	switch {
	case currentToken.Text.StartsWith("`") && currentToken.Newlines == 0 && currentToken.WhitespaceBefore == "" &&
		(currentToken.Previous.Text == ")" || b.flags.lastToken.Type == tkWord):
		// A tagged template stays attached to its tag.
	case b.startOfStatement(currentToken):
		// Strings want at least a space before them.
		b.output.SpaceBeforeToken = true
	default:
		b.handleWhitespaceAndComments(currentToken, false)
		switch {
		case b.flags.lastToken.Type == tkReserved || b.flags.lastToken.Type == tkWord || b.flags.inlineFrame:
			b.output.SpaceBeforeToken = true
		case inTypes(b.flags.lastToken.Type, tkComma, tkStartExpr, tkEquals, tkOperator):
			if !b.startOfObjectProperty() {
				b.allowWrapOrPreservedNewline(currentToken, false)
			}
		case currentToken.Text.StartsWith("`") && b.flags.lastToken.Type == tkEndExpr &&
			(currentToken.Previous.Text == "]" || currentToken.Previous.Text == ")") && currentToken.Newlines == 0:
			b.output.SpaceBeforeToken = true
		default:
			b.printNewline(false, false)
		}
	}
	b.printToken(currentToken)
}

func (b *Beautifier) handleEquals(currentToken *core.Token) {
	if !b.startOfStatement(currentToken) {
		b.handleWhitespaceAndComments(currentToken, false)
	}

	if b.flags.declarationStatement {
		// An '=' in a var line: line breaks are now handled differently.
		b.flags.declarationAssignment = true
	}
	b.output.SpaceBeforeToken = true
	b.printToken(currentToken)
	b.output.SpaceBeforeToken = true
}

func (b *Beautifier) handleComma(currentToken *core.Token) {
	b.handleWhitespaceAndComments(currentToken, true)

	b.printToken(currentToken)
	b.output.SpaceBeforeToken = true
	switch {
	case b.flags.declarationStatement:
		if isExpression(b.flags.parent.mode) {
			// No break on the comma in for (var a = 1, b = 2).
			b.flags.declarationAssignment = false
		}

		if b.flags.declarationAssignment {
			b.flags.declarationAssignment = false
			b.printNewline(false, true)
		} else if b.options.CommaFirst {
			// A line break before the comma becomes one after it, fixed up when the next token
			// is printed.
			b.allowWrapOrPreservedNewline(currentToken, false)
		}
	case b.flags.mode == modeObjectLiteral ||
		(b.flags.mode == modeStatement && b.flags.parent.mode == modeObjectLiteral):
		if b.flags.mode == modeStatement {
			b.restoreMode()
		}

		if !b.flags.inlineFrame {
			b.printNewline(false, false)
		}
	case b.options.CommaFirst:
		// In an expression or a do block, as above.
		b.allowWrapOrPreservedNewline(currentToken, false)
	}
}

func (b *Beautifier) handleOperator(currentToken *core.Token) {
	isGeneratorAsterisk := currentToken.Text == "*" &&
		(reservedArray(b.flags.lastToken, "function", "yield") ||
			inTypes(b.flags.lastToken.Type, tkStartBlock, tkComma, tkEndBlock, tkSemicolon))
	isUnary := inList(currentToken.Text, "-", "+") &&
		(inTypes(b.flags.lastToken.Type, tkStartBlock, tkStartExpr, tkEquals, tkOperator) ||
			inList(b.flags.lastToken.Text, lineStarters...) ||
			b.flags.lastToken.Text == ",")

	if !b.startOfStatement(currentToken) {
		b.handleWhitespaceAndComments(currentToken, !isGeneratorAsterisk)
	}

	// ActionScript's import .*;
	if currentToken.Text == "*" && b.flags.lastToken.Type == tkDot {
		b.printToken(currentToken)
		return
	}

	// No spaces around the namespacing operator ::.
	if currentToken.Text == "::" {
		b.printToken(currentToken)
		return
	}

	// A numeric property value with a +/- sign.
	if inList(currentToken.Text, "-", "+") && b.startOfObjectProperty() {
		b.printToken(currentToken)
		return
	}

	// Lines may wrap between operators when operator_position is before-newline or
	// preserve-newline.
	if b.flags.lastToken.Type == tkOperator &&
		(b.options.OperatorPosition == operatorBeforeNewline || b.options.OperatorPosition == operatorPreserveNewline) {
		b.allowWrapOrPreservedNewline(currentToken, false)
	}

	if currentToken.Text == ":" && b.flags.inCase {
		b.printToken(currentToken)

		b.flags.inCase = false
		b.flags.caseBody = true
		if b.tokens.Peek(0).Type != tkStartBlock {
			b.indent()
			b.printNewline(false, false)
			b.flags.caseBlock = false
		} else {
			b.flags.caseBlock = true
			b.output.SpaceBeforeToken = true
		}
		return
	}

	spaceBefore := true
	spaceAfter := true
	inTernary := false
	if currentToken.Text == ":" {
		if b.flags.ternaryDepth == 0 {
			// A colon outside a ternary or an object is invalid; this is a best guess.
			spaceBefore = false
		} else {
			b.flags.ternaryDepth--
			inTernary = true
		}
	} else if currentToken.Text == "?" {
		b.flags.ternaryDepth++
	}

	// operator_position takes precedence over the logic below.
	if !isUnary && !isGeneratorAsterisk && b.options.PreserveNewlines && isPositionable(currentToken.Text) {
		isColon := currentToken.Text == ":"
		isTernaryColon := isColon && inTernary
		isOtherColon := isColon && !inTernary

		switch b.options.OperatorPosition {
		case operatorBeforeNewline:
			// A colon outside a ternary gets no space before it.
			b.output.SpaceBeforeToken = !isOtherColon

			b.printToken(currentToken)

			if !isColon || isTernaryColon {
				b.allowWrapOrPreservedNewline(currentToken, false)
			}

			b.output.SpaceBeforeToken = true
			return

		case operatorAfterNewline:
			// Anything but a colon outside a ternary may get a line break before it.
			b.output.SpaceBeforeToken = true

			if !isColon || isTernaryColon {
				if b.tokens.Peek(0).Newlines != 0 {
					b.printNewline(false, true)
				} else {
					b.allowWrapOrPreservedNewline(currentToken, false)
				}
			} else {
				b.output.SpaceBeforeToken = false
			}

			b.printToken(currentToken)

			b.output.SpaceBeforeToken = true
			return

		case operatorPreserveNewline:
			if !isOtherColon {
				b.allowWrapOrPreservedNewline(currentToken, false)
			}

			// No space before after a line break, or before a colon outside a ternary.
			spaceBefore = !(b.output.JustAddedNewline() || isOtherColon)

			b.output.SpaceBeforeToken = spaceBefore
			b.printToken(currentToken)
			b.output.SpaceBeforeToken = true
			return
		}
	}

	switch {
	case isGeneratorAsterisk:
		b.allowWrapOrPreservedNewline(currentToken, false)
		spaceBefore = false
		nextToken := b.tokens.Peek(0)
		spaceAfter = nextToken != nil && inTypes(nextToken.Type, tkWord, tkReserved)
	case currentToken.Text == "...":
		b.allowWrapOrPreservedNewline(currentToken, false)
		spaceBefore = b.flags.lastToken.Type == tkStartBlock
		spaceAfter = false
	case inList(currentToken.Text, "--", "++", "!", "~") || isUnary:
		// Unary operators, and binary +/- that look unary.
		if b.flags.lastToken.Type == tkComma || b.flags.lastToken.Type == tkStartExpr {
			b.allowWrapOrPreservedNewline(currentToken, false)
		}

		spaceBefore = false
		spaceAfter = false

		// A line break between -- or ++ and anything else is kept
		// (http://www.ecma-international.org/ecma-262/5.1/#sec-7.9.1).
		if currentToken.Newlines != 0 && (currentToken.Text == "--" || currentToken.Text == "++" || currentToken.Text == "~") {
			newLineNeeded := reservedArray(b.flags.lastToken, specialWords...) && currentToken.Newlines != 0
			if newLineNeeded && (b.previousFlags.ifBlock || b.previousFlags.elseBlock) {
				b.restoreMode()
			}
			b.printNewline(newLineNeeded, true)
		}

		if b.flags.lastToken.Text == ";" && isExpression(b.flags.mode) {
			// for (;; ++i)
			spaceBefore = true
		}

		switch b.flags.lastToken.Type {
		case tkReserved:
			spaceBefore = true
		case tkEndExpr:
			spaceBefore = !(b.flags.lastToken.Text == "]" && (currentToken.Text == "--" || currentToken.Text == "++"))
		case tkOperator:
			// a++ + ++b; a - -b
			spaceBefore = inList(currentToken.Text, "--", "-", "++", "+") && inList(b.flags.lastToken.Text, "--", "-", "++", "+")
			// + and - after -- or ++ are binary: a-- + b
			if inList(currentToken.Text, "+", "-") && inList(b.flags.lastToken.Text, "--", "++") {
				spaceAfter = true
			}
		}

		if ((b.flags.mode == modeBlockStatement && !b.flags.inlineFrame) || b.flags.mode == modeStatement) &&
			(b.flags.lastToken.Text == "{" || b.flags.lastToken.Text == ";") {
			// { foo; --i }
			// foo(); --bar;
			b.printNewline(false, false)
		}
	}

	b.output.SpaceBeforeToken = b.output.SpaceBeforeToken || spaceBefore
	b.printToken(currentToken)
	b.output.SpaceBeforeToken = spaceAfter
}

func (b *Beautifier) handleBlockComment(currentToken *core.Token, preserveStatementFlags bool) {
	if b.output.Raw {
		b.output.AddRawToken(currentToken)
		if currentToken.Directives != nil && currentToken.Directives["preserve"] == "end" {
			// When testing raw output, a directive does not turn it off.
			b.output.Raw = b.options.TestOutputRaw
		}
		return
	}

	if currentToken.Directives != nil {
		b.printNewline(false, preserveStatementFlags)
		b.printToken(currentToken)
		if currentToken.Directives["preserve"] == "start" {
			b.output.Raw = true
		}
		b.printNewline(false, true)
		return
	}

	// An inline block comment.
	if !newline.Test(currentToken.Text) && currentToken.Newlines == 0 {
		b.output.SpaceBeforeToken = true
		b.printToken(currentToken)
		b.output.SpaceBeforeToken = true
		return
	}
	b.printBlockComment(currentToken, preserveStatementFlags)
}

func (b *Beautifier) printBlockComment(currentToken *core.Token, preserveStatementFlags bool) {
	lines := splitLinebreaks(currentToken.Text)
	lastIndent := currentToken.WhitespaceBefore
	lastIndentLength := lastIndent.Len()

	// A block comment starts on a new line.
	b.printNewline(false, preserveStatementFlags)

	// Its first line is always indented.
	b.printTokenLineIndentation(currentToken)
	b.output.AddToken(lines[0])
	b.printNewline(false, preserveStatementFlags)

	if len(lines) <= 1 {
		return
	}
	lines = lines[1:]
	javadoc := allLinesStartWith(lines, "*")
	starless := eachLineMatchesIndent(lines, lastIndent)

	if javadoc {
		b.flags.alignment = 1
	}

	for _, line := range lines {
		switch {
		case javadoc:
			// Javadoc: reformat and re-indent.
			b.printTokenLineIndentation(currentToken)
			b.output.AddToken(leadingWhitespace.Replace(line, ""))
		case starless && line != "":
			// Starless: re-indent non-empty content, without trimming.
			b.printTokenLineIndentation(currentToken)
			b.output.AddToken(line.SubstringFrom(lastIndentLength))
		default:
			// Other comments are output raw.
			b.output.CurrentLine.SetIndent(-1, 0)
			b.output.AddToken(line)
		}

		// A comment on its own line or over several lines is followed by a line break.
		b.printNewline(false, preserveStatementFlags)
	}

	b.flags.alignment = 0
}

func (b *Beautifier) handleComment(currentToken *core.Token, preserveStatementFlags bool) {
	if currentToken.Newlines != 0 {
		b.printNewline(false, preserveStatementFlags)
	} else {
		b.output.Trim(true)
	}

	b.output.SpaceBeforeToken = true
	b.printToken(currentToken)
	b.printNewline(false, preserveStatementFlags)
}

func (b *Beautifier) handleDot(currentToken *core.Token) {
	if !b.startOfStatement(currentToken) {
		b.handleWhitespaceAndComments(currentToken, true)
	}

	if isDigits(b.flags.lastToken.Text) {
		b.output.SpaceBeforeToken = true
	}

	if reservedArray(b.flags.lastToken, specialWords...) {
		b.output.SpaceBeforeToken = false
	} else {
		// Line breaks before dots are kept; break_chained_methods forces one after a ')':
		// bar().baz()
		b.allowWrapOrPreservedNewline(currentToken,
			b.flags.lastToken.Text == ")" && b.options.BreakChainedMethods)
	}

	// A chained method's dot is unindented only when it starts a line; otherwise the
	// redundant-indentation removal takes care of it.
	if b.options.UnindentChainedMethods && b.output.JustAddedNewline() {
		b.deindent()
	}

	b.printToken(currentToken)
}

func (b *Beautifier) handleUnknown(currentToken *core.Token, preserveStatementFlags bool) {
	b.printToken(currentToken)

	if currentToken.Text.EndsWith("\n") {
		b.printNewline(false, preserveStatementFlags)
	}
}

func (b *Beautifier) handleEOF(currentToken *core.Token) {
	// Close the open statements.
	for b.flags.mode == modeStatement {
		b.restoreMode()
	}
	b.handleWhitespaceAndComments(currentToken, false)
}
