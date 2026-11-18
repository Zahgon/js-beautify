package jsvalue

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// SyntaxError is the exception JSON.parse throws; Message is V8's text for it.
type SyntaxError struct {
	Message string
}

func (e *SyntaxError) Error() string { return e.Message }

// jsonToken is the class V8's JSON scanner gives a character.
type jsonToken int

const (
	tokEOS jsonToken = iota
	tokString
	tokNumber
	tokTrue
	tokFalse
	tokNull
	tokWhitespace
	tokColon
	tokComma
	tokLBrack
	tokRBrack
	tokLBrace
	tokRBrace
	tokIllegal
)

func tokenOf(c uint16) jsonToken {
	switch c {
	case '"':
		return tokString
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return tokNumber
	case 't':
		return tokTrue
	case 'f':
		return tokFalse
	case 'n':
		return tokNull
	case ' ', '\t', '\r', '\n':
		return tokWhitespace
	case ':':
		return tokColon
	case ',':
		return tokComma
	case '[':
		return tokLBrack
	case ']':
		return tokRBrack
	case '{':
		return tokLBrace
	case '}':
		return tokRBrace
	}
	return tokIllegal
}

// V8's message templates for JSON.parse failures. reportAt appends
// " at position %d (line %d column %d)" to all but msgUnexpectedEOS.
const (
	msgUnexpectedEOS          = "Unexpected end of JSON input"
	msgUnexpectedNumber       = "Unexpected number in JSON"
	msgUnexpectedString       = "Unexpected string in JSON"
	msgExpectedPropNameOrRBr  = "Expected property name or '}' in JSON"
	msgExpectedCommaOrRBrack  = "Expected ',' or ']' after array element in JSON"
	msgExpectedCommaOrRBrace  = "Expected ',' or '}' after property value in JSON"
	msgExpectedColon          = "Expected ':' after property name in JSON"
	msgExpectedDoubleQuoted   = "Expected double-quoted property name in JSON"
	msgExponentMissingNumber  = "Exponent part is missing a number in JSON"
	msgUnterminatedFraction   = "Unterminated fractional number in JSON"
	msgNoNumberAfterMinus     = "No number after minus sign in JSON"
	msgBadControlCharacter    = "Bad control character in string literal in JSON"
	msgBadUnicodeEscape       = "Bad Unicode escape in JSON"
	msgBadEscapedCharacter    = "Bad escaped character in JSON"
	msgUnterminatedString     = "Unterminated string in JSON"
	msgUnexpectedAfterJSON    = "Unexpected non-whitespace character after JSON"
	maxContextCharacters      = 10
	minSourceLengthForContext = maxContextCharacters*2 + 1
)

// jsonParser follows V8's JsonParser: the same scanning order, so the same error is reported at
// the same position.
type jsonParser struct {
	src    []uint16
	cursor int
	err    *SyntaxError
}

