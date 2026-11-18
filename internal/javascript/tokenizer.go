package javascript

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Token types of the JavaScript tokenizer.
const (
	tkStartExpr    = "TK_START_EXPR"
	tkEndExpr      = "TK_END_EXPR"
	tkStartBlock   = "TK_START_BLOCK"
	tkEndBlock     = "TK_END_BLOCK"
	tkWord         = "TK_WORD"
	tkReserved     = "TK_RESERVED"
	tkSemicolon    = "TK_SEMICOLON"
	tkString       = "TK_STRING"
	tkEquals       = "TK_EQUALS"
	tkOperator     = "TK_OPERATOR"
	tkComma        = "TK_COMMA"
	tkBlockComment = "TK_BLOCK_COMMENT"
	tkComment      = "TK_COMMENT"
	tkDot          = "TK_DOT"
	tkUnknown      = "TK_UNKNOWN"
	tkStart        = core.TokenStart
	tkEOF          = core.TokenEOF
)

var (
	directivesCore = core.NewDirectives(`\/\*`, `\*\/`)

	numberPattern = jsrt.MustCompile(`0[xX][0123456789abcdefABCDEF_]*n?|0[oO][01234567_]*n?|0[bB][01_]*n?|\d[\d_]*n|(?:\.\d[\d_]*|\d[\d_]*\.?[\d_]*)(?:[eE][+-]?[\d_]+)?`, "")

	digit = jsrt.MustCompile(`[0-9]`, "")

	// positionableOperators are the operators whose line break operator_position governs.
	positionableOperators = []jsrt.String{
		">>>", "===", "!==", "&&=", "??=", "||=",
		"<<", "&&", ">=", "**", "!=", "==", "<=", ">>", "||", "??", "|>",
		"<", "/", "-", "+", ">", ":", "&", "%", "?", "^", "|", "*",
	}

	punctPattern = jsrt.MustCompile(punctSource(), "")

	// lineStarters are the words that always start a new line.
	lineStarters = []jsrt.String{"continue", "try", "throw", "return", "var", "let", "const", "if", "switch",
		"case", "default", "for", "while", "break", "function", "import", "export"}
	reservedWords = append(append([]jsrt.String(nil), lineStarters...), "do", "in", "of", "else", "get",
		"set", "new", "catch", "finally", "typeof", "yield", "async", "await", "from", "as", "class", "extends")
	reservedWordPattern = jsrt.MustCompile("^(?:"+jsrt.Join(reservedWords, "|")+")$", "")

	xmlPattern = jsrt.MustCompile(`[\s\S]*?<(\/?)([-a-zA-Z:0-9_.]+|{[^}]+?}|!\[CDATA\[[^\]]*?\]\]|)(\s*{[^}]+?}|\s+[-a-zA-Z:0-9_.]+|\s+[-a-zA-Z:0-9_.]+\s*=\s*('[^']*'|"[^"]*"|{([^{}]|{[^}]+?})+?}))*\s*(\/?)\s*>`, "")

	restOfInput       = jsrt.MustCompile(`[\s\S]*`, "g")
	lineCommentStart  = jsrt.MustCompile(`\/\/`, "")
	blockCommentStart = jsrt.MustCompile(`\/\*`, "")
	blockCommentEnd   = jsrt.MustCompile(`\*\/`, "")
	htmlCommentStart  = jsrt.MustCompile(`<!--`, "")
	htmlCommentEnd    = jsrt.MustCompile(`-->`, "")
	includeStart      = jsrt.MustCompile(`#include`, "")
	shebangStart      = jsrt.MustCompile(`#!`, "")
	singleQuoteEnd    = jsrt.MustCompile(`['\\\n\r\u2028\u2029]`, "")
	doubleQuoteEnd    = jsrt.MustCompile(`["\\\n\r\u2028\u2029]`, "")
	templateTextEnd   = jsrt.MustCompile("[`\\\\$]", "")
	templateExprEnd   = jsrt.MustCompile("[`}\\\\]", "")
	curlyStartSpace   = jsrt.MustCompile(`^{\s+`, "")
	curlyEndSpace     = jsrt.MustCompile(`\s+}$`, "")
	unescapeKeep      = jsrt.MustCompile(`([\s]|[^\\]|\\\\)+`, "g")
	unescapeHex       = jsrt.MustCompile(`x([0-9A-Fa-f]{2})`, "g")
	unescapeUnicode   = jsrt.MustCompile(`u([0-9A-Fa-f]{4})`, "g")
	unescapeCodePoint = jsrt.MustCompile(`u\{([0-9A-Fa-f]+)\}`, "g")
)

