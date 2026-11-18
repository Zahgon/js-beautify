// Package html is a port of js-beautify's HTML beautifier (js/src/html), which also formats the
// scripts and style sheets a document embeds through the JavaScript and CSS beautifiers.
package html

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Func is a child beautifier (JavaScript or CSS) the HTML beautifier calls for <script> and <style>.
type Func = func(source string, options map[string]any) (string, error)

// Beautify formats HTML source as html_beautify(source, options) does, formatting embedded
// scripts with js and style sheets with css. The error carries the message of the exception the
// original throws, such as an invalid option value, including those of js and css.
func Beautify(source string, options map[string]any, js, css Func) (string, error) {
	b, err := NewBeautifier(jsrt.FromUTF8(source), options, js, css)
	if err != nil {
		return "", err
	}
	out, err := b.Beautify()
	if err != nil {
		return "", err
	}
	return out.UTF8(), nil
}
