// Package nodefmt reproduces the parts of the Node runtime that decide what the js-beautify
// command line prints: how console.error shows an exception (util.inspect of an Error and of its
// string properties), the exceptions fs and path functions throw, and how bytes read from files
// and standard input are decoded into strings.
package nodefmt

import (
	"fmt"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Prop is an own enumerable property of an exception, shown by util.inspect after the stack.
// Value is an int or a string.
type Prop struct {
	Key   string
	Value any
}

// Error is a JavaScript exception: its name ("Error", "TypeError",
// "TypeError [ERR_INVALID_ARG_TYPE]"...), its message and the own properties Node adds to it.
type Error struct {
	Name    string
	Message string
	Props   []Prop
	// Frame describes where the exception was thrown; Inspect prints it as the stack frame.
	Frame string
}

// NewError returns an exception with the given name and message.
func NewError(name, message string) *Error {
	return &Error{Name: name, Message: message}
}

// Error returns the exception's message, which is what the original's err.message holds.
func (e *Error) Error() string { return e.Message }

// String returns String(err): "name: message", or the name alone when the message is empty.
func (e *Error) String() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// Inspect returns util.inspect(err): the stack (the String form followed by the frame) and, when
// the exception has own properties, a block listing them, one per line.
func (e *Error) Inspect() string {
	frame := e.Frame
	if frame == "" {
		frame = "<anonymous>"
	}
	s := e.String() + "\n    at " + frame
	if len(e.Props) == 0 {
		return s
	}
	var b strings.Builder
	b.WriteString(s)
	b.WriteString(" {")
	for i, p := range e.Props {
		b.WriteString("\n  ")
		b.WriteString(p.Key)
		b.WriteString(": ")
		switch v := p.Value.(type) {
		case string:
			b.WriteString(QuoteString(v))
		default:
			fmt.Fprint(&b, v)
		}
		if i < len(e.Props)-1 {
			b.WriteByte(',')
		}
	}
	b.WriteString("\n}")
	return b.String()
}

// Prop returns the value of the exception's own property key, or nil.
func (e *Error) Prop(key string) any {
	for _, p := range e.Props {
		if p.Key == key {
			return p.Value
		}
	}
	return nil
}

// QuoteString returns util.inspect(s) for a string: single quotes unless s holds a single quote,
// then double quotes if s has none, else backticks if s has neither a backtick nor "${", else
// single quotes with \' escapes. Control characters, backslashes and lone surrogates are escaped.
func QuoteString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") {
		if !strings.Contains(s, `"`) {
			quote = '"'
		} else if !strings.Contains(s, "`") && !strings.Contains(s, "${") {
			quote = '`'
		}
	}
	units := jsrt.FromUTF8(s).Units()
	var b strings.Builder
	b.WriteByte(quote)
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == '\'' && quote == '\'':
			b.WriteString(`\'`)
		case u == '\\':
			b.WriteString(`\\`)
		case u == '\b':
			b.WriteString(`\b`)
		case u == '\t':
			b.WriteString(`\t`)
		case u == '\n':
			b.WriteString(`\n`)
		case u == '\f':
			b.WriteString(`\f`)
		case u == '\r':
			b.WriteString(`\r`)
		case u < 0x20 || (u > 0x7E && u < 0xA0):
			fmt.Fprintf(&b, `\x%02X`, u)
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			b.WriteString(string(jsrt.FromUnits(units[i : i+2]).UTF8()))
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			fmt.Fprintf(&b, `\u%x`, u)
		default:
			b.WriteString(jsrt.FromUnit(u).UTF8())
		}
	}
	b.WriteByte(quote)
	return b.String()
}