// punctSource builds the punctuation regex as the original does: the operators longest first,
// regex-escaped and joined with |, with "?." (not followed by a digit) in front.
func punctSource() jsrt.String {
	punct := jsrt.String(">>>= " +
		"... >>= <<= === >>> !== **= &&= ??= ||= " +
		"=> ^= :: /= << <= == && -= >= >> != -- += ** || ?? ++ %= &= *= |= |> " +
		"= ! ? > < : / ^ - + * & % ~ |")
	punct = jsrt.MustCompile(`[-[\]{}()*+?.,\\^$|#]`, "g").Replace(punct, `\$&`)
	punct = `\?\.(?!\d) ` + punct
	return jsrt.MustCompile(" ", "g").Replace(punct, "|")
}

// tokenizerPatterns are the readers the tokenizer builds over its input.
type tokenizerPatterns struct {
	identifier         *core.TemplatablePattern
	number             *core.Pattern
	punct              *core.Pattern
	comment            *core.Pattern
	blockComment       *core.Pattern
	htmlCommentStart   *core.Pattern
	htmlCommentEnd     *core.Pattern
	include            *core.Pattern
	shebang            *core.Pattern
	xml                *core.Pattern
	singleQuote        *core.TemplatablePattern
	doubleQuote        *core.TemplatablePattern
	templateText       *core.TemplatablePattern
	templateExpression *core.TemplatablePattern
}

// Tokenizer splits JavaScript into tokens (js/src/javascript/tokenizer.js).
type Tokenizer struct {
	*core.Tokenizer
	options  *Options
	patterns tokenizerPatterns

	// inHTMLComment is set between a leading "<!--" and the next "-->".
	inHTMLComment bool
	// hasCharEscapes records a \x or \u escape in the string being read.
	hasCharEscapes bool
}

// NewTokenizer returns a tokenizer for input.
func NewTokenizer(input jsrt.String, options *Options) *Tokenizer {
	t := &Tokenizer{Tokenizer: core.NewTokenizer(input), options: options}
	t.Whitespace = t.Whitespace.Matching(`\u00A0\u1680\u180e\u2000-\u200a\u202f\u205f\u3000\ufeff`, `\u2028\u2029`)

	patternReader := core.NewPattern(t.Input)
	templatable := core.NewTemplatablePattern(t.Input).ReadOptions(options.Options)

	t.patterns = tokenizerPatterns{
		identifier: templatable.StartingWith(identifier).Matching(identifierMatch),
		number:     patternReader.Matching(numberPattern),
		punct:      patternReader.Matching(punctPattern),
		// A comment ends just before the nearest line break or at the end of the input.
		comment: patternReader.StartingWith(lineCommentStart).Until(newline),
		// A block comment ends with the nearest */ or at the end of the input.
		blockComment:       patternReader.StartingWith(blockCommentStart).UntilAfter(blockCommentEnd),
		htmlCommentStart:   patternReader.Matching(htmlCommentStart),
		htmlCommentEnd:     patternReader.Matching(htmlCommentEnd),
		include:            patternReader.StartingWith(includeStart).UntilAfter(lineBreak),
		shebang:            patternReader.StartingWith(shebangStart).UntilAfter(lineBreak),
		xml:                patternReader.Matching(xmlPattern),
		singleQuote:        templatable.Until(singleQuoteEnd),
		doubleQuote:        templatable.Until(doubleQuoteEnd),
		templateText:       templatable.Until(templateTextEnd),
		templateExpression: templatable.Until(templateExprEnd),
	}
	return t
}

