// Package css is a port of js-beautify's CSS beautifier (js/src/css), which also formats LESS and
// SCSS.
package css

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

var (
	directivesCore = core.NewDirectives(`\/\*`, `\*\/`)

	lineBreak     = jsrt.MustCompile(`\r\n|[\r\n]`, "")
	allLineBreaks = jsrt.MustCompile(`\r\n|[\r\n]`, "g")

	whitespacePattern    = jsrt.MustCompile(`(?:\s|\n)+`, "g")
	blockCommentPattern  = jsrt.MustCompile(`\/\*(?:[\s\S]*?)((?:\*\/)|$)`, "g")
	commentPattern       = jsrt.MustCompile(`\/\/(?:[^\n\r  ]*)`, "g")
	variableEndPattern   = jsrt.MustCompile(`[: ,;{}()[\]\/='"]`, "g")
	leadingIndentPattern = jsrt.MustCompile(`^[\t ]*`, "")

	// nestedAtRule lists the at-rules whose block holds rules
	// (https://developer.mozilla.org/en-US/docs/Web/CSS/At-rule); conditionalGroupRule those
	// whose block holds only rule sets.
	nestedAtRule = map[jsrt.String]bool{
		"page": true, "font-face": true, "keyframes": true,
		// also in conditionalGroupRule below
		"media": true, "supports": true, "document": true,
	}
	conditionalGroupRule = map[jsrt.String]bool{"media": true, "supports": true, "document": true}

	nonSemicolonNewlineProperty = []jsrt.String{"grid-template-areas", "grid-template"}
)

// isAtRuleIn is the original's `name in RULES` test on an object literal, which is also true
// for the properties every object inherits.
func isAtRuleIn(rules map[jsrt.String]bool, name jsrt.String) bool {
	return rules[name] || jsrt.ObjectPrototypeHas(name)
}

// isWhitespace is /\s/.test(s) for a string of at most one code unit.
func isWhitespace(s jsrt.String) bool {
	return s != "" && jsrt.IsWhitespace(uint16(s.CharCodeAt(0)))
}

// Beautifier formats one style sheet (Beautifier in js/src/css/beautifier.js).
type Beautifier struct {
	sourceText jsrt.String
	options    *Options
	ch         jsrt.String
	input      *core.InputScanner
	output     *core.Output

	indentLevel int
	nestedLevel int
}

// NewBeautifier returns a beautifier for sourceText; options are read and validated here.
func NewBeautifier(sourceText jsrt.String, options map[string]any) (*Beautifier, error) {
	o, err := NewOptions(options)
	if err != nil {
		return nil, err
	}
	return &Beautifier{sourceText: sourceText, options: o}, nil
}

// eatString reads up to and including the first of endChars or a line break; a backslash
// escapes the next code unit.
func (b *Beautifier) eatString(endChars jsrt.String) jsrt.String {
	var result jsrt.String
	b.ch = b.input.Next()
	for b.ch != "" {
		result += b.ch
		if b.ch == "\\" {
			next := b.input.Next()
			if next == "" {
				// At the end of the input the original appends next()'s null as text.
				next = "null"
			}
			result += next
		} else if endChars.Includes(b.ch) || b.ch == "\n" {
			break
		}
		b.ch = b.input.Next()
	}
	return result
}

// eatWhitespace skips whitespace from the current position. When allowAtLeastOneNewLine is
// set, every line break skipped starts a new output line, up to max_preserve_newlines (at least
// one). It reports whether there was whitespace.
func (b *Beautifier) eatWhitespace(allowAtLeastOneNewLine bool) bool {
	result := isWhitespace(b.input.Peek(0))
	newlineCount := 0
	for isWhitespace(b.input.Peek(0)) {
		b.ch = b.input.Next()
		if allowAtLeastOneNewLine && b.ch == "\n" {
			if newlineCount == 0 || newlineCount < b.options.MaxPreserveNewlines {
				newlineCount++
				b.output.AddNewLine(true)
			}
		}
	}
	return result
}

