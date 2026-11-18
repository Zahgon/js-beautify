package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Token types shared by every tokenizer.
const (
	TokenStart = "TK_START"
	TokenRaw   = "TK_RAW"
	TokenEOF   = "TK_EOF"
)

// Lexer is the set of methods a language tokenizer overrides (the original's subclasses
// override _reset, _get_next_token, _is_comment, _is_opening and _is_closing). *Tokenizer
// implements all of them with the base behaviour, so a language tokenizer embeds *Tokenizer,
// redefines the methods it overrides, and runs Tokenize(itself):
//
//	type Tokenizer struct {
//		*core.Tokenizer
//		// language patterns and state
//	}
//
//	func (t *Tokenizer) GetNextToken(previous, open *core.Token) *core.Token { ... }
//
//	tokens := t.Tokenize(t)
type Lexer interface {
	Reset()
	GetNextToken(previousToken, openToken *Token) *Token
	IsComment(currentToken *Token) bool
	IsOpening(currentToken *Token) bool
	IsClosing(currentToken, openToken *Token) bool
}

// Tokenizer is the base tokenizer (js/src/core/tokenizer.js).
type Tokenizer struct {
	// Input scans the source; Whitespace is the whitespace pattern (_patterns.whitespace), which
	// a language tokenizer may replace with Whitespace.Matching(...).
	Input      *InputScanner
	Whitespace *WhitespacePattern

	tokens *TokenStream
}

// NewTokenizer returns a tokenizer over input.
func NewTokenizer(input jsrt.String) *Tokenizer {
	in := NewInputScanner(input)
	return &Tokenizer{Input: in, Whitespace: NewWhitespacePattern(in)}
}

// Tokenize reads the whole input into a token stream, using lexer (normally the language
// tokenizer embedding t) for the overridable steps. Comments are attached to the following
// token, and opening and closing tokens are paired.
func (t *Tokenizer) Tokenize(lexer Lexer) *TokenStream {
	t.Input.Restart()
	t.tokens = NewTokenStream(nil)

	lexer.Reset()

	var current *Token
	previous := NewToken(TokenStart, "", 0, "")
	var openToken *Token
	var openStack []*Token
	comments := NewTokenStream(nil)

	for previous.Type != TokenEOF {
		current = lexer.GetNextToken(previous, openToken)
		for lexer.IsComment(current) {
			comments.Add(current)
			current = lexer.GetNextToken(previous, openToken)
		}

		if !comments.IsEmpty() {
			current.CommentsBefore = comments
			comments = NewTokenStream(nil)
		}

		current.Parent = openToken

		if lexer.IsOpening(current) {
			openStack = append(openStack, openToken)
			openToken = current
		} else if openToken != nil && lexer.IsClosing(current, openToken) {
			current.Opened = openToken
			openToken.Closed = current
			openToken = nil
			if n := len(openStack); n > 0 {
				openToken = openStack[n-1]
				openStack = openStack[:n-1]
			}
			current.Parent = openToken
		}

		current.Previous = previous
		previous.Next = current

		t.tokens.Add(current)
		previous = current
	}

	return t.tokens
}

// IsFirstToken reports whether no token has been produced yet (_is_first_token).
func (t *Tokenizer) IsFirstToken() bool {
	return t.tokens.IsEmpty()
}

// Reset is the base _reset: nothing to reset.
func (t *Tokenizer) Reset() {}

// GetNextToken is the base _get_next_token: everything up to the end of the line is one raw
// token.
func (t *Tokenizer) GetNextToken(previousToken, openToken *Token) *Token {
	t.ReadWhitespace()
	resultingString := t.Input.Read(anyLinePattern, nil, false)
	if resultingString != "" {
		return t.CreateToken(TokenRaw, resultingString)
	}
	return t.CreateToken(TokenEOF, "")
}

var anyLinePattern = jsrt.MustCompile(`.+`, "g")

// IsComment is the base _is_comment: no token is a comment.
func (t *Tokenizer) IsComment(currentToken *Token) bool { return false }

// IsOpening is the base _is_opening: no token opens a block.
func (t *Tokenizer) IsOpening(currentToken *Token) bool { return false }

// IsClosing is the base _is_closing: no token closes a block.
func (t *Tokenizer) IsClosing(currentToken, openToken *Token) bool { return false }

// CreateToken returns a token with the newlines and whitespace read by the last ReadWhitespace.
func (t *Tokenizer) CreateToken(typ string, text jsrt.String) *Token {
	return NewToken(typ, text, t.Whitespace.NewlineCount, t.Whitespace.WhitespaceBeforeToken)
}

// ReadWhitespace reads the whitespace at the current position (_readWhitespace).
func (t *Tokenizer) ReadWhitespace() jsrt.String {
	return t.Whitespace.Read()
}