// Tokenize reads the whole input into a token stream.
func (t *Tokenizer) Tokenize() *core.TokenStream {
	return t.Tokenizer.Tokenize(t)
}

// IsComment reports whether a token is attached to the next one as a comment: comments and
// unknown tokens are.
func (t *Tokenizer) IsComment(currentToken *core.Token) bool {
	return currentToken.Type == tkComment || currentToken.Type == tkBlockComment || currentToken.Type == tkUnknown
}

// IsOpening reports whether a token opens a block or an expression.
func (t *Tokenizer) IsOpening(currentToken *core.Token) bool {
	return currentToken.Type == tkStartBlock || currentToken.Type == tkStartExpr
}

// IsClosing reports whether currentToken closes openToken.
func (t *Tokenizer) IsClosing(currentToken, openToken *core.Token) bool {
	return (currentToken.Type == tkEndBlock || currentToken.Type == tkEndExpr) &&
		openToken != nil && ((currentToken.Text == "]" && openToken.Text == "[") ||
		(currentToken.Text == ")" && openToken.Text == "(") ||
		(currentToken.Text == "}" && openToken.Text == "{"))
}

// Reset clears the state kept between tokens.
func (t *Tokenizer) Reset() {
	t.inHTMLComment = false
}

// GetNextToken reads the next token.
func (t *Tokenizer) GetNextToken(previousToken, openToken *core.Token) *core.Token {
	t.ReadWhitespace()
	c := t.Input.Peek(0)
	if c == "" {
		return t.CreateToken(tkEOF, "")
	}

	if token := t.readNonJavascript(c); token != nil {
		return token
	}
	if token := t.readString(c); token != nil {
		return token
	}
	// '#{' starts a record (issue #2062).
	if token := t.readPair(c, t.Input.Peek(1)); token != nil {
		return token
	}
	if token := t.readWord(previousToken); token != nil {
		return token
	}
	if token := t.readSingles(c); token != nil {
		return token
	}
	if token := t.readComment(c); token != nil {
		return token
	}
	if token := t.readRegexp(c, previousToken); token != nil {
		return token
	}
	if token := t.readXML(c, previousToken); token != nil {
		return token
	}
	if token := t.readPunctuation(); token != nil {
		return token
	}
	return t.CreateToken(tkUnknown, t.Input.Next())
}

func (t *Tokenizer) readWord(previousToken *core.Token) *core.Token {
	resultingString := t.patterns.identifier.Read()
	if resultingString != "" {
		resultingString = allLineBreaks.Replace(resultingString, "\n")
		if !(previousToken.Type == tkDot ||
			(previousToken.Type == tkReserved && (previousToken.Text == "set" || previousToken.Text == "get"))) &&
			reservedWordPattern.Test(resultingString) {
			// 'in' and 'of' after a word or a string are operators.
			if (resultingString == "in" || resultingString == "of") &&
				(previousToken.Type == tkWord || previousToken.Type == tkString) {
				return t.CreateToken(tkOperator, resultingString)
			}
			return t.CreateToken(tkReserved, resultingString)
		}
		return t.CreateToken(tkWord, resultingString)
	}

	resultingString = t.patterns.number.Read()
	if resultingString != "" {
		return t.CreateToken(tkWord, resultingString)
	}
	return nil
}