// foundNestedPseudoClass reports whether the next special character after a ':' opens a block,
// which makes the ':' a nested pseudo-class rather than a property separator.
func (b *Beautifier) foundNestedPseudoClass() bool {
	openParen := 0
	i := 1
	ch := b.input.Peek(i)
	for ch != "" {
		switch ch {
		case "{":
			return true
		case "(":
			// pseudo-classes can contain ()
			openParen++
		case ")":
			if openParen == 0 {
				return false
			}
			openParen--
		case ";", "}":
			return false
		}
		i++
		ch = b.input.Peek(i)
	}
	return false
}

func (b *Beautifier) printString(outputString jsrt.String) {
	b.output.SetIndent(b.indentLevel, 0)
	b.output.NonBreakingSpace = true
	b.output.AddToken(outputString)
}

func (b *Beautifier) preserveSingleSpace(isAfterSpace bool) {
	if isAfterSpace {
		b.output.SpaceBeforeToken = true
	}
}

func (b *Beautifier) indent() {
	b.indentLevel++
}

func (b *Beautifier) outdent() {
	if b.indentLevel > 0 {
		b.indentLevel--
	}
}

// Beautify returns the formatted style sheet.
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

	baseIndentString := leadingIndentPattern.Exec(sourceText).Text()

	output, err := core.NewOutput(b.options.Options, baseIndentString)
	if err != nil {
		return "", err
	}
	b.output = output
	b.input = core.NewInputScanner(sourceText)
	b.indentLevel = 0
	b.nestedLevel = 0

	b.ch = ""
	parenLevel := 0

	insideRule := false
	// The value side of a property: blue in `label { content: blue }`.
	insidePropertyValue := false
	enteringConditionalGroup := false
	insideNonNestedAtRule := false
	insideScssMap := false
	var topCharacter jsrt.String
	insideNonSemiColonValues := false

	for {
		whitespace := b.input.Read(whitespacePattern, nil, false)
		isAfterSpace := whitespace != ""
		previousCh := topCharacter
		b.ch = b.input.Next()
		if b.ch == "\\" && b.input.HasNext() {
			b.ch += b.input.Next()
		}
		topCharacter = b.ch

		if b.ch == "" {
			break
		} else if b.ch == "/" && b.input.Peek(0) == "*" {
			// A block comment always starts on a new line, also after a declaration on the same
			// line or in minified code.
			b.output.AddNewLine(false)
			b.input.Back()

			comment := b.input.Read(blockCommentPattern, nil, false)

			// Handle the ignore directive.
			if directives := directivesCore.GetDirectives(comment); directives != nil && directives["ignore"] == "start" {
				comment += directivesCore.ReadIgnored(b.input)
			}

			b.printString(comment)

			// Preserve the line breaks after the comment.
			b.eatWhitespace(true)

			// A block comment does not share its line with properties.
			b.output.AddNewLine(false)
		} else if b.ch == "/" && b.input.Peek(0) == "/" {
			// A line comment keeps the space before it on the line of a rule.
			b.output.SpaceBeforeToken = true
			b.input.Back()
			b.printString(b.input.Read(commentPattern, nil, false))

			// Preserve the line breaks after the comment.
			b.eatWhitespace(true)
		} else if b.ch == "$" {
			b.preserveSingleSpace(isAfterSpace)

			b.printString(b.ch)

			// Strip the trailing space, if present, for the hash property checks.
			variable := b.input.PeekUntilAfter(variableEndPattern)

			if variable.EndsWith(" ") || variable.EndsWith(":") {
				// A variable or pseudo-class: print it and one space after it.
				variable = b.eatString(": ").TrimEnd()
				b.printString(variable)
				b.output.SpaceBeforeToken = true
			}

			// It might be a sass variable.
			if parenLevel == 0 && variable.Includes(":") {
				insidePropertyValue = true
				b.indent()
			}
		} else if b.ch == "@" {
			b.preserveSingleSpace(isAfterSpace)

			if b.input.Peek(0) == "{" {
				// A less property mixin @{...}.
				ch := b.ch
				b.printString(ch + b.eatString("}"))
			} else {
				b.printString(b.ch)

				// Strip the trailing space, if present, for the hash property checks.
				variableOrRule := b.input.PeekUntilAfter(variableEndPattern)

				if variableOrRule.EndsWith(" ") || variableOrRule.EndsWith(":") {
					// A variable or pseudo-class: print it and one space after it.
					variableOrRule = b.eatString(": ").TrimEnd()
					b.printString(variableOrRule)
					b.output.SpaceBeforeToken = true
				}

				if parenLevel == 0 && variableOrRule.Includes(":") {
					// It might be a less variable.
					insidePropertyValue = true
					b.indent()
				} else if isAtRuleIn(nestedAtRule, variableOrRule) {
					// It might be a nesting at-rule.
					b.nestedLevel++
					if isAtRuleIn(conditionalGroupRule, variableOrRule) {
						enteringConditionalGroup = true
					}
				} else if parenLevel == 0 && !insidePropertyValue {
					// It might be a non-nested at-rule.
					insideNonNestedAtRule = true
				}
			}
		} else if b.ch == "#" && b.input.Peek(0) == "{" {
			b.preserveSingleSpace(isAfterSpace)
			ch := b.ch
			b.printString(ch + b.eatString("}"))
		} else if b.ch == "{" {
			if insidePropertyValue {
				insidePropertyValue = false
				b.outdent()
			}

			// A non-nested at-rule becomes nested.
			insideNonNestedAtRule = false

			if enteringConditionalGroup {
				// A conditional group holds only rule sets.
				enteringConditionalGroup = false
				insideRule = b.indentLevel >= b.nestedLevel
			} else {
				// Otherwise declarations are allowed too.
				insideRule = b.indentLevel >= b.nestedLevel-1
			}
			if b.options.NewlineBetweenRules && insideRule {
				if b.output.PreviousLine != nil && b.output.PreviousLine.Item(-1) != "{" {
					b.output.EnsureEmptyLineAbove("/", ",")
				}
			}

			b.output.SpaceBeforeToken = true

			// The order of printString and indent differs so that '{' is indented correctly.
			if b.options.BraceStyle == "expand" {
				b.output.AddNewLine(false)
				b.printString(b.ch)
				b.indent()
				b.output.SetIndent(b.indentLevel, 0)
			} else {
				if previousCh == "(" {
					// Inside a mixin whose first parameter is an object.
					b.output.SpaceBeforeToken = false
				} else if previousCh != "," {
					b.indent()
				}
				b.printString(b.ch)
			}

			b.eatWhitespace(true)
			b.output.AddNewLine(false)
		} else if b.ch == "}" {
			b.outdent()
			b.output.AddNewLine(false)
			if previousCh == "{" {
				b.output.Trim(true)
			}

			if insidePropertyValue {
				b.outdent()
				insidePropertyValue = false
			}
			b.printString(b.ch)
			insideRule = false
			if b.nestedLevel != 0 {
				b.nestedLevel--
			}

			b.eatWhitespace(true)
			b.output.AddNewLine(false)

			if b.options.NewlineBetweenRules && !b.output.JustAddedBlankline() {
				if b.input.Peek(0) != "}" {
					b.output.AddNewLine(true)
				}
			}
			if b.input.Peek(0) == ")" {
				b.output.Trim(true)
				if b.options.BraceStyle == "expand" {
					b.output.AddNewLine(true)
				}
			}
		} else if b.ch == ":" {
			for _, property := range nonSemicolonNewlineProperty {
				if b.input.LookBack(property) {
					insideNonSemiColonValues = true
					break
				}
			}

			if (insideRule || enteringConditionalGroup) && !(b.input.LookBack("&") || b.foundNestedPseudoClass()) && !b.input.LookBack("(") && !insideNonNestedAtRule && parenLevel == 0 {
				// The 'property: value' delimiter, possibly in a conditional group query.
				b.printString(":")
				if !insidePropertyValue {
					insidePropertyValue = true
					b.output.SpaceBeforeToken = true
					b.eatWhitespace(true)
					b.indent()
				}
			} else {
				// A sass/less parent reference or a nested pseudo-class: no space.

				// A space before a pseudo-class or pseudo-element means "in any child": keep it.
				if b.input.LookBack(" ") {
					b.output.SpaceBeforeToken = true
				}
				if b.input.Peek(0) == ":" {
					// pseudo-element
					b.ch = b.input.Next()
					b.printString("::")
				} else {
					// pseudo-class
					b.printString(":")
				}
			}
		} else if b.ch == "\"" || b.ch == "'" {
			preserveQuoteSpace := previousCh == "\"" || previousCh == "'"
			b.preserveSingleSpace(preserveQuoteSpace || isAfterSpace)
			ch := b.ch
			b.printString(ch + b.eatString(ch))
			b.eatWhitespace(true)
		} else if b.ch == ";" {
			insideNonSemiColonValues = false
			if parenLevel == 0 {
				if insidePropertyValue {
					b.outdent()
					insidePropertyValue = false
				}
				insideNonNestedAtRule = false
				b.printString(b.ch)
				b.eatWhitespace(true)

				// Keep a line comment on the same line. Block comments get a new line before
				// them anyway.
				if b.input.Peek(0) != "/" {
					b.output.AddNewLine(false)
				}
			} else {
				b.printString(b.ch)
				b.eatWhitespace(true)
				b.output.SpaceBeforeToken = true
			}
		} else if b.ch == "(" {
			// It may be a url.
			if b.input.LookBack("url") {
				b.printString(b.ch)
				b.eatWhitespace(false)
				parenLevel++
				b.indent()
				b.ch = b.input.Next()
				if b.ch == ")" || b.ch == "\"" || b.ch == "'" {
					b.input.Back()
				} else if b.ch != "" {
					ch := b.ch
					b.printString(ch + b.eatString(")"))
					if parenLevel != 0 {
						parenLevel--
						b.outdent()
					}
				}
			} else {
				// lookBack is not exact here: tokens would be needed to rule out whitespace.
				spaceNeeded := b.input.LookBack("with")
				b.preserveSingleSpace(isAfterSpace || spaceNeeded)
				b.printString(b.ch)

				if insidePropertyValue && previousCh == "$" && b.options.SelectorSeparatorNewline {
					// A scss/sass map.
					b.output.AddNewLine(false)
					insideScssMap = true
				} else {
					b.eatWhitespace(false)
					parenLevel++
					b.indent()
				}
			}
		} else if b.ch == ")" {
			if parenLevel != 0 {
				parenLevel--
				b.outdent()
			}
			if insideScssMap && b.input.Peek(0) == ";" && b.options.SelectorSeparatorNewline {
				insideScssMap = false
				b.outdent()
				b.output.AddNewLine(false)
			}
			b.printString(b.ch)
		} else if b.ch == "," {
			b.printString(b.ch)
			b.eatWhitespace(true)
			if b.options.SelectorSeparatorNewline && (!insidePropertyValue || insideScssMap) && parenLevel == 0 && !insideNonNestedAtRule {
				b.output.AddNewLine(false)
			} else {
				b.output.SpaceBeforeToken = true
			}
		} else if (b.ch == ">" || b.ch == "+" || b.ch == "~") && !insidePropertyValue && parenLevel == 0 {
			// Combinator spacing.
			if b.options.SpaceAroundCombinator {
				b.output.SpaceBeforeToken = true
				b.printString(b.ch)
				b.output.SpaceBeforeToken = true
			} else {
				b.printString(b.ch)
				// Squash the whitespace after it.
				b.eatWhitespace(false)
			}
		} else if b.ch == "]" {
			b.printString(b.ch)
		} else if b.ch == "[" {
			b.preserveSingleSpace(isAfterSpace)
			b.printString(b.ch)
		} else if b.ch == "=" {
			// No whitespace before or after.
			b.eatWhitespace(false)
			b.printString("=")
		} else if b.ch == "!" && !b.input.LookBack("\\") {
			// !important
			b.output.SpaceBeforeToken = true
			b.printString(b.ch)
		} else {
			preserveAfterSpace := previousCh == "\"" || previousCh == "'"
			b.preserveSingleSpace(preserveAfterSpace || isAfterSpace)
			b.printString(b.ch)

			if !b.output.JustAddedNewline() && b.input.Peek(0) == "\n" && insideNonSemiColonValues {
				b.output.AddNewLine(false)
			}
		}
	}

	return b.output.GetCode(eol)
}
