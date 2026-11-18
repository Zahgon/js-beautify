package html

import (
	"errors"
	"maps"
	"slices"

	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

var (
	lineBreak     = re(`\r\n|[\r\n]`)
	allLineBreaks = jsrt.MustCompile(`\r\n|[\r\n]`, "g")
	leadingIndent = re(`^[\t ]*`)

	tagCheckPattern           = re(`^<([^\s>]*)`)
	handlebarsTagCheckPattern = re(`^{{~?(?:[\^]|#\*?)?([^\s}]+)`)
	notBlockHelperStart       = re(`[^#\^]`)
	handlebarsIfPattern       = re(`{{#if`)

	cssType        = re(`text/css`)
	javascriptType = re(`module|importmap|((text|application|dojo)\/(x-)?(javascript|ecmascript|jscript|livescript|(ld\+)?json|method|aspect))`)
	htmlType       = re(`(text|application|dojo)\/(x-)?(html)`)
	// test/null is the original's test-only type for scripts that are only indented.
	nullType = re(`test\/null`)

	trailingBlankLine = re(`\n[ \t]*$`)
	wrappedStart      = re(`^(<!--|<!\[CDATA\[)`)
	wrappedContent    = re(`^(<!--[^\n]*|<!\[CDATA\[)(\n?)([ \t\n]*)([\s\S]*)(-->|]]>)$`)
	trailingSpaces    = re(`[ \t]+$`)
	everyNewline      = jsrt.MustCompile(`\n`, "g")

	// pClosers are the elements whose start tag ends an open <p>.
	pClosers = []jsrt.String{"address", "article", "aside", "blockquote", "details", "div", "dl", "fieldset",
		"figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "main", "menu",
		"nav", "ol", "p", "pre", "section", "table", "ul"}
	pParentExcludes = []jsrt.String{"a", "audio", "del", "ins", "map", "noscript", "video"}
)

// contentType is the type the original gives text tokens it handles itself.
const contentType = "TK_CONTENT"

// errNullLastToken is the TypeError the original throws when a handler reads the type of the
// previous parser token while it is still null, which happens when the document starts with
// tokens the beautifier only prints raw.
var errNullLastToken = errors.New("Cannot read properties of null (reading 'type')")

// lastToken is what the handlers see of the previous parser token: its type. null is set while
// only unknown tokens have been printed, where the original's last_token is null.
type lastToken struct {
	typ  string
	null bool
}

// tagToken is the parser's view of a tag or comment (TagOpenParserToken in
// js/src/html/beautifier.js).
type tagToken struct {
	parent               *tagToken
	text                 jsrt.String
	tagName              jsrt.String
	isInlineElement      bool
	isUnformatted        bool
	isContentUnformatted bool
	isEmptyElement       bool
	isStartTag           bool
	isEndTag             bool
	indentContent        bool
	multilineContent     bool
	// customBeautifierName is "javascript", "css", "html", "null" or "" (none).
	customBeautifierName jsrt.String
	startTagToken        *tagToken
	attrCount            int
	hasWrappedAttrs      bool
	alignmentSize        int
	tagComplete          bool
	tagStartChar         jsrt.String
	tagCheck             jsrt.String
}

// newTagToken parses the tag that rawToken opens; a nil rawToken gives the empty, complete
// token the parser starts with.
func newTagToken(options *Options, parent *tagToken, rawToken *core.Token) *tagToken {
	t := &tagToken{parent: parent}
	if rawToken == nil {
		t.tagComplete = true
		return t
	}

	t.tagStartChar = rawToken.Text.CharAt(0)
	t.text = rawToken.Text

	if t.tagStartChar == "<" {
		if m := tagCheckPattern.Exec(rawToken.Text); m != nil {
			t.tagCheck = m.GroupText(1)
		}
	} else {
		if m := handlebarsTagCheckPattern.Exec(rawToken.Text); m != nil {
			t.tagCheck = m.GroupText(1)
		}
		// "{{#> myPartial}}" or "{{~#> myPartial}}"
		if (rawToken.Text.StartsWith("{{#>") || rawToken.Text.StartsWith("{{~#>")) && t.tagCheck.CharAt(0) == ">" {
			if t.tagCheck == ">" && rawToken.Next != nil {
				t.tagCheck = rawToken.Next.Text.Split(" ")[0]
			} else {
				t.tagCheck = rawToken.Text.Split(">")[1]
			}
		}
	}

	t.tagCheck = t.tagCheck.ToLowerCase()

	if rawToken.Type == tokenComment {
		t.tagComplete = true
	}

	t.isStartTag = t.tagCheck.CharAt(0) != "/"
	t.tagName = t.tagCheck
	if !t.isStartTag {
		t.tagName = t.tagCheck.SubstringFrom(1)
	}
	t.isEndTag = !t.isStartTag || (rawToken.Closed != nil && rawToken.Closed.Text == "/>")

	// With the whitespace control "~" ({{~#if true}}), handlebars tags start at index 3.
	handlebarStarts := 2
	if t.tagStartChar == "{" && t.text.Len() >= 3 && t.text.CharAt(2) == "~" {
		handlebarStarts = 3
	}

	// Handlebars tags that do not start with # or ^ are single tags, so they both start and end;
	// so are all handlebars tags when they are not indented.
	t.isEndTag = t.isEndTag || (t.tagStartChar == "{" &&
		(!options.IndentHandlebars || t.text.Len() < 3 || notBlockHelperStart.Test(t.text.CharAt(handlebarStarts))))
	return t
}

// Beautifier formats one HTML document (Beautifier in js/src/html/beautifier.js).
type Beautifier struct {
	sourceText  jsrt.String
	options     *Options
	jsBeautify  Func
	cssBeautify Func
	tagStack    *tagStack

	isWrapAttributesForce                bool
	isWrapAttributesForceExpandMultiline bool
	isWrapAttributesForceAligned         bool
	isWrapAttributesAlignedMultiple      bool
	isWrapAttributesPreserve             bool
	isWrapAttributesPreserveAligned      bool

	// err is the exception a handler raised; the beautification stops there.
	err error
}

// NewBeautifier returns a beautifier for sourceText that formats embedded scripts and style
// sheets with js and css (either may be nil: that content is then only indented). Options are
// read and validated here.
func NewBeautifier(sourceText jsrt.String, options map[string]any, js, css Func) (*Beautifier, error) {
	o, err := NewOptions(options)
	if err != nil {
		return nil, err
	}
	wrap := o.WrapAttributes
	return &Beautifier{
		sourceText:  sourceText,
		options:     o,
		jsBeautify:  js,
		cssBeautify: css,

		isWrapAttributesForce:                wrap.StartsWith("force"),
		isWrapAttributesForceExpandMultiline: wrap == "force-expand-multiline",
		isWrapAttributesForceAligned:         wrap == "force-aligned",
		isWrapAttributesAlignedMultiple:      wrap == "aligned-multiple",
		isWrapAttributesPreserve:             wrap.StartsWith("preserve"),
		isWrapAttributesPreserveAligned:      wrap == "preserve-aligned",
	}, nil
}

// typeOf returns the type of the previous parser token, recording the original's TypeError
// when it is null.
func (b *Beautifier) typeOf(t lastToken) string {
	if t.null && b.err == nil {
		b.err = errNullLastToken
	}
	return t.typ
}

// Beautify returns the formatted document.
func (b *Beautifier) Beautify() (jsrt.String, error) {
	if b.options.Disabled {
		return b.sourceText, nil
	}

	sourceText := b.sourceText
	eol := b.options.EOL
	if eol == "auto" {
		eol = "\n"
		if m := lineBreak.Exec(sourceText); m != nil {
			eol = m.Text()
		}
	}

	// Line breaks are normalized to \n before parsing.
	sourceText = allLineBreaks.Replace(sourceText, "\n")

	baseIndentString := leadingIndent.Exec(sourceText).Text()

	lastTagToken := newTagToken(b.options, nil, nil)

	pr, err := newPrinter(b.options, baseIndentString)
	if err != nil {
		return "", err
	}
	tokens, err := newTokenizer(sourceText, b.options).tokenize()
	if err != nil {
		return "", err
	}

	b.tagStack = &tagStack{printer: pr}

	last := lastToken{}
	// The original keeps the previous parser token for raw-printed tokens, starting from null.
	parserToken := lastToken{null: true}
	for rawToken := tokens.Next(); rawToken.Type != tokenEOF; rawToken = tokens.Next() {
		switch {
		case rawToken.Type == tokenTagOpen || rawToken.Type == tokenComment:
			lastTagToken = b.handleTagOpen(pr, rawToken, lastTagToken, last, tokens)
			parserToken = lastToken{typ: tokenTagOpen}
		case rawToken.Type == tokenAttribute || rawToken.Type == tokenEquals || rawToken.Type == tokenValue ||
			(rawToken.Type == tokenText && !lastTagToken.tagComplete):
			parserToken = lastToken{typ: b.handleInsideTag(pr, rawToken, lastTagToken, last)}
		case rawToken.Type == tokenTagClose:
			parserToken = lastToken{typ: b.handleTagClose(pr, rawToken, lastTagToken)}
		case rawToken.Type == tokenText:
			parserToken = lastToken{typ: b.handleText(pr, rawToken, lastTagToken)}
		case rawToken.Type == tokenControlFlowOpen:
			parserToken = lastToken{typ: b.handleControlFlowOpen(pr, rawToken)}
		case rawToken.Type == tokenControlFlowClose:
			parserToken = lastToken{typ: b.handleControlFlowClose(pr, rawToken)}
		default:
			// Unknown tokens are printed as they are.
			pr.addRawToken(rawToken)
		}
		if b.err != nil {
			return "", b.err
		}
		last = parserToken
	}
	return pr.output.GetCode(eol)
}

func (b *Beautifier) handleControlFlowOpen(pr *printer, rawToken *core.Token) string {
	pr.setSpaceBeforeToken(rawToken.Newlines != 0 || rawToken.WhitespaceBefore != "", true)
	if rawToken.Newlines != 0 {
		pr.printPreservedNewlines(rawToken)
	} else {
		pr.setSpaceBeforeToken(rawToken.Newlines != 0 || rawToken.WhitespaceBefore != "", true)
	}
	pr.printToken(rawToken)
	pr.indent()
	return rawToken.Type
}

func (b *Beautifier) handleControlFlowClose(pr *printer, rawToken *core.Token) string {
	pr.deindent()
	if rawToken.Newlines != 0 {
		pr.printPreservedNewlines(rawToken)
	} else {
		pr.setSpaceBeforeToken(rawToken.Newlines != 0 || rawToken.WhitespaceBefore != "", true)
	}
	pr.printToken(rawToken)
	return rawToken.Type
}

func (b *Beautifier) handleTagClose(pr *printer, rawToken *core.Token, lastTagToken *tagToken) string {
	pr.alignmentSize = 0
	lastTagToken.tagComplete = true

	pr.setSpaceBeforeToken(rawToken.Newlines != 0 || rawToken.WhitespaceBefore != "", true)
	if lastTagToken.isUnformatted {
		pr.addRawToken(rawToken)
	} else {
		if lastTagToken.tagStartChar == "<" {
			// A space before "/>", none before ">".
			pr.setSpaceBeforeToken(rawToken.Text.CharAt(0) == "/", true)
			if b.isWrapAttributesForceExpandMultiline && lastTagToken.hasWrappedAttrs {
				pr.printNewline(false)
			}
		}
		pr.printToken(rawToken)
	}

	if lastTagToken.indentContent && !(lastTagToken.isUnformatted || lastTagToken.isContentUnformatted) {
		pr.indent()
		// Indent only once per opened tag.
		lastTagToken.indentContent = false
	}

	if !lastTagToken.isInlineElement && !(lastTagToken.isUnformatted || lastTagToken.isContentUnformatted) {
		pr.setWrapPoint()
	}
	return rawToken.Type
}

func (b *Beautifier) handleInsideTag(pr *printer, rawToken *core.Token, lastTagToken *tagToken, last lastToken) string {
	wrapped := lastTagToken.hasWrappedAttrs

	pr.setSpaceBeforeToken(rawToken.Newlines != 0 || rawToken.WhitespaceBefore != "", true)
	switch {
	case lastTagToken.isUnformatted:
		pr.addRawToken(rawToken)
	case lastTagToken.tagStartChar == "{" && rawToken.Type == tokenText:
		// Inside handlebars, keep line breaks or a single space between the tag and its content.
		if pr.printPreservedNewlines(rawToken) {
			rawToken.Newlines = 0
			pr.addRawToken(rawToken)
		} else {
			pr.printToken(rawToken)
		}
	default:
		switch {
		case rawToken.Type == tokenAttribute:
			pr.setSpaceBeforeToken(true, false)
		case rawToken.Type == tokenEquals:
			// No space before =.
			pr.setSpaceBeforeToken(false, false)
		case rawToken.Type == tokenValue && rawToken.Previous.Type == tokenEquals:
			// No space before a value.
			pr.setSpaceBeforeToken(false, false)
		}

		if rawToken.Type == tokenAttribute && lastTagToken.tagStartChar == "<" {
			if b.isWrapAttributesPreserve || b.isWrapAttributesPreserveAligned {
				pr.traverseWhitespace(rawToken)
				wrapped = wrapped || rawToken.Newlines != 0
			}

			// The force modes wrap when the tag has at least wrap_attributes_min_attrs
			// attributes: every attribute after the first, and the first one too for
			// force-expand-multiline.
			if b.isWrapAttributesForce && lastTagToken.attrCount >= b.options.WrapAttributesMinAttrs &&
				(b.typeOf(last) != tokenTagOpen || b.isWrapAttributesForceExpandMultiline) {
				pr.printNewline(false)
				wrapped = true
			}
		}
		pr.printToken(rawToken)
		wrapped = wrapped || pr.previousTokenWrapped()
		lastTagToken.hasWrappedAttrs = wrapped
	}
	return rawToken.Type
}

func (b *Beautifier) handleText(pr *printer, rawToken *core.Token, lastTagToken *tagToken) string {
	switch {
	case lastTagToken.customBeautifierName != "":
		if err := b.printCustomBeautifierText(pr, rawToken, lastTagToken); err != nil {
			b.err = err
		}
	case lastTagToken.isUnformatted || lastTagToken.isContentUnformatted:
		pr.addRawToken(rawToken)
	default:
		pr.traverseWhitespace(rawToken)
		pr.printToken(rawToken)
	}
	return contentType
}

// childBeautifier returns the beautifier for the content of a script or style element, or nil
// when the content is only indented.
func (b *Beautifier) childBeautifier(name jsrt.String) func(jsrt.String, map[string]any) (jsrt.String, error) {
	external := func(f Func) func(jsrt.String, map[string]any) (jsrt.String, error) {
		return func(source jsrt.String, options map[string]any) (jsrt.String, error) {
			out, err := f(source.UTF8(), options)
			return jsrt.FromUTF8(out), err
		}
	}
	switch {
	case name == "javascript" && b.jsBeautify != nil:
		return external(b.jsBeautify)
	case name == "css" && b.cssBeautify != nil:
		return external(b.cssBeautify)
	case name == "html":
		return func(source jsrt.String, options map[string]any) (jsrt.String, error) {
			child, err := NewBeautifier(source, options, b.jsBeautify, b.cssBeautify)
			if err != nil {
				return "", err
			}
			return child.Beautify()
		}
	}
	return nil
}

// childOptions are the options a script or style beautifier gets: the HTML beautifier's raw
// options (with their "js" and "css" sections) and eol "\n", since the HTML output converts
// line breaks itself.
func (b *Beautifier) childOptions() map[string]any {
	options := maps.Clone(b.options.Raw)
	options["eol"] = "\n"
	return options
}

// printCustomBeautifierText prints the content of a script or style element, formatted by the
// beautifier for its type or only indented.
func (b *Beautifier) printCustomBeautifierText(pr *printer, rawToken *core.Token, lastTagToken *tagToken) error {
	if rawToken.Text == "" {
		return nil
	}
	text := rawToken.Text
	name := lastTagToken.customBeautifierName
	beautifier := b.childBeautifier(name)

	scriptIndentLevel := 1
	switch b.options.IndentScripts {
	case "keep":
		scriptIndentLevel = 0
	case "separate":
		scriptIndentLevel = -pr.indentLevel
	}
	indentation := pr.getFullIndent(scriptIndentLevel)

	// An empty line at the end of the text is dropped; one is added back after the text,
	// before the closing tag.
	text = trailingBlankLine.Replace(text, "")

	var pre, post jsrt.String
	// Content wrapped in a comment or CDATA section.
	if name != "html" && text.CharAt(0) == "<" && wrappedStart.Test(text) {
		// The pattern ends with (-->|]]>)$: checking the end first spares the quadratic
		// backtracking of a failed match on a long unterminated comment.
		var matched *jsrt.Match
		if text.EndsWith("-->") || text.EndsWith("]]>") {
			matched = wrappedContent.Exec(text)
		}
		// Wrapping started but not finished: print the text raw.
		if matched == nil {
			pr.addRawToken(rawToken)
			return nil
		}

		pre = indentation + matched.GroupText(1) + "\n"
		text = matched.GroupText(4)
		if end := matched.GroupText(5); end != "" {
			post = indentation + end
		}

		text = trailingBlankLine.Replace(text, "")

		if lead := matched.GroupText(3); matched.GroupText(2) != "" || lead.Includes("\n") {
			// The spaces starting the first line of the wrapped text are the indentation
			// to remove when the text is only indented.
			if m := trailingSpaces.Exec(lead); m != nil {
				rawToken.WhitespaceBefore = m.Text()
			}
		}
	}

	if text != "" {
		if beautifier != nil {
			var err error
			if text, err = beautifier(indentation+text, b.childOptions()); err != nil {
				return err
			}
		} else {
			// Only indent the text.
			if white := rawToken.WhitespaceBefore; white != "" {
				text = jsrt.MustCompile("\n("+white+")?", "g").Replace(text, "\n")
			}
			text = indentation + everyNewline.Replace(text, "\n"+indentation)
		}
	}

	if pre != "" {
		if text == "" {
			text = pre + post
		} else {
			text = pre + text + "\n" + post
		}
	}

	pr.printNewline(false)
	if text != "" {
		rawToken.Text = text
		rawToken.WhitespaceBefore = ""
		rawToken.Newlines = 0
		pr.addRawToken(rawToken)
		pr.printNewline(true)
	}
	return nil
}

func (b *Beautifier) handleTagOpen(pr *printer, rawToken *core.Token, lastTagToken *tagToken, last lastToken,
	tokens *core.TokenStream) *tagToken {
	parserToken := b.getTagOpenToken(rawToken)

	if (lastTagToken.isUnformatted || lastTagToken.isContentUnformatted) && !lastTagToken.isEmptyElement &&
		rawToken.Type == tokenTagOpen && !parserToken.isStartTag {
		// The end tag of an unformatted or content_unformatted element is printed raw, so that
		// the line breaks inside the element stay exactly as they are.
		pr.addRawToken(rawToken)
		parserToken.startTagToken = b.tagStack.tryPop(parserToken.tagName)
	} else {
		pr.traverseWhitespace(rawToken)
		b.setTagPosition(pr, rawToken, parserToken, lastTagToken, last)
		if !parserToken.isInlineElement {
			pr.setWrapPoint()
		}
		pr.printToken(rawToken)
	}

	// Count the attributes.
	if parserToken.isStartTag && b.isWrapAttributesForce {
		for i := 0; ; i++ {
			peek := tokens.Peek(i)
			if peek.Type == tokenAttribute {
				parserToken.attrCount++
			}
			if peek.Type == tokenEOF || peek.Type == tokenTagClose {
				break
			}
		}
	}

	// Attributes are aligned after the tag name in the aligned modes.
	if b.isWrapAttributesForceAligned || b.isWrapAttributesAlignedMultiple || b.isWrapAttributesPreserveAligned {
		parserToken.alignmentSize = rawToken.Text.Len() + 1
	}

	if !parserToken.tagComplete && !parserToken.isUnformatted {
		pr.alignmentSize = parserToken.alignmentSize
	}
	return parserToken
}

// getTagOpenToken parses a tag and classifies it with the options.
func (b *Beautifier) getTagOpenToken(rawToken *core.Token) *tagToken {
	o := b.options
	t := newTagToken(o, b.tagStack.getParserToken(), rawToken)

	t.alignmentSize = o.WrapAttributesIndentSize

	t.isEndTag = t.isEndTag || slices.Contains(o.VoidElements, t.tagCheck)

	t.isEmptyElement = t.tagComplete || (t.isStartTag && t.isEndTag)

	t.isUnformatted = !t.tagComplete && slices.Contains(o.Unformatted, t.tagCheck)
	t.isContentUnformatted = !t.isEmptyElement && slices.Contains(o.ContentUnformatted, t.tagCheck)
	t.isInlineElement = slices.Contains(o.Inline, t.tagName) ||
		(o.InlineCustomElements && t.tagName.Includes("-")) || t.tagStartChar == "{"
	return t
}

func (b *Beautifier) setTagPosition(pr *printer, rawToken *core.Token, parserToken, lastTagToken *tagToken,
	last lastToken) {
	if !parserToken.isEmptyElement {
		if parserToken.isEndTag {
			// Close the element and everything opened inside it.
			parserToken.startTagToken = b.tagStack.tryPop(parserToken.tagName)
		} else {
			// A start tag may end an element whose end tag is optional.
			if b.doOptionalEndElement(parserToken) != nil && !parserToken.isInlineElement {
				pr.printNewline(false)
			}

			b.tagStack.recordTag(parserToken)

			if (parserToken.tagName == "script" || parserToken.tagName == "style") &&
				!(parserToken.isUnformatted || parserToken.isContentUnformatted) {
				parserToken.customBeautifierName = getCustomBeautifierName(parserToken.tagCheck, rawToken)
			}
		}
	}

	if slices.Contains(b.options.ExtraLiners, parserToken.tagCheck) {
		pr.printNewline(false)
		if !pr.output.JustAddedBlankline() {
			pr.printNewline(true)
		}
	}

	switch {
	case parserToken.isEmptyElement:
		// {{else}} goes back to the indentation of the enclosing if, unless or each block.
		if parserToken.tagStartChar == "{" && parserToken.tagCheck == "else" {
			b.tagStack.indentToTag("if", "unless", "each")
			parserToken.indentContent = true
			// No new line when the {{#if}} is on the current line.
			if !pr.currentLineHasMatch(handlebarsIfPattern) {
				pr.printNewline(false)
			}
		}

		// A comment after an end tag stays on that line.
		if !(parserToken.tagName == "!--" && b.typeOf(last) == tokenTagClose &&
			lastTagToken.isEndTag && parserToken.text.IndexOf("\n") == -1) {
			if !(parserToken.isInlineElement || parserToken.isUnformatted) {
				pr.printNewline(false)
			}
			b.calculateParentMultiline(pr, parserToken)
		}
	case parserToken.isEndTag:
		doEndExpand := parserToken.startTagToken != nil && parserToken.startTagToken.multilineContent
		doEndExpand = doEndExpand || (!parserToken.isInlineElement &&
			!(lastTagToken.isInlineElement || lastTagToken.isUnformatted) &&
			!(b.typeOf(last) == tokenTagClose && parserToken.startTagToken == lastTagToken) &&
			b.typeOf(last) != contentType)

		if parserToken.isContentUnformatted || parserToken.isUnformatted {
			doEndExpand = false
		}
		if doEndExpand {
			pr.printNewline(false)
		}
	default:
		parserToken.indentContent = parserToken.customBeautifierName == ""

		if parserToken.tagStartChar == "<" {
			switch parserToken.tagName {
			case "html":
				parserToken.indentContent = b.options.IndentInnerHTML
			case "head":
				parserToken.indentContent = b.options.IndentHeadInnerHTML
			case "body":
				parserToken.indentContent = b.options.IndentBodyInnerHTML
			}
		}

		if !(parserToken.isInlineElement || parserToken.isUnformatted) &&
			(b.typeOf(last) != contentType || parserToken.isContentUnformatted) {
			pr.printNewline(false)
		}

		b.calculateParentMultiline(pr, parserToken)
	}
}

// calculateParentMultiline marks the parent element multiline when the tag starts a new line,
// unless an inline element starts it inside an inline parent.
func (b *Beautifier) calculateParentMultiline(pr *printer, parserToken *tagToken) {
	if parserToken.parent != nil && pr.output.JustAddedNewline() &&
		!((parserToken.isInlineElement || parserToken.isUnformatted) && parserToken.parent.isInlineElement) {
		parserToken.parent.multilineContent = true
	}
}

// doOptionalEndElement closes the open element whose end tag may be omitted before
// parserToken (https://www.w3.org/TR/html5/syntax.html#optional-tags) and returns its parser
// token, or nil. An element is otherwise assumed to end with its parent. Handlebars tags such as
// {{#tr}} never take part.
func (b *Beautifier) doOptionalEndElement(parserToken *tagToken) *tagToken {
	if parserToken.tagStartChar == "{" {
		parserToken.parent = b.tagStack.getParserToken()
		return nil
	}

	if parserToken.isEmptyElement || !parserToken.isStartTag || parserToken.parent == nil {
		return nil
	}

	var result *tagToken
	tryPop := func(tag jsrt.String, stopList ...jsrt.String) {
		if result == nil {
			result = b.tagStack.tryPop(tag, stopList...)
		}
	}
	tables := []jsrt.String{"table", "thead", "tbody", "tfoot"}

	switch name := parserToken.tagName; {
	case name == "body":
		// A head element's end tag may be omitted if the head element is not immediately
		// followed by a space character or a comment.
		tryPop("head")
	case name == "li":
		tryPop("li", "ol", "ul", "menu")
	case name == "dd" || name == "dt":
		tryPop("dt", "dl")
		tryPop("dd", "dl")
	case parserToken.parent.tagName == "p" && slices.Contains(pClosers, name):
		// The parent of the <p> must not be an <a>, <audio>, <del>, <ins>, <map>,
		// <noscript> or <video> element (autonomous custom elements are not considered).
		pParent := parserToken.parent.parent
		if pParent == nil || !slices.Contains(pParentExcludes, pParent.tagName) {
			tryPop("p")
		}
	case name == "rp" || name == "rt":
		tryPop("rt", "ruby", "rtc")
		tryPop("rp", "ruby", "rtc")
	case name == "optgroup":
		tryPop("optgroup", "select")
	case name == "option":
		tryPop("option", "select", "datalist", "optgroup")
	case name == "colgroup":
		tryPop("caption", "table")
	case name == "thead":
		tryPop("caption", "table")
		tryPop("colgroup", "table")
	case name == "tbody" || name == "tfoot":
		tryPop("caption", "table")
		tryPop("colgroup", "table")
		tryPop("thead", "table")
		tryPop("tbody", "table")
	case name == "tr":
		tryPop("caption", "table")
		tryPop("colgroup", "table")
		tryPop("tr", tables...)
	case name == "th" || name == "td":
		tryPop("td", append(tables, "tr")...)
		tryPop("th", append(tables, "tr")...)
	}

	// The parent may have changed.
	parserToken.parent = b.tagStack.getParserToken()
	return result
}

// getTypeAttribute returns the value of the type attribute of the tag startToken opens, or "".
func getTypeAttribute(startToken *core.Token) jsrt.String {
	for t := startToken.Next; t.Type != tokenEOF && startToken.Closed != t; t = t.Next {
		if t.Type == tokenAttribute && t.Text == "type" {
			if t.Next != nil && t.Next.Type == tokenEquals && t.Next.Next != nil && t.Next.Next.Type == tokenValue {
				return t.Next.Next.Text
			}
			break
		}
	}
	return ""
}

// getCustomBeautifierName returns the beautifier for the content of a script or style element:
// from its type attribute, or by default javascript for scripts and css for style sheets.
func getCustomBeautifierName(tagCheck jsrt.String, rawToken *core.Token) jsrt.String {
	if rawToken.Closed == nil {
		return ""
	}

	var typeAttribute jsrt.String
	switch tagCheck {
	case "script":
		typeAttribute = "text/javascript"
	case "style":
		typeAttribute = "text/css"
	}
	if t := getTypeAttribute(rawToken); t != "" {
		typeAttribute = t
	}

	switch {
	case cssType.Search(typeAttribute) > -1:
		return "css"
	case javascriptType.Search(typeAttribute) > -1:
		return "javascript"
	case htmlType.Search(typeAttribute) > -1:
		return "html"
	case nullType.Search(typeAttribute) > -1:
		return "null"
	}
	return ""
}