func (t *Tokenizer) readSingles(c jsrt.String) *core.Token {
	var token *core.Token
	switch {
	case c == "(" || c == "[":
		token = t.CreateToken(tkStartExpr, c)
	case c == ")" || c == "]":
		token = t.CreateToken(tkEndExpr, c)
	case c == "{":
		token = t.CreateToken(tkStartBlock, c)
	case c == "}":
		token = t.CreateToken(tkEndBlock, c)
	case c == ";":
		token = t.CreateToken(tkSemicolon, c)
	case c == "." && isDotFollower(t.Input.Peek(1)):
		token = t.CreateToken(tkDot, c)
	case c == ",":
		token = t.CreateToken(tkComma, c)
	}
	if token != nil {
		t.Input.Next()
	}
	return token
}

// isDotFollower is /[^\d\.]/.test(next) for the code unit after a ".": a "." followed by
// anything but a digit or another "." is a dot token. At the end of the input the original
// tests the string "null", which matches.
func isDotFollower(next jsrt.String) bool {
	return next == "" || (next != "." && !jsrt.IsDigit(uint16(next.CharCodeAt(0))))
}

func (t *Tokenizer) readPair(c, d jsrt.String) *core.Token {
	if c == "#" && d == "{" {
		token := t.CreateToken(tkStartBlock, c+d)
		t.Input.Next()
		t.Input.Next()
		return token
	}
	return nil
}

func (t *Tokenizer) readPunctuation() *core.Token {
	resultingString := t.patterns.punct.Read()
	switch resultingString {
	case "":
		return nil
	case "=":
		return t.CreateToken(tkEquals, resultingString)
	case "?.":
		return t.CreateToken(tkDot, resultingString)
	}
	return t.CreateToken(tkOperator, resultingString)
}

func (t *Tokenizer) readNonJavascript(c jsrt.String) *core.Token {
	if c == "#" {
		if t.IsFirstToken() {
			if s := t.patterns.shebang.Read(); s != "" {
				return t.CreateToken(tkUnknown, s.Trim()+"\n")
			}
		}

		// ExtendScript #include directives.
		if s := t.patterns.include.Read(); s != "" {
			return t.CreateToken(tkUnknown, s.Trim()+"\n")
		}

		c = t.Input.Next()

		// SpiderMonkey's obsolete sharp variables for circular references (#1=, #1#).
		sharp := jsrt.String("#")
		if t.Input.HasNext() && t.Input.TestChar(digit, 0) {
			for {
				c = t.Input.Next()
				sharp += c
				if !(t.Input.HasNext() && c != "#" && c != "=") {
					break
				}
			}
			if c == "#" {
				// #1# is complete.
			} else if t.Input.Peek(0) == "[" && t.Input.Peek(1) == "]" {
				sharp += "[]"
				t.Input.Next()
				t.Input.Next()
			} else if t.Input.Peek(0) == "{" && t.Input.Peek(1) == "}" {
				sharp += "{}"
				t.Input.Next()
				t.Input.Next()
			}
			return t.CreateToken(tkWord, sharp)
		}

		t.Input.Back()
	} else if c == "<" && t.IsFirstToken() {
		if s := t.patterns.htmlCommentStart.Read(); s != "" {
			for t.Input.HasNext() && !isNewline(t.Input.Peek(0)) {
				s += t.Input.Next()
			}
			t.inHTMLComment = true
			return t.CreateToken(tkComment, s)
		}
	} else if t.inHTMLComment && c == "-" {
		if s := t.patterns.htmlCommentEnd.Read(); s != "" {
			t.inHTMLComment = false
			return t.CreateToken(tkComment, s)
		}
	}
	return nil
}

func (t *Tokenizer) readComment(c jsrt.String) *core.Token {
	if c != "/" {
		return nil
	}
	switch t.Input.Peek(1) {
	case "*":
		comment := t.patterns.blockComment.Read()
		directives := directivesCore.GetDirectives(comment)
		if directives != nil && directives["ignore"] == "start" {
			comment += directivesCore.ReadIgnored(t.Input)
		}
		comment = allLineBreaks.Replace(comment, "\n")
		token := t.CreateToken(tkBlockComment, comment)
		token.Directives = directives
		return token
	case "/":
		return t.CreateToken(tkComment, t.patterns.comment.Read())
	}
	return nil
}

