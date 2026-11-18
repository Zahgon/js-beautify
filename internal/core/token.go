package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Token is a token produced by a Tokenizer (js/src/core/token.js).
type Token struct {
	Type string
	Text jsrt.String

	// CommentsBefore holds the comments read just before this token, or nil.
	CommentsBefore *TokenStream

	// Newlines is the number of line breaks in the whitespace before the token, and
	// WhitespaceBefore the whitespace after the last of them.
	Newlines         int
	WhitespaceBefore jsrt.String

	// Parent is the innermost open token enclosing this one; Previous and Next link the stream;
	// Opened and Closed link a closing token and the token it closes.
	Parent   *Token
	Next     *Token
	Previous *Token
	Opened   *Token
	Closed   *Token

	// Directives are the beautify directives of a comment token (see Directives), or nil.
	Directives map[jsrt.String]jsrt.String
}

// NewToken returns a token of the given type and text.
func NewToken(typ string, text jsrt.String, newlines int, whitespaceBefore jsrt.String) *Token {
	return &Token{Type: typ, Text: text, Newlines: newlines, WhitespaceBefore: whitespaceBefore}
}
