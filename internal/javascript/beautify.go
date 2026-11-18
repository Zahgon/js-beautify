package javascript

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Beautify formats JavaScript source as js_beautify(source, options) does. The error carries the
// message of the exception the original throws, such as an invalid option value.
func Beautify(source string, options map[string]any) (string, error) {
	b, err := NewBeautifier(jsrt.FromUTF8(source), options)
	if err != nil {
		return "", err
	}
	out, err := b.Beautify()
	if err != nil {
		return "", err
	}
	return out.UTF8(), nil
}