func (t *Tokenizer) readString(c jsrt.String) *core.Token {
	if c != "`" && c != "'" && c != `"` {
		return nil
	}
	resultingString := t.Input.Next()
	t.hasCharEscapes = false

	if c == "`" {
		resultingString += t.readStringRecursive("`", true, "${")
	} else {
		resultingString += t.readStringRecursive(c, false, "")
	}

	if t.hasCharEscapes && t.options.UnescapeStrings {
		resultingString = unescapeString(resultingString)
	}

	if t.Input.Peek(0) == c {
		resultingString += t.Input.Next()
	}

	resultingString = allLineBreaks.Replace(resultingString, "\n")
	return t.CreateToken(tkString, resultingString)
}

// allowRegexpOrXML reports whether a regex or an E4X literal may start after previousToken.
func allowRegexpOrXML(previousToken *core.Token) bool {
	return (previousToken.Type == tkReserved &&
		inList(previousToken.Text, "return", "case", "throw", "else", "do", "typeof", "yield")) ||
		(previousToken.Type == tkEndExpr && previousToken.Text == ")" && previousToken.Opened != nil &&
			previousToken.Opened.Previous.Type == tkReserved && inList(previousToken.Opened.Previous.Text, "if", "while", "for")) ||
		inTypes(previousToken.Type, tkComment, tkStartExpr, tkStartBlock, tkStart, tkEndBlock, tkOperator,
			tkEquals, tkEOF, tkSemicolon, tkComma)
}

