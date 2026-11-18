package cli

import "strings"

// usageText returns the help of the command called scriptName, as js-beautify's usage() builds
// it: the common options, then those of the beautifier the name selects.
func usageText(scriptName string) string {
	msg := []string{
		scriptName + "@" + Version,
		"",
		"CLI Options:",
		"  -f, --file       Input file(s) (Pass '-' for stdin)",
		"  -r, --replace    Write output in-place, replacing input",
		"  -o, --outfile    Write output to file (default stdout)",
		"  --config         Path to config file",
		`  --type           [js|css|html] ["js"]`,
		"  -q, --quiet      Suppress logging to stdout",
		"  -h, --help       Show this help",
		"  -v, --version    Show the version",
		"",
		"Beautifier Options:",
		"  -s, --indent-size                 Indentation size [4]",
		`  -c, --indent-char                 Indentation character [" "]`,
		"  -t, --indent-with-tabs            Indent with tabs, overrides -s and -c",
		"  -e, --eol                         Character(s) to use as line terminators.",
		`                                    [first newline in file, otherwise "\n]`,
		"  -n, --end-with-newline            End output with newline",
		"  --indent-empty-lines              Keep indentation on empty lines",
		"  --templating                      List of templating languages (auto,none,angular,django,erb,handlebars,php,smarty)",
		`                                    ["auto", auto = none in JavaScript, auto = all except angular in html (and inline javascript/css)]`,
		"  --editorconfig                    Use EditorConfig to set up the options",
	}
	scriptType, _, _ := strings.Cut(scriptName, "-")
	switch scriptType {
	case "js":
		msg = append(msg,
			"  -l, --indent-level                Initial indentation level [0]",
			"  -p, --preserve-newlines           Preserve line-breaks (--no-preserve-newlines disables)",
			"  -m, --max-preserve-newlines       Number of line-breaks to be preserved in one chunk [10]",
			"  -P, --space-in-paren              Add padding spaces within paren, ie. f( a, b )",
			"  -E, --space-in-empty-paren        Add a single space inside empty paren, ie. f( )",
			"  -j, --jslint-happy                Enable jslint-stricter mode",
			"  -a, --space-after-anon-function   Add a space before an anonymous function's parens, ie. function ()",
			"  --space_after_named_function      Add a space before a named function's parens, ie. function example ()",
			"  -b, --brace-style                 [collapse|expand|end-expand|none][,preserve-inline] [collapse,preserve-inline]",
			"  -u, --unindent-chained-methods    Don't indent chained method calls",
			"  -B, --break-chained-methods       Break chained method calls across subsequent lines",
			"  -k, --keep-array-indentation      Preserve array indentation",
			"  -x, --unescape-strings            Decode printable characters encoded in xNN notation",
			"  -w, --wrap-line-length            Wrap lines that exceed N characters [0]",
			"  -X, --e4x                         Pass E4X xml literals through untouched",
			"  --good-stuff                      Warm the cockles of Crockford's heart",
			"  -C, --comma-first                 Put commas at the beginning of new line instead of end",
			"  -O, --operator-position           Set operator position (before-newline|after-newline|preserve-newline) [before-newline]",
		)
	case "html":
		msg = append(msg,
			`  -b, --brace-style                 [collapse|expand|end-expand] ["collapse"]`,
			"  -I, --indent-inner-html           Indent body and head sections. Default is false.",
			"  -H, --indent-handlebars           Indent handlebars. Default is false.",
			`  -S, --indent-scripts              [keep|separate|normal] ["normal"]`,
			"  -w, --wrap-line-length            Wrap lines that exceed N characters [0]",
			`  -A, --wrap-attributes             Wrap html tag attributes to new lines [auto|force|force-aligned|force-expand-multiline|aligned-multiple|preserve|preserve-aligned] ["auto"]`,
			"  -M, --wrap-attributes-min-attrs   Minimum number of html tag attributes for force wrap attribute options [2]",
			"  -i, --wrap-attributes-indent-size Indent wrapped tags to after N characters [indent-level]",
			"  -p, --preserve-newlines           Preserve line-breaks (--no-preserve-newlines disables)",
			"  -m, --max-preserve-newlines       Number of line-breaks to be preserved in one chunk [10]",
			"  -U, --unformatted                 List of tags (defaults to inline) that should not be reformatted",
			"  -T, --content_unformatted         List of tags (defaults to pre) whose content should not be reformatted",
			"  -E, --extra_liners                List of tags (defaults to [head,body,/html] that should have an extra newline",
			`  --unformatted_content_delimiter   Keep text content together between this string [""]`,
		)
	case "css":
		msg = append(msg,
			`  -b, --brace-style                       [collapse|expand] ["collapse"]`,
			"  -L, --selector-separator-newline        Add a newline between multiple selectors.",
			"  -N, --newline-between-rules             Add a newline between CSS rules.",
		)
	}
	return strings.Join(msg, "\n")
}
