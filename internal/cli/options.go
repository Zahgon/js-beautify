package cli

import (
	"strings"

	"github.com/beautifier/js-beautify-go/internal/glob"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
	"github.com/beautifier/js-beautify-go/internal/nopt"
)

// knownOpts is the types table js-beautify gives nopt.
var knownOpts = map[string]nopt.TypeSpec{
	// Beautifier
	"indent_size":                 nopt.One(nopt.Number),
	"indent_char":                 nopt.One(nopt.String),
	"eol":                         nopt.One(nopt.String),
	"indent_level":                nopt.One(nopt.Number),
	"indent_with_tabs":            nopt.One(nopt.Boolean),
	"preserve_newlines":           nopt.One(nopt.Boolean),
	"max_preserve_newlines":       nopt.One(nopt.Number),
	"space_in_paren":              nopt.One(nopt.Boolean),
	"space_in_empty_paren":        nopt.One(nopt.Boolean),
	"jslint_happy":                nopt.One(nopt.Boolean),
	"space_after_anon_function":   nopt.One(nopt.Boolean),
	"space_after_named_function":  nopt.One(nopt.Boolean),
	"brace_style":                 nopt.One(braceStyleType),
	"unindent_chained_methods":    nopt.One(nopt.Boolean),
	"break_chained_methods":       nopt.One(nopt.Boolean),
	"keep_array_indentation":      nopt.One(nopt.Boolean),
	"unescape_strings":            nopt.One(nopt.Boolean),
	"wrap_line_length":            nopt.One(nopt.Number),
	"wrap_attributes":             literals("auto", "force", "force-aligned", "force-expand-multiline", "aligned-multiple", "preserve", "preserve-aligned"),
	"wrap_attributes_min_attrs":   nopt.One(nopt.Number),
	"wrap_attributes_indent_size": nopt.One(nopt.Number),
	"e4x":                         nopt.One(nopt.Boolean),
	"end_with_newline":            nopt.One(nopt.Boolean),
	"comma_first":                 nopt.One(nopt.Boolean),
	"operator_position":           literals("before-newline", "after-newline", "preserve-newline"),
	"indent_empty_lines":          nopt.One(nopt.Boolean),
	"templating":                  nopt.List(nopt.String, nopt.Array),
	// CSS-only
	"selector_separator_newline":      nopt.One(nopt.Boolean),
	"newline_between_rules":           nopt.One(nopt.Boolean),
	"space_around_combinator":         nopt.One(nopt.Boolean),
	"space_around_selector_separator": nopt.One(nopt.Boolean),
	// HTML-only
	"max_char":                      nopt.One(nopt.Number),
	"inline":                        nopt.List(nopt.String, nopt.Array),
	"inline_custom_elements":        nopt.List(nopt.Boolean),
	"unformatted":                   nopt.List(nopt.String, nopt.Array),
	"content_unformatted":           nopt.List(nopt.String, nopt.Array),
	"indent_inner_html":             nopt.List(nopt.Boolean),
	"indent_handlebars":             nopt.List(nopt.Boolean),
	"indent_scripts":                literals("keep", "separate", "normal"),
	"extra_liners":                  nopt.List(nopt.String, nopt.Array),
	"unformatted_content_delimiter": nopt.One(nopt.String),
	// CLI
	"version":      nopt.One(nopt.Boolean),
	"help":         nopt.One(nopt.Boolean),
	"files":        nopt.List(globType, nopt.Array),
	"outfile":      nopt.One(nopt.Path),
	"replace":      nopt.One(nopt.Boolean),
	"quiet":        nopt.One(nopt.Boolean),
	"type":         literals("js", "css", "html"),
	"config":       nopt.One(nopt.Path),
	"editorconfig": nopt.One(nopt.Boolean),
}

func literals(values ...string) nopt.TypeSpec {
	ts := make([]nopt.Type, len(values))
	for i, v := range values {
		ts[i] = nopt.Literal(v)
	}
	return nopt.List(ts...)
}

// The custom types js-beautify registers in nopt.typeDefs.
var (
	braceStyleType = nopt.Literal("brace_style")
	globType       = nopt.Literal("glob")
)

