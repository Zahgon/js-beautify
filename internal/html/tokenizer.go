package html

import (
	"slices"

	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Token types of the HTML tokenizer (TOKEN in js/src/html/tokenizer.js).
const (
	tokenTagOpen          = "TK_TAG_OPEN"
	tokenTagClose         = "TK_TAG_CLOSE"
	tokenControlFlowOpen  = "TK_CONTROL_FLOW_OPEN"
	tokenControlFlowClose = "TK_CONTROL_FLOW_CLOSE"
	tokenAttribute        = "TK_ATTRIBUTE"
	tokenEquals           = "TK_EQUALS"
	tokenValue            = "TK_VALUE"
	tokenComment          = "TK_COMMENT"
	tokenText             = "TK_TEXT"
	tokenUnknown          = "TK_UNKNOWN"
	tokenEOF              = core.TokenEOF
)

var directivesCore = core.NewDirectives(`<\!--`, `-->`)

// re compiles a regex literal of the original that has no flags.
func re(source jsrt.String) *jsrt.Regexp { return jsrt.MustCompile(source, "") }

// tokenizer splits HTML into tags, attributes, text and comments (Tokenizer in
// js/src/html/tokenizer.js).
type tokenizer struct {
	*core.Tokenizer
	options *Options

	word                         *core.TemplatablePattern
	wordControlFlowCloseExcluded *core.TemplatablePattern
	singleQuote                  *core.TemplatablePattern
	doubleQuote                  *core.TemplatablePattern
	attribute                    *core.TemplatablePattern
	elementName                  *core.TemplatablePattern
	angularControlFlowStart      *core.Pattern
	handlebarsComment            *core.Pattern
	handlebars                   *core.Pattern
	handlebarsOpen               *core.Pattern
	handlebarsRawClose           *core.Pattern
	comment                      *core.Pattern
	cdata                        *core.Pattern
	conditionalComment           *core.Pattern
	processing                   *core.Pattern
	// unformattedContentDelimiter is nil when unformatted_content_delimiter is empty.
	unformattedContentDelimiter *core.Pattern

	// closingTagPatterns caches the regexes that find the end of an element's raw content.
	closingTagPatterns map[jsrt.String]*jsrt.Regexp
	// err is the SyntaxError of an invalid closing-tag regex; the token stream ends there.
	err error
}

func newTokenizer(input jsrt.String, options *Options) *tokenizer {
	t := &tokenizer{Tokenizer: core.NewTokenizer(input), options: options,
		closingTagPatterns: map[jsrt.String]*jsrt.Regexp{}}

	// Words end at whitespace or when a tag starts; when handlebars are indented, they are
	// tags too.
	templatable := core.NewTemplatablePattern(t.Input).ReadOptions(options.Options)
	pattern := core.NewPattern(t.Input)

	t.word = templatable.Until(re(`[\n\r\t <]`))
	t.wordControlFlowCloseExcluded = templatable.Until(re(`[\n\r\t <}]`))
	t.singleQuote = templatable.UntilAfter(re(`'`))
	t.doubleQuote = templatable.UntilAfter(re(`"`))
	t.attribute = templatable.Until(re(`[\n\r\t =>]|\/>`))
	t.elementName = templatable.Until(re(`[\n\r\t >\/]`))

	t.angularControlFlowStart = pattern.Matching(re(`\@[a-zA-Z]+[^({]*[({]`))
	t.handlebarsComment = pattern.StartingWith(re(`{{!--`)).UntilAfter(re(`--}}`))
	t.handlebars = pattern.StartingWith(re(`{{`)).UntilAfter(re(`}}`))
	t.handlebarsOpen = pattern.Until(re(`[\n\r\t }]`))
	t.handlebarsRawClose = pattern.Until(re(`}}`))
	t.comment = pattern.StartingWith(re(`<!--`)).UntilAfter(re(`-->`))
	t.cdata = pattern.StartingWith(re(`<!\[CDATA\[`)).UntilAfter(re(`]]>`))
	// https://en.wikipedia.org/wiki/Conditional_comment
	t.conditionalComment = pattern.StartingWith(re(`<!\[`)).UntilAfter(re(`]>`))
	t.processing = pattern.StartingWith(re(`<\?`)).UntilAfter(re(`\?>`))

	if options.IndentHandlebars {
		t.word = t.word.Exclude(core.Handlebars)
		t.wordControlFlowCloseExcluded = t.wordControlFlowCloseExcluded.Exclude(core.Handlebars)
	}

	if options.UnformattedContentDelimiter != "" {
		literal := t.Input.GetLiteralRegexp(options.UnformattedContentDelimiter)
		t.unformattedContentDelimiter = pattern.Matching(literal).UntilAfter(literal)
	}
	return t
}

// tokenize reads the whole input. The error is the SyntaxError the original throws when an
// unformatted tag name does not make a valid closing-tag regex.
func (t *tokenizer) tokenize() (*core.TokenStream, error) {
	tokens := t.Tokenize(t)
	return tokens, t.err
}

// IsOpening reports whether a token opens a tag or a control flow block.
func (t *tokenizer) IsOpening(current *core.Token) bool {
	return current.Type == tokenTagOpen || current.Type == tokenControlFlowOpen
}

// IsClosing reports whether current closes open: ">" or "/>" for a "<" tag, "}}" for a
// handlebars tag, "}" for a control flow block.
func (t *tokenizer) IsClosing(current, open *core.Token) bool {
	return (current.Type == tokenTagClose &&
		(((current.Text == ">" || current.Text == "/>") && open.Text.CharAt(0) == "<") ||
			(current.Text == "}}" && open.Text.CharAt(0) == "{" && open.Text.CharAt(1) == "{"))) ||
		(current.Type == tokenControlFlowClose && current.Text == "}" && open.Text.EndsWith("{"))
}

// GetNextToken reads the next token, trying each kind of token in the original's order.
func (t *tokenizer) GetNextToken(previous, open *core.Token) *core.Token {
	t.ReadWhitespace()
	c := t.Input.Peek(0)
	// After a SyntaxError the original has stopped: end the stream.
	if c == "" || t.err != nil {
		return t.CreateToken(tokenEOF, "")
	}

	token := t.readOpenHandlebars(c, open)
	if token == nil {
		token = t.readAttribute(c, previous, open)
	}
	if token == nil {
		token = t.readClose(c, open)
	}
	if token == nil {
		token = t.readScriptAndStyle(c, previous)
	}
	if token == nil {
		token = t.readControlFlows(c, open)
	}
	if token == nil {
		token = t.readRawContent(previous, open)
	}
	if token == nil {
		token = t.readContentWord(c, open)
	}
	if token == nil {
		token = t.readCommentOrCdata(c)
	}
	if token == nil {
		token = t.readProcessing(c)
	}
	if token == nil {
		token = t.readOpen(c, open)
	}
	if token != nil {
		return token
	}
	return t.CreateToken(tokenUnknown, t.Input.Next())
}

// readCommentOrCdata reads an HTML comment (with a whole ignored section when the comment is a
// beautify ignore:start directive) or a CDATA section.
func (t *tokenizer) readCommentOrCdata(c jsrt.String) *core.Token {
	if c != "<" {
		return nil
	}
	var resultingString jsrt.String
	var directives map[jsrt.String]jsrt.String
	// Comments are read as literals, even more than preformatted tags: only the closing
	// marker counts.
	if t.Input.Peek(1) == "!" {
		resultingString = t.comment.Read()
		// Directives are only processed in HTML comments.
		if resultingString != "" {
			directives = directivesCore.GetDirectives(resultingString)
			if directives != nil && directives["ignore"] == "start" {
				resultingString += directivesCore.ReadIgnored(t.Input)
			}
		} else {
			resultingString = t.cdata.Read()
		}
	}
	if resultingString == "" {
		return nil
	}
	token := t.CreateToken(tokenComment, resultingString)
	token.Directives = directives
	return token
}

// readProcessing reads a conditional comment (<![if IE]>) or a processing instruction (<?xml ?>).
func (t *tokenizer) readProcessing(c jsrt.String) *core.Token {
	if c != "<" {
		return nil
	}
	var resultingString jsrt.String
	if peek1 := t.Input.Peek(1); peek1 == "!" || peek1 == "?" {
		resultingString = t.conditionalComment.Read()
		if resultingString == "" {
			resultingString = t.processing.Read()
		}
	}
	if resultingString == "" {
		return nil
	}
	return t.CreateToken(tokenComment, resultingString)
}

// readOpen reads the start of a tag: "<" or "</" and the element name.
func (t *tokenizer) readOpen(c jsrt.String, open *core.Token) *core.Token {
	if (open != nil && open.Type != tokenControlFlowOpen) || c != "<" {
		return nil
	}
	resultingString := t.Input.Next()
	if t.Input.Peek(0) == "/" {
		resultingString += t.Input.Next()
	}
	resultingString += t.elementName.Read()
	return t.CreateToken(tokenTagOpen, resultingString)
}

// readOpenHandlebars reads a handlebars comment or the start of a handlebars tag ("{{#if").
func (t *tokenizer) readOpenHandlebars(c jsrt.String, open *core.Token) *core.Token {
	if open != nil && open.Type != tokenControlFlowOpen {
		return nil
	}
	if !(slices.Contains(t.options.Templating, "angular") || t.options.IndentHandlebars) ||
		c != "{" || t.Input.Peek(1) != "{" {
		return nil
	}
	if t.options.IndentHandlebars && t.Input.Peek(2) == "!" {
		resultingString := t.handlebarsComment.Read()
		if resultingString == "" {
			resultingString = t.handlebars.Read()
		}
		return t.CreateToken(tokenComment, resultingString)
	}
	return t.CreateToken(tokenTagOpen, t.handlebarsOpen.Read())
}

// readControlFlows reads the start ("@if (a) {") or the end ("}") of an angular control flow
// block, when angular templating is on.
func (t *tokenizer) readControlFlows(c jsrt.String, open *core.Token) *core.Token {
	if !slices.Contains(t.options.Templating, "angular") {
		return nil
	}
	if c == "@" {
		resultingString := t.angularControlFlowStart.Read()
		if resultingString == "" {
			return nil
		}
		openingParentheses := 0
		if resultingString.EndsWith("(") {
			openingParentheses = 1
		}
		closingParentheses := 0
		// The opening brace of the block is the first "{" outside parentheses, as in
		// @if({value: true} !== null) {
		for !(resultingString.EndsWith("{") && openingParentheses == closingParentheses) {
			next := t.Input.Next()
			if next == "" {
				break
			} else if next == "(" {
				openingParentheses++
			} else if next == ")" {
				closingParentheses++
			}
			resultingString += next
		}
		return t.CreateToken(tokenControlFlowOpen, resultingString)
	}
	if c == "}" && open != nil && open.Type == tokenControlFlowOpen {
		return t.CreateToken(tokenControlFlowClose, t.Input.Next())
	}
	return nil
}

// readClose reads the end of a tag: ">" or "/>" for a "<" tag, "}}" for a handlebars tag.
func (t *tokenizer) readClose(c jsrt.String, open *core.Token) *core.Token {
	if open == nil || open.Type != tokenTagOpen {
		return nil
	}
	if open.Text.CharAt(0) == "<" && (c == ">" || (c == "/" && t.Input.Peek(1) == ">")) {
		resultingString := t.Input.Next()
		if c == "/" {
			resultingString += t.Input.Next()
		}
		return t.CreateToken(tokenTagClose, resultingString)
	}
	if open.Text.CharAt(0) == "{" && c == "}" && t.Input.Peek(1) == "}" {
		t.Input.Next()
		t.Input.Next()
		return t.CreateToken(tokenTagClose, "}}")
	}
	return nil
}

// readAttribute reads an attribute name, "=" or a value inside a "<" tag.
func (t *tokenizer) readAttribute(c jsrt.String, previous, open *core.Token) *core.Token {
	if open == nil || open.Text.CharAt(0) != "<" {
		return nil
	}
	switch c {
	case "=":
		return t.CreateToken(tokenEquals, t.Input.Next())
	case `"`, "'":
		content := t.Input.Next()
		if c == `"` {
			content += t.doubleQuote.Read()
		} else {
			content += t.singleQuote.Read()
		}
		return t.CreateToken(tokenValue, content)
	}
	resultingString := t.attribute.Read()
	if resultingString == "" {
		return nil
	}
	if previous.Type == tokenEquals {
		return t.CreateToken(tokenValue, resultingString)
	}
	return t.CreateToken(tokenAttribute, resultingString)
}

// isContentUnformatted reports whether the content of tagName is read as one raw text: void
// elements have no content; content_unformatted and unformatted elements keep theirs.
func (t *tokenizer) isContentUnformatted(tagName jsrt.String) bool {
	return !slices.Contains(t.options.VoidElements, tagName) &&
		(slices.Contains(t.options.ContentUnformatted, tagName) || slices.Contains(t.options.Unformatted, tagName))
}

// openedTagName returns the lowercased name of the "<" tag that previous closes, when previous
// is the ">" ending a start tag (an empty tag "/>" has no content).
func openedTagName(previous *core.Token) (jsrt.String, bool) {
	if previous.Type != tokenTagClose || previous.Opened.Text.CharAt(0) != "<" || previous.Text.CharAt(0) == "/" {
		return "", false
	}
	return previous.Opened.Text.SubstringFrom(1).ToLowerCase(), true
}

// readUntilClosingTag reads up to the closing tag of tagName. The original builds the regex
// from the tag name as it is, so a name holding regex syntax can make it invalid: the
// SyntaxError then ends the tokenization.
func (t *tokenizer) readUntilClosingTag(tagName jsrt.String) jsrt.String {
	pattern, ok := t.closingTagPatterns[tagName]
	if !ok {
		var err error
		pattern, err = jsrt.Compile(`</`+tagName+`[\n\r\t ]*?>`, "ig")
		if err != nil {
			t.err = err
			return ""
		}
		t.closingTagPatterns[tagName] = pattern
	}
	return t.Input.ReadUntil(pattern, false)
}

// readRawContent reads the inside of a handlebars tag, or the whole content of an element whose
// content is unformatted.
func (t *tokenizer) readRawContent(previous, open *core.Token) *core.Token {
	var resultingString jsrt.String
	if open != nil && open.Text.CharAt(0) == "{" {
		resultingString = t.handlebarsRawClose.Read()
	} else if tagName, ok := openedTagName(previous); ok && t.isContentUnformatted(tagName) {
		resultingString = t.readUntilClosingTag(tagName)
	}
	if resultingString == "" {
		return nil
	}
	return t.CreateToken(tokenText, resultingString)
}

// readScriptAndStyle reads the content of a script or style element as one text token. The
// content may be wrapped in a comment or CDATA section.
func (t *tokenizer) readScriptAndStyle(c jsrt.String, previous *core.Token) *core.Token {
	tagName, ok := openedTagName(previous)
	if !ok || (tagName != "script" && tagName != "style") {
		return nil
	}
	if token := t.readCommentOrCdata(c); token != nil {
		token.Type = tokenText
		return token
	}
	if resultingString := t.readUntilClosingTag(tagName); resultingString != "" {
		return t.CreateToken(tokenText, resultingString)
	}
	return nil
}

// readContentWord reads a run of text up to whitespace or a tag, or a section delimited by
// unformatted_content_delimiter.
func (t *tokenizer) readContentWord(c jsrt.String, open *core.Token) *core.Token {
	var resultingString jsrt.String
	if t.unformattedContentDelimiter != nil && c == t.options.UnformattedContentDelimiter.CharAt(0) {
		resultingString = t.unformattedContentDelimiter.Read()
	}
	if resultingString == "" {
		if open != nil && open.Type == tokenControlFlowOpen {
			resultingString = t.wordControlFlowCloseExcluded.Read()
		} else {
			resultingString = t.word.Read()
		}
	}
	if resultingString == "" {
		return nil
	}
	return t.CreateToken(tokenText, resultingString)
}
