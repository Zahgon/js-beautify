package core

import (
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Options holds the settings every beautifier shares (js/src/core/options.js). Language options
// embed it, call NewOptions with their section name, then read their own settings from Raw with
// the Get* methods, in the original's order so that the same error surfaces first.
type Options struct {
	// Raw is raw_options: the caller's options with dashes in top-level keys turned into
	// underscores and the language section merged over them. Language options may rewrite
	// entries before reading them, as the JavaScript options do for deprecated brace styles.
	Raw map[string]any

	Disabled            bool
	EOL                 jsrt.String
	EndWithNewline      bool
	IndentSize          int
	IndentChar          jsrt.String
	IndentLevel         int
	PreserveNewlines    bool
	MaxPreserveNewlines int
	IndentWithTabs      bool
	WrapLineLength      int
	IndentEmptyLines    bool
	Templating          []jsrt.String
}

// NewOptions reads the shared settings from options, merging the mergeChildField section
// ("js", "css" or "html"; "" for none) over the top level.
func NewOptions(options map[string]any, mergeChildField string) (*Options, error) {
	o := &Options{Raw: MergeOpts(options, mergeChildField)}

	// Support passing the source text back with no change.
	o.Disabled = o.GetBoolean("disabled", false)

	o.EOL = o.GetCharacters("eol", "auto")
	o.EndWithNewline = o.GetBoolean("end_with_newline", false)
	o.IndentSize = o.GetNumber("indent_size", 4)
	o.IndentChar = o.GetCharacters("indent_char", " ")
	o.IndentLevel = o.GetNumber("indent_level", 0)

	o.PreserveNewlines = o.GetBoolean("preserve_newlines", true)
	o.MaxPreserveNewlines = o.GetNumber("max_preserve_newlines", 32786)
	if !o.PreserveNewlines {
		o.MaxPreserveNewlines = 0
	}

	o.IndentWithTabs = o.GetBoolean("indent_with_tabs", o.IndentChar == "\t")
	if o.IndentWithTabs {
		o.IndentChar = "\t"
		// Since 1.8.6 an indent_size of 1 with tabs means the default of 4.
		if o.IndentSize == 1 {
			o.IndentSize = 4
		}
	}

	// max_char is the 1.3.x name of wrap_line_length.
	o.WrapLineLength = o.GetNumber("wrap_line_length", o.GetNumber("max_char", 0))

	o.IndentEmptyLines = o.GetBoolean("indent_empty_lines", false)

	// 'auto' means all templating off for JavaScript and CSS, and all but angular on for HTML.
	var err error
	o.Templating, err = o.GetSelectionList("templating",
		[]jsrt.String{"auto", "none", "angular", "django", "erb", "handlebars", "php", "smarty"},
		[]jsrt.String{"auto"})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// arrayValue is _get_array before the elements are known to be strings: the elements of an
// array option (strings as jsrt.String), the pieces of a string option split on every run of characters other than
// [a-zA-Z0-9_/-], or the default.
func (o *Options) arrayValue(name string, defaultValue []jsrt.String) []any {
	var result []any
	switch v := o.Raw[name].(type) {
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				result = append(result, jsrt.FromUTF8(s))
			} else {
				result = append(result, e)
			}
		}
		return result
	case string:
		for _, s := range arraySplitPattern.Split(jsrt.FromUTF8(v)) {
			result = append(result, s)
		}
		return result
	}
	for _, s := range defaultValue {
		result = append(result, s)
	}
	return result
}

var arraySplitPattern = jsrt.MustCompile(`[^a-zA-Z0-9_\/\-]+`, "")

// GetArray returns _get_array(name, defaultValue). The original keeps non-string elements of an
// array option, but only ever tests the array for membership of strings with strict equality,
// which such an element never satisfies, so they are left out here.
func (o *Options) GetArray(name string, defaultValue []jsrt.String) []jsrt.String {
	result := []jsrt.String{}
	for _, v := range o.arrayValue(name, defaultValue) {
		if s, ok := v.(jsrt.String); ok {
			result = append(result, s)
		}
	}
	return result
}

// GetBoolean returns _get_boolean(name, defaultValue): the truthiness of the option when it is
// present.
func (o *Options) GetBoolean(name string, defaultValue bool) bool {
	v, ok := o.Raw[name]
	if !ok {
		return defaultValue
	}
	return jsrt.Truthy(v)
}

// GetCharacters returns _get_characters(name, defaultValue): a string option with the first
// literal \r, \n and \t (backslash sequences) turned into the characters; the default for any
// other type.
func (o *Options) GetCharacters(name string, defaultValue jsrt.String) jsrt.String {
	s, ok := o.Raw[name].(string)
	if !ok {
		return defaultValue
	}
	return jsrt.FromUTF8(s).Replace(`\r`, "\r").Replace(`\n`, "\n").Replace(`\t`, "\t")
}