// shortHands is js-beautify's shorthand table, including the dashed spellings that
// dasherizeShorthands derives from the one-letter shorthands ("indent-size" for --indent_size).
var shortHands = dasherize(map[string][]string{
	// Beautifier
	"s": {"--indent_size"},
	"c": {"--indent_char"},
	"e": {"--eol"},
	"l": {"--indent_level"},
	"t": {"--indent_with_tabs"},
	"p": {"--preserve_newlines"},
	"m": {"--max_preserve_newlines"},
	"P": {"--space_in_paren"},
	"Q": {"--space_in_empty_paren"},
	"j": {"--jslint_happy"},
	"a": {"--space_after_anon_function"},
	"b": {"--brace_style"},
	"u": {"--unindent_chained_methods"},
	"B": {"--break_chained_methods"},
	"k": {"--keep_array_indentation"},
	"x": {"--unescape_strings"},
	"w": {"--wrap_line_length"},
	"X": {"--e4x"},
	"n": {"--end_with_newline"},
	"C": {"--comma_first"},
	"O": {"--operator_position"},
	// CSS-only
	"L": {"--selector_separator_newline"},
	"N": {"--newline_between_rules"},
	// HTML-only
	"A": {"--wrap_attributes"},
	"M": {"--wrap_attributes_min_attrs"},
	"i": {"--wrap_attributes_indent_size"},
	"W": {"--max_char"},
	"d": {"--inline"},
	"U": {"--unformatted"},
	"T": {"--content_unformatted"},
	"I": {"--indent_inner_html"},
	"H": {"--indent_handlebars"},
	"S": {"--indent_scripts"},
	"E": {"--extra_liners"},
	// non-dasherized hybrid shortcuts
	"good-stuff": {"--keep_array_indentation", "--keep_function_indentation", "--jslint_happy"},
	"js":         {"--type", "js"},
	"css":        {"--type", "css"},
	"html":       {"--type", "html"},
	// CLI
	"v":    {"--version"},
	"h":    {"--help"},
	"f":    {"--files"},
	"file": {"--files"},
	"o":    {"--outfile"},
	"r":    {"--replace"},
	"q":    {"--quiet"},
})

// dasherize adds, for every one-letter shorthand of an underscored option, the option's name
// without dashes and with underscores turned into dashes ("indent-size": ["--indent_size"]).
func dasherize(hash map[string][]string) map[string][]string {
	for key, val := range hash {
		if len(key) == 1 && strings.Contains(val[0], "_") {
			flag := strings.ReplaceAll(strings.TrimLeft(val[0], "-"), "_", "-")
			hash[flag] = []string{val[0]}
		}
	}
	return hash
}

// validBraceStyles are the words js-beautify's brace_style type accepts.
var validBraceStyles = map[string]bool{
	"collapse": true, "collapse-preserve-inline": true, "expand": true, "end-expand": true,
	"expand-strict": true, "none": true, "preserve-inline": true,
}

// validateBraceStyle is js-beautify's brace_style type: every word of the value, split on runs
// of characters other than letters, digits, "_" and "-", must be a brace style.
func validateBraceStyle(v any) (any, bool, error) {
	s, ok := v.(string)
	if !ok {
		return nil, false, nodefmt.NewError("TypeError", "val.split is not a function")
	}
	for _, word := range splitBraceStyle(s) {
		if !validBraceStyles[word] {
			return v, false, nil
		}
	}
	return v, true, nil
}

// splitBraceStyle returns s.split(/[^a-zA-Z0-9_\-]+/).
func splitBraceStyle(s string) []string {
	isWordChar := func(c byte) bool {
		return c == '_' || c == '-' || (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'z')
	}
	var out []string
	start := 0
	for i := 0; i < len(s); {
		if isWordChar(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && !isWordChar(s[j]) {
			j++
		}
		out = append(out, s[start:i])
		start, i = j, j
	}
	return append(out, s[start:])
}

// typeDefs returns nopt.typeDefs as js-beautify extends it: the standard validators, then the
// brace_style and glob types. A glob value with magic is kept as it is; any other value is
// validated as a path.
func typeDefs(cwd, home string) []nopt.TypeDef {
	defs := nopt.StandardTypeDefs(cwd, home)
	return append(defs,
		nopt.TypeDef{Type: braceStyleType, Validate: validateBraceStyle},
		nopt.TypeDef{Type: globType, Validate: func(v any) (any, bool, error) {
			if s, ok := v.(string); ok {
				magic, err := glob.HasMagic(s)
				if err != nil {
					return nil, false, err
				}
				if magic {
					return s, true, nil
				}
			}
			return nopt.ValidatePath(v, cwd, home)
		}},
	)
}
