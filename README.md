# js-beautify for Go

A Go port of [js-beautify](https://github.com/beautifier/js-beautify) 2.0.3: the JavaScript, CSS and
HTML beautifiers and the `js-beautify`, `css-beautify` and `html-beautify` commands. It produces the
same output as the JavaScript release, byte for byte, including the options it accepts, the
`.jsbeautifyrc`, environment and EditorConfig settings the commands read, and the messages they print.
The module needs nothing beyond the Go standard library.

# Installation

```sh
go get github.com/beautifier/js-beautify-go
go install github.com/beautifier/js-beautify-go/cmd/...@latest   # js-beautify, css-beautify, html-beautify
```

# Usage

## Library

```go
import beautify "github.com/beautifier/js-beautify-go"

out, err := beautify.JS(source, map[string]any{"indent_size": 2, "brace_style": "expand"})
css, err := beautify.CSS(source, nil)
html, err := beautify.HTML(source, map[string]any{"wrap_attributes": "force", "js": map[string]any{"indent_size": 2}})
```

Options are the JavaScript options object: a `map[string]any` holding JSON-typed values (`bool`,
`float64`, `string`, `[]any`, `map[string]any`, `nil`). Names may use dashes or underscores, values are
coerced as the JavaScript release coerces them, and an invalid value returns the same error message.

## Command line

```sh
js-beautify foo.js
css-beautify -r styles/*.css
html-beautify --wrap-attributes force -o out.html index.html
cat foo.js | js-beautify -s 2 -
```

`beautify.CLI(name, args, stdin, stdout, stderr)` runs any of the three commands in-process and returns
the exit status.

# Options

These are the command-line flags of the commands:

```text
CLI Options:
  -f, --file       Input file(s) (Pass '-' for stdin)
  -r, --replace    Write output in-place, replacing input
  -o, --outfile    Write output to file (default stdout)
  --config         Path to config file
  --type           [js|css|html] ["js"] Select beautifier type (NOTE: Does *not* filter files, only defines which beautifier type to run)
  -q, --quiet      Suppress logging to stdout
  -h, --help       Show this help
  -v, --version    Show the version

Beautifier Options:
  -s, --indent-size                 Indentation size [4]
  -c, --indent-char                 Indentation character [" "]
  -t, --indent-with-tabs            Indent with tabs, overrides -s and -c
  -e, --eol                         Character(s) to use as line terminators.
                                    [first newline in file, otherwise "\n]
  -n, --end-with-newline            End output with newline
  --editorconfig                    Use EditorConfig to set up the options
  -l, --indent-level                Initial indentation level [0]
  -p, --preserve-newlines           Preserve line-breaks (--no-preserve-newlines disables)
  -m, --max-preserve-newlines       Number of line-breaks to be preserved in one chunk [10]
  -P, --space-in-paren              Add padding spaces within paren, ie. f( a, b )
  -E, --space-in-empty-paren        Add a single space inside empty paren, ie. f( )
  -j, --jslint-happy                Enable jslint-stricter mode
  -a, --space-after-anon-function   Add a space before an anonymous function's parens, ie. function ()
  --space-after-named-function      Add a space before a named function's parens, i.e. function example ()
  -b, --brace-style                 [collapse|expand|end-expand|none][,preserve-inline] [collapse,preserve-inline]
  -u, --unindent-chained-methods    Don't indent chained method calls
  -B, --break-chained-methods       Break chained method calls across subsequent lines
  -k, --keep-array-indentation      Preserve array indentation
  -x, --unescape-strings            Decode printable characters encoded in xNN notation
  -w, --wrap-line-length            Wrap lines that exceed N characters [0]
  -X, --e4x                         Pass E4X xml literals through untouched
  --good-stuff                      Warm the cockles of Crockford's heart
  -C, --comma-first                 Put commas at the beginning of new line instead of end
  -O, --operator-position           Set operator position (before-newline|after-newline|preserve-newline) [before-newline]
  --indent-empty-lines              Keep indentation on empty lines
  --templating                      List of templating languages (auto,django,erb,handlebars,php,smarty,angular) ["auto"] auto = none in JavaScript, all in HTML
```

Which correspond to the underscored option keys of the library functions

**defaults per CLI options**
```json
{
    "indent_size": 4,
    "indent_char": " ",
    "indent_with_tabs": false,
    "editorconfig": false,
    "eol": "\n",
    "end_with_newline": false,
    "indent_level": 0,
    "preserve_newlines": true,
    "max_preserve_newlines": 10,
    "space_in_paren": false,
    "space_in_empty_paren": false,
    "jslint_happy": false,
    "space_after_anon_function": false,
    "space_after_named_function": false,
    "brace_style": "collapse",
    "unindent_chained_methods": false,
    "break_chained_methods": false,
    "keep_array_indentation": false,
    "unescape_strings": false,
    "wrap_line_length": 0,
    "e4x": false,
    "comma_first": false,
    "operator_position": "before-newline",
    "indent_empty_lines": false,
    "templating": ["auto"]
}
```

**defaults not exposed in the cli**
```json
{
  "eval_code": false,
  "space_before_conditional": true
}
```

Notice not all defaults are exposed via the CLI.


## Loading settings from environment or .jsbeautifyrc

In addition to CLI arguments, you may pass config to the commands via:

 * any `jsbeautify_`-prefixed environment variables
 * a `JSON`-formatted file indicated by the `--config` parameter
 * a `.jsbeautifyrc` file containing `JSON` data at any level of the filesystem above `$PWD`

Configuration sources provided earlier in this stack will override later ones.

## Setting inheritance and Language-specific overrides

The settings are a shallow tree whose values are inherited for all languages, but
can be overridden.  This works for settings passed directly to the library functions and for settings loaded from a config file, such as .jsbeautifyrc.

Below is an example configuration tree showing all the supported locations
for language override nodes.  We'll use `indent_size` to discuss how this configuration would behave, but any number of settings can be inherited or overridden:

```json
{
    "indent_size": 4,
    "html": {
        "end_with_newline": true,
        "js": {
            "indent_size": 2
        },
        "css": {
            "indent_size": 2
        }
    },
    "css": {
        "indent_size": 1
    },
    "js": {
       "preserve-newlines": true
    }
}
```

Using the above example would have the following result:

* HTML files
  * Inherit `indent_size` of 4 spaces from the top-level setting.
  * The files would also end with a newline.
  * JavaScript and CSS inside HTML
    * Inherit the HTML `end_with_newline` setting.
    * Override their indentation to 2 spaces.
* CSS files
  * Override the top-level setting to an `indent_size` of 1 space.
* JavaScript files
  * Inherit `indent_size` of 4 spaces from the top-level setting.
  * Set `preserve-newlines` to `true`.

## CSS & HTML

In addition to the `js-beautify` executable, `css-beautify` and `html-beautify`
are also provided as an easy interface into those scripts. Alternatively,
`js-beautify --css` or `js-beautify --html` will accomplish the same thing, respectively.

```go
// Programmatic access: beautify.JS, beautify.CSS and beautify.HTML all take the source and an options map.
```

The CSS & HTML beautifiers are much simpler in scope, and possess far fewer options.

```text
CSS Beautifier Options:
  -s, --indent-size                  Indentation size [4]
  -c, --indent-char                  Indentation character [" "]
  -t, --indent-with-tabs             Indent with tabs, overrides -s and -c
  -e, --eol                          Character(s) to use as line terminators. (default newline - "\\n")
  -n, --end-with-newline             End output with newline
  -b, --brace-style                  [collapse|expand] ["collapse"]
  -L, --selector-separator-newline   Add a newline between multiple selectors
  -N, --newline-between-rules        Add a newline between CSS rules
  --indent-empty-lines               Keep indentation on empty lines

HTML Beautifier Options:
  -s, --indent-size                  Indentation size [4]
  -c, --indent-char                  Indentation character [" "]
  -t, --indent-with-tabs             Indent with tabs, overrides -s and -c
  -e, --eol                          Character(s) to use as line terminators. (default newline - "\\n")
  -n, --end-with-newline             End output with newline
  -p, --preserve-newlines            Preserve existing line-breaks (--no-preserve-newlines disables)
  -m, --max-preserve-newlines        Maximum number of line-breaks to be preserved in one chunk [10]
  -I, --indent-inner-html            Indent <head> and <body> sections. Default is false.
  -b, --brace-style                  [collapse-preserve-inline|collapse|expand|end-expand|none] ["collapse"]
  -S, --indent-scripts               [keep|separate|normal] ["normal"]
  -w, --wrap-line-length             Maximum characters per line (0 disables) [250]
  -A, --wrap-attributes              Wrap attributes to new lines [auto|force|force-aligned|force-expand-multiline|aligned-multiple|preserve|preserve-aligned] ["auto"]
  -M, --wrap-attributes-min-attrs    Minimum number of html tag attributes for force wrap attribute options [2]
  -i, --wrap-attributes-indent-size  Indent wrapped attributes to after N characters [indent-size] (ignored if wrap-attributes is "aligned")
  -d, --inline                       List of tags to be considered inline tags
  --inline_custom_elements           Inline custom elements [true]
  -U, --unformatted                  List of tags (defaults to inline) that should not be reformatted
  -T, --content_unformatted          List of tags (defaults to pre) whose content should not be reformatted
  -E, --extra_liners                 List of tags (defaults to [head,body,/html] that should have an extra newline before them.
  --editorconfig                     Use EditorConfig to set up the options
  --indent_scripts                   Sets indent level inside script tags ("normal", "keep", "separate")
  --unformatted_content_delimiter    Keep text content together between this string [""]
  --indent-empty-lines               Keep indentation on empty lines
  --templating                       List of templating languages (auto,none,django,erb,handlebars,php,smarty,angular) ["auto"] auto = none in JavaScript, all in html
```

## Directives

Directives let you control the behavior of the Beautifier from within your source files. Directives are placed in comments inside the file.  Directives are in the format `/* beautify {name}:{value} */` in CSS and JavaScript. In HTML they are formatted as `<!-- beautify {name}:{value} -->`.

### Ignore directive

The `ignore` directive makes the beautifier completely ignore part of a file, treating it as literal text that is not parsed.
The input below will remain unchanged after beautification:

```js
// Use ignore when the content is not parsable in the current language, JavaScript in this case.
var a =  1;
/* beautify ignore:start */
 {This is some strange{template language{using open-braces?
/* beautify ignore:end */
```

### Preserve directive

NOTE: this directive only works in HTML and JavaScript, not CSS.

The `preserve` directive makes the Beautifier parse and then keep the existing formatting of a section of code.

The input below will remain unchanged after beautification:

```js
// Use preserve when the content is valid syntax in the current language, JavaScript in this case.
// This will parse the code and preserve the existing formatting.
/* beautify preserve:start */
{
    browserName: 'internet explorer',
    platform:    'Windows 7',
    version:     '8'
}
/* beautify preserve:end */
```

# License

You are free to use this in any way you want, in case you find this useful or working for you but you must keep the copyright notice and license. (MIT)

# Credits

* Created by Einar Lielmanis, <einar@beautifier.io>
* Python version flourished by Stefano Sanfilippo <a.little.coder@gmail.com>
* Command-line by Daniel Stockman <daniel.stockman@gmail.com>
* Maintained and expanded by Liam Newman <bitwiseman@beautifier.io>

Thanks also to Jason Diamond, Patrick Hof, Nochum Sossonko, Andreas Schneider, Dave
Vasilevsky, Vital Batmanov, Ron Baldwin, Gabriel Harrison, Chris J. Shull,
Mathias Bynens, Vittorio Gambaletta and others.

(README.md: js-beautify@2.0.3, Go port)