func (t *Tokenizer) readRegexp(c jsrt.String, previousToken *core.Token) *core.Token {
	if c != "/" || !allowRegexpOrXML(previousToken) {
		return nil
	}
	resultingString := t.Input.Next()
	esc := false
	inCharClass := false
	for t.Input.HasNext() && (esc || inCharClass || t.Input.Peek(0) != c) && !isNewline(t.Input.Peek(0)) {
		next := t.Input.Peek(0)
		resultingString += next
		if !esc {
			esc = next == `\`
			if next == "[" {
				inCharClass = true
			} else if next == "]" {
				inCharClass = false
			}
		} else {
			esc = false
		}
		t.Input.Next()
	}

	if t.Input.Peek(0) == c {
		resultingString += t.Input.Next()
		// Flags: only [gimsuy] are valid, but whatever word follows is taken.
		resultingString += t.Input.Read(identifier, nil, false)
	}
	return t.CreateToken(tkString, resultingString)
}

// trimCurlyName removes the spaces just inside the braces of a {expression} tag name.
func trimCurlyName(name jsrt.String) jsrt.String {
	return curlyEndSpace.Replace(curlyStartSpace.Replace(name, "{"), "}")
}

func (t *Tokenizer) readXML(c jsrt.String, previousToken *core.Token) *core.Token {
	if !t.options.E4X || c != "<" || !allowRegexpOrXML(previousToken) {
		return nil
	}
	match := t.patterns.xml.ReadMatch()
	if match == nil {
		return nil
	}
	var xmlStr jsrt.String
	rootTag := trimCurlyName(match.GroupText(2))
	isCurlyRoot := rootTag.IndexOf("{") == 0
	depth := 0
	for match != nil {
		isEndTag := match.GroupText(1) != ""
		tagName := match.GroupText(2)
		isSingletonTag := match.GroupText(match.NumGroups()) != "" || tagName.Slice(0, 8) == "![CDATA["
		// With a {expression} root every other named tag counts too.
		if !isSingletonTag && (tagName == rootTag || (isCurlyRoot && trimCurlyName(tagName) != "")) {
			if isEndTag {
				depth--
			} else {
				depth++
			}
		}
		xmlStr += match.Text()
		if depth <= 0 {
			break
		}
		match = t.patterns.xml.ReadMatch()
	}
	// An element that is not closed keeps the rest of the input unformatted.
	if match == nil {
		xmlStr += t.Input.Match(restOfInput).Text()
	}
	xmlStr = allLineBreaks.Replace(xmlStr, "\n")
	return t.CreateToken(tkString, xmlStr)
}

// unescapeString decodes the \x and \u escapes of a string literal for unescape_strings. The
// literal is returned unchanged when an escape is malformed or one of \x7f-\xff appears (the
// string is probably binary); control characters, quotes and backslashes stay escaped.
func unescapeString(s jsrt.String) jsrt.String {
	var out jsrt.String
	inputScan := core.NewInputScanner(s)

	for inputScan.HasNext() {
		// Keep whitespace, non-backslash characters and backslash pairs.
		if matched := inputScan.Match(unescapeKeep); matched != nil {
			out += matched.Text()
		}

		if inputScan.Peek(0) != `\` {
			continue
		}
		inputScan.Next()
		var matched *jsrt.Match
		switch inputScan.Peek(0) {
		case "x":
			matched = inputScan.Match(unescapeHex)
		case "u":
			matched = inputScan.Match(unescapeUnicode)
			if matched == nil {
				matched = inputScan.Match(unescapeCodePoint)
			}
		default:
			out += `\`
			if inputScan.HasNext() {
				out += inputScan.Next()
			}
			continue
		}

		if matched == nil {
			return s
		}

		escaped := jsrt.ParseInt(matched.GroupText(1), 16)
		text := matched.Text()
		switch {
		case escaped > 0x7e && escaped <= 0xff && text.IndexOf("x") == 0:
			return s
		case escaped >= 0x00 && escaped < 0x20:
			out += `\` + text
		case escaped > 0x10FFFF:
			// Out of range: keep the escape and go on.
			out += `\` + text
		case escaped == 0x22 || escaped == 0x27 || escaped == 0x5c:
			out += `\` + jsrt.FromUnit(uint16(escaped))
		default:
			// String.fromCharCode keeps the low 16 bits of a code point above U+FFFF.
			out += jsrt.FromUnit(uint16(uint32(escaped)))
		}
	}
	return out
}

// readStringRecursive reads the body of a string up to its delimiter. Template literals
// (allowUnescapedNewlines, startSub "${") recurse into their ${...} expressions, which in turn
// recurse into nested template literals (delimiter "}", startSub "`").
func (t *Tokenizer) readStringRecursive(delimiter jsrt.String, allowUnescapedNewlines bool, startSub jsrt.String) jsrt.String {
	var pattern *core.TemplatablePattern
	switch delimiter {
	case "'":
		pattern = t.patterns.singleQuote
	case `"`:
		pattern = t.patterns.doubleQuote
	case "`":
		pattern = t.patterns.templateText
	case "}":
		pattern = t.patterns.templateExpression
	}

	resultingString := pattern.Read()
	for t.Input.HasNext() {
		next := t.Input.Next()
		if next == delimiter || (!allowUnescapedNewlines && isNewline(next)) {
			t.Input.Back()
			break
		} else if next == `\` && t.Input.HasNext() {
			currentChar := t.Input.Peek(0)
			if currentChar == "x" || currentChar == "u" {
				t.hasCharEscapes = true
			} else if currentChar == "\r" && t.Input.Peek(1) == "\n" {
				t.Input.Next()
			}
			next += t.Input.Next()
		} else if startSub != "" {
			if startSub == "${" && next == "$" && t.Input.Peek(0) == "{" {
				next += t.Input.Next()
			}
			if startSub == next {
				if delimiter == "`" {
					next += t.readStringRecursive("}", allowUnescapedNewlines, "`")
				} else {
					next += t.readStringRecursive("`", allowUnescapedNewlines, "${")
				}
				if t.Input.HasNext() {
					next += t.Input.Next()
				}
			}
		}
		next += pattern.Read()
		resultingString += next
	}
	return resultingString
}