// GetNumber returns _get_number(name, defaultValue): parseInt(String(value), 10), or the default
// when that is NaN. Magnitudes beyond 2^53 are clamped; every use of such a value behaves the
// same either way.
func (o *Options) GetNumber(name string, defaultValue int) int {
	v, ok := o.Raw[name]
	if !ok {
		return defaultValue
	}
	n := jsrt.ParseInt(jsrt.ToString(v), 10)
	if math.IsNaN(n) {
		return defaultValue
	}
	const limit = 1 << 53
	return int(math.Max(-limit, math.Min(limit, n)))
}

// GetSelection returns _get_selection: the one value of a selection option.
func (o *Options) GetSelection(name string, selectionList, defaultValue []jsrt.String) (jsrt.String, error) {
	result, err := o.GetSelectionList(name, selectionList, defaultValue)
	if err != nil {
		return "", err
	}
	if len(result) != 1 {
		return "", errors.New("Invalid Option Value: The option '" + name + "' can only be one of the following values:\n" +
			listString(selectionList) + "\nYou passed in: '" + o.rawString(name) + "'")
	}
	return result[0], nil
}

// GetSelectionList returns _get_selection_list: the values of an option that must all come from
// selectionList. A nil defaultValue means the first entry of the list.
func (o *Options) GetSelectionList(name string, selectionList, defaultValue []jsrt.String) ([]jsrt.String, error) {
	if len(selectionList) == 0 {
		return nil, errors.New("Selection list cannot be empty.")
	}
	if len(defaultValue) == 0 {
		defaultValue = selectionList[:1]
	}
	if !IsValidSelection(stringsToValues(defaultValue), selectionList) {
		return nil, errors.New("Invalid Default Value!")
	}
	result := o.arrayValue(name, defaultValue)
	if !IsValidSelection(result, selectionList) {
		return nil, errors.New("Invalid Option Value: The option '" + name + "' can contain only the following values:\n" +
			listString(selectionList) + "\nYou passed in: '" + o.rawString(name) + "'")
	}
	out := make([]jsrt.String, len(result))
	for i, v := range result {
		out[i] = v.(jsrt.String)
	}
	return out, nil
}

// IsValidSelection returns _is_valid_selection: result is not empty and every element is a
// string from selectionList.
func IsValidSelection(result []any, selectionList []jsrt.String) bool {
	if len(result) == 0 || len(selectionList) == 0 {
		return false
	}
	for _, item := range result {
		s, ok := item.(jsrt.String)
		if !ok || !containsString(selectionList, s) {
			return false
		}
	}
	return true
}

func stringsToValues(list []jsrt.String) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

func containsString(list []jsrt.String, s jsrt.String) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// listString is String(array) for a list of strings.
func listString(list []jsrt.String) string {
	return jsrt.Join(list, ",").UTF8()
}

// rawString is the raw option value as JavaScript string concatenation writes it.
func (o *Options) rawString(name string) string {
	v, ok := o.Raw[name]
	if !ok {
		return "undefined"
	}
	return jsrt.ToString(v).UTF8()
}

// MergeOpts returns _mergeOpts(allOptions, childFieldName): the normalized top-level options
// without the child section, overridden by the entries of the child section when it is an
// object. The child section's keys are copied as they are, without normalization, as the
// original does.
func MergeOpts(allOptions map[string]any, childFieldName string) map[string]any {
	finalOpts := map[string]any{}
	all := NormalizeOpts(allOptions)
	for name, v := range all {
		if name != childFieldName {
			finalOpts[name] = v
		}
	}
	if childFieldName != "" {
		if child, ok := all[childFieldName]; ok && jsrt.Truthy(child) {
			for name, v := range childEntries(child) {
				finalOpts[name] = v
			}
		}
	}
	return finalOpts
}

// childEntries returns what for-in enumerates on a merged section: an object's entries, an
// array's elements or a string's characters under their index; nothing for other values.
func childEntries(v any) map[string]any {
	out := map[string]any{}
	switch c := v.(type) {
	case map[string]any:
		return c
	case []any:
		for i, e := range c {
			out[string(jsrt.NumberToString(float64(i)))] = e
		}
	case string:
		for i, u := range jsrt.FromUTF8(c).Units() {
			out[string(jsrt.NumberToString(float64(i)))] = string(jsrt.FromUnits([]uint16{u}).UTF8())
		}
	}
	return out
}

// NormalizeOpts returns _normalizeOpts(options): the same options with every "-" in a key
// replaced by "_". When two keys normalize alike ("end_with_newline" and "end-with-newline")
// JavaScript keeps the one inserted last. A Go map has no insertion order; the dashed spelling
// is taken as the later one (options are normally written in underscore form, with the
// command-line style spelling added as an override), and keys are otherwise taken in sorted
// order.
func NormalizeOpts(options map[string]any) map[string]any {
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		di, dj := strings.Contains(keys[i], "-"), strings.Contains(keys[j], "-")
		if di != dj {
			return dj
		}
		return keys[i] < keys[j]
	})
	converted := make(map[string]any, len(options))
	for _, k := range keys {
		converted[strings.ReplaceAll(k, "-", "_")] = options[k]
	}
	return converted
}
