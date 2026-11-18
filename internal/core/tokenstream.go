package core

// TokenStream is a sequence of tokens with a read position (js/src/core/tokenstream.js).
type TokenStream struct {
	tokens      []*Token
	position    int
	parentToken *Token
}

// NewTokenStream returns an empty stream; tokens added to it get parentToken as their parent
// when it is not nil.
func NewTokenStream(parentToken *Token) *TokenStream {
	return &TokenStream{parentToken: parentToken}
}

// Restart moves the read position back to the first token.
func (s *TokenStream) Restart() {
	s.position = 0
}

// IsEmpty reports whether the stream holds no tokens.
func (s *TokenStream) IsEmpty() bool {
	return len(s.tokens) == 0
}

// HasNext reports whether tokens remain to be read.
func (s *TokenStream) HasNext() bool {
	return s.position < len(s.tokens)
}

// Next returns the next token and advances, or nil at the end.
func (s *TokenStream) Next() *Token {
	if !s.HasNext() {
		return nil
	}
	s.position++
	return s.tokens[s.position-1]
}

// Peek returns the token index places after the read position (Peek(0) is the next one), or nil.
func (s *TokenStream) Peek(index int) *Token {
	index += s.position
	if index >= 0 && index < len(s.tokens) {
		return s.tokens[index]
	}
	return nil
}

// Add appends a token.
func (s *TokenStream) Add(token *Token) {
	if s.parentToken != nil {
		token.Parent = s.parentToken
	}
	s.tokens = append(s.tokens, token)
}
