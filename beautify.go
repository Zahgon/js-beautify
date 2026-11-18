// Package beautify is a Go port of js-beautify 2.0.3: the JavaScript, CSS and HTML beautifiers and
// the js-beautify, css-beautify and html-beautify commands, with the original's output byte for byte.
//
// Options are passed the way the original's JavaScript API receives them: a map whose values are
// nil, bool, float64, string, []any or map[string]any (the types encoding/json decodes into).
// A nil map means no options. Keys may use dashes or underscores ("indent-size" is "indent_size"),
// values are coerced as the original coerces them, and the nested "js", "css" and "html" sections
// override the top-level keys for that language.
package beautify

import (
	"io"

	"github.com/beautifier/js-beautify-go/internal/cli"
	"github.com/beautifier/js-beautify-go/internal/css"
	"github.com/beautifier/js-beautify-go/internal/html"
	"github.com/beautifier/js-beautify-go/internal/javascript"
)

// Version is the js-beautify release this package reproduces.
const Version = cli.Version

// JS beautifies JavaScript source, as js_beautify(source, options) does. The error carries the
// message of the exception the original throws (an invalid option value, for example).
func JS(source string, options map[string]any) (string, error) {
	return javascript.Beautify(source, options)
}

// CSS beautifies CSS, LESS or SCSS source, as css_beautify(source, options) does.
func CSS(source string, options map[string]any) (string, error) {
	return css.Beautify(source, options)
}

// HTML beautifies HTML source, as html_beautify(source, options) does, including the scripts and
// style sheets it embeds.
func HTML(source string, options map[string]any) (string, error) {
	return html.Beautify(source, options, javascript.Beautify, css.Beautify)
}

// CLI runs the command line program called name ("js-beautify", "css-beautify" or
// "html-beautify") with args (without the program name), as the original's executable of that
// name does when run from the current directory with the process environment. It reads standard
// input from stdin, writes to stdout and stderr, and returns the exit status.
func CLI(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return cli.Run(name, args, stdin, stdout, stderr, cli.Beautifiers{JS: JS, CSS: CSS, HTML: HTML})
}