// ParseJSON returns JSON.parse(text): objects become *Object, arrays []any, numbers float64.
// On malformed input it returns a *SyntaxError carrying V8's message.
func ParseJSON(text string) (any, error) {
	p := &jsonParser{src: jsrt.FromUTF8(text).Units()}
	v := p.parseValue()
	if p.err == nil {
		p.skipWhitespace()
		if p.peek() != tokEOS {
			p.reportAt(msgUnexpectedAfterJSON)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	return v, nil
}

func (p *jsonParser) atEnd() bool { return p.cursor >= len(p.src) }

func (p *jsonParser) peek() jsonToken {
	if p.atEnd() {
		return tokEOS
	}
	return tokenOf(p.src[p.cursor])
}

func (p *jsonParser) skipWhitespace() {
	for !p.atEnd() && tokenOf(p.src[p.cursor]) == tokWhitespace {
		p.cursor++
	}
}

// location returns the 1-based line and column of the cursor; \r, \n and \r\n end lines.
func (p *jsonParser) location() (int, int) {
	line, lineStart := 1, 0
	for i := 0; i < p.cursor; i++ {
		if p.src[i] == '\r' && i < p.cursor-1 && p.src[i+1] == '\n' {
			i++
		}
		if p.src[i] == '\r' || p.src[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return line, p.cursor - lineStart + 1
}

// reportAt records a message template that carries the cursor's position.
func (p *jsonParser) reportAt(template string) {
	if p.err != nil {
		return
	}
	line, col := p.location()
	p.err = &SyntaxError{fmt.Sprintf("%s at position %d (line %d column %d)", template, p.cursor, line, col)}
}

// reportToken reports the token at the cursor with the generic message V8 picks for it.
func (p *jsonParser) reportToken(tok jsonToken) {
	if p.err != nil {
		return
	}
	switch tok {
	case tokEOS:
		p.err = &SyntaxError{msgUnexpectedEOS}
		return
	case tokNumber:
		p.reportAt(msgUnexpectedNumber)
		return
	case tokString:
		p.reportAt(msgUnexpectedString)
		return
	}
	src := p.src
	whole := string(jsrt.FromUnits(src).UTF8())
	switch whole {
	case "[object Object]", "undefined", "Infinity", "NaN":
		p.err = &SyntaxError{`"` + whole + `" is not valid JSON`}
		return
	}
	token := string(jsrt.FromUnit(src[p.cursor]).UTF8())
	pos, n := p.cursor, len(src)
	var msg string
	switch {
	case n < minSourceLengthForContext:
		msg = fmt.Sprintf(`Unexpected token '%s', "%s" is not valid JSON`, token, whole)
	case pos < maxContextCharacters:
		msg = fmt.Sprintf(`Unexpected token '%s', "%s"... is not valid JSON`, token, unitsUTF8(src[:pos+maxContextCharacters]))
	case pos < n-maxContextCharacters:
		msg = fmt.Sprintf(`Unexpected token '%s', ..."%s"... is not valid JSON`, token, unitsUTF8(src[pos-maxContextCharacters:pos+maxContextCharacters]))
	default:
		msg = fmt.Sprintf(`Unexpected token '%s', ..."%s" is not valid JSON`, token, unitsUTF8(src[pos-maxContextCharacters:]))
	}
	p.err = &SyntaxError{msg}
}

func unitsUTF8(u []uint16) string {
	return jsrt.FromUnits(u).UTF8()
}

// expectNext skips whitespace and consumes tok, or reports template at the cursor.
func (p *jsonParser) expectNext(tok jsonToken, template string) bool {
	p.skipWhitespace()
	if p.peek() == tok {
		p.cursor++
		return true
	}
	p.reportAt(template)
	return false
}

// check skips whitespace and consumes tok if it is next.
func (p *jsonParser) check(tok jsonToken) bool {
	p.skipWhitespace()
	if p.peek() == tok {
		p.cursor++
		return true
	}
	return false
}

// parseValue parses one value. V8 does this iteratively; recursion gives the same results for
// configuration-sized inputs.
func (p *jsonParser) parseValue() any {
	p.skipWhitespace()
	switch tok := p.peek(); tok {
	case tokString:
		p.cursor++
		s, ok := p.scanString()
		if !ok {
			return nil
		}
		return s
	case tokNumber:
		v, _ := p.parseNumber()
		return v
	case tokLBrace:
		return p.parseObject()
	case tokLBrack:
		return p.parseArray()
	case tokTrue:
		p.scanLiteral("true")
		return true
	case tokFalse:
		p.scanLiteral("false")
		return false
	case tokNull:
		p.scanLiteral("null")
		return nil
	default:
		p.reportToken(tok)
		return nil
	}
}

func (p *jsonParser) parseObject() any {
	p.cursor++
	obj := NewObject()
	if p.check(tokRBrace) {
		return obj
	}
	if !p.expectNext(tokString, msgExpectedPropNameOrRBr) {
		return nil
	}
	for {
		key, ok := p.scanString()
		if !ok || !p.expectNext(tokColon, msgExpectedColon) {
			return nil
		}
		v := p.parseValue()
		if p.err != nil {
			return nil
		}
		obj.Set(key, v)
		if !p.check(tokComma) {
			break
		}
		if !p.expectNext(tokString, msgExpectedDoubleQuoted) {
			return nil
		}
	}
	if !p.expectNext(tokRBrace, msgExpectedCommaOrRBrace) {
		return nil
	}
	return obj
}

func (p *jsonParser) parseArray() any {
	p.cursor++
	arr := []any{}
	if p.check(tokRBrack) {
		return arr
	}
	for {
		v := p.parseValue()
		if p.err != nil {
			return nil
		}
		arr = append(arr, v)
		if !p.check(tokComma) {
			break
		}
	}
	if !p.expectNext(tokRBrack, msgExpectedCommaOrRBrack) {
		return nil
	}
	return arr
}

// scanLiteral consumes the literal word whose first character is at the cursor.
func (p *jsonParser) scanLiteral(word string) {
	remaining := len(p.src) - p.cursor
	if remaining >= len(word) {
		match := true
		for i := 1; i < len(word); i++ {
			if p.src[p.cursor+i] != uint16(word[i]) {
				match = false
				break
			}
		}
		if match {
			p.cursor += len(word)
			return
		}
	}
	p.cursor++
	for i := 1; i < len(word) && i < remaining; i++ {
		if p.src[p.cursor] != uint16(word[i]) {
			p.reportToken(tokenOf(p.src[p.cursor]))
			return
		}
		p.cursor++
	}
	p.reportToken(tokEOS)
}

func isNumberPart(c uint16) bool {
	return jsrt.IsDigit(c) || c == '.' || c == 'e' || c == 'E' || c == '-' || c == '+'
}

// current returns the code unit at the cursor, or -1 at the end.
func (p *jsonParser) current() int {
	if p.atEnd() {
		return -1
	}
	return int(p.src[p.cursor])
}

func (p *jsonParser) next() int {
	p.cursor++
	return p.current()
}

func (p *jsonParser) skipDigits() {
	for !p.atEnd() && jsrt.IsDigit(p.src[p.cursor]) {
		p.cursor++
	}
}

func (p *jsonParser) parseNumber() (float64, bool) {
	start := p.cursor
	c := p.current()
	if c == '-' {
		c = p.next()
	}
	if c == '0' {
		c = p.next()
		if c >= 0 && isNumberPart(uint16(c)) && jsrt.IsDigit(uint16(c)) {
			p.reportToken(tokNumber)
			return 0, false
		}
	} else {
		digitsStart := p.cursor
		p.skipDigits()
		if p.cursor == digitsStart {
			p.reportAt(msgNoNumberAfterMinus)
			return 0, false
		}
	}
	if p.current() == '.' {
		c = p.next()
		if c < 0 || !jsrt.IsDigit(uint16(c)) {
			p.reportAt(msgUnterminatedFraction)
			return 0, false
		}
		p.skipDigits()
	}
	if c = p.current(); c == 'e' || c == 'E' {
		c = p.next()
		if c == '-' || c == '+' {
			c = p.next()
		}
		if c < 0 || !jsrt.IsDigit(uint16(c)) {
			p.reportAt(msgExponentMissingNumber)
			return 0, false
		}
		p.skipDigits()
	}
	// The scanned text is a valid JSON number, so ParseFloat can only report a range error,
	// for which it returns the same ±Inf or ±0 as V8.
	v, _ := strconv.ParseFloat(unitsUTF8(p.src[start:p.cursor]), 64)
	return v, true
}

// scanString scans a string whose opening quote has been consumed.
func (p *jsonParser) scanString() (string, bool) {
	var out []uint16
	for {
		if p.atEnd() {
			p.reportAt(msgUnterminatedString)
			return "", false
		}
		c := p.src[p.cursor]
		switch {
		case c == '"':
			p.cursor++
			return unitsUTF8(out), true
		case c == '\\':
			e := p.next()
			switch e {
			case '"', '\\', '/':
				out = append(out, uint16(e))
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				v := 0
				for i := 0; i < 4; i++ {
					d := hexValue(p.next())
					if d < 0 {
						p.reportAt(msgBadUnicodeEscape)
						return "", false
					}
					v = v*16 + d
				}
				out = append(out, uint16(v))
			default:
				switch {
				case e < 0:
					p.reportToken(tokEOS)
				case e > 0xFF:
					// V8 only classifies Latin-1 characters as escapes.
					p.reportToken(tokIllegal)
				default:
					p.reportAt(msgBadEscapedCharacter)
				}
				return "", false
			}
			p.cursor++
		case c < 0x20:
			p.reportAt(msgBadControlCharacter)
			return "", false
		default:
			out = append(out, c)
			p.cursor++
		}
	}
}

func hexValue(c int) int {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return -1
}

// Stringify returns JSON.stringify(v): objects in key order (undefined properties left out),
// undefined array elements, NaN and infinities as null.
func Stringify(v any) string {
	var b strings.Builder
	stringify(&b, v)
	return b.String()
}

func stringify(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil, undefined:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(string(jsrt.NumberToString(x)))
		}
	case string:
		quoteJSON(b, x)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			stringify(b, e)
		}
		b.WriteByte(']')
	case *Object:
		b.WriteByte('{')
		first := true
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			if IsUndefined(e) {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			quoteJSON(b, k)
			b.WriteByte(':')
			stringify(b, e)
		}
		b.WriteByte('}')
	}
}

func quoteJSON(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
