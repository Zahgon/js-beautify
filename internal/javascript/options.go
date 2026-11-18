package javascript

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// The values of operator_position.
const (
	operatorBeforeNewline   jsrt.String = "before-newline"
	operatorAfterNewline    jsrt.String = "after-newline"
	operatorPreserveNewline jsrt.String = "preserve-newline"
)

var validPositionValues = []jsrt.String{operatorBeforeNewline, operatorAfterNewline, operatorPreserveNewline}

// Options are the JavaScript beautifier's settings (js/src/javascript/options.js).
type Options struct {
	*core.Options

	// BraceStyle is "collapse", "expand", "end-expand" or "none"; BracePreserveInline is set
	// when "preserve-inline" is listed too.
	BraceStyle          jsrt.String
	BracePreserveInline bool

	UnindentChainedMethods  bool
	BreakChainedMethods     bool
	SpaceInParen            bool
	SpaceInEmptyParen       bool
	JslintHappy             bool
	SpaceAfterAnonFunction  bool
	SpaceAfterNamedFunction bool
	KeepArrayIndentation    bool
	SpaceBeforeConditional  bool
	UnescapeStrings         bool
	E4X                     bool
	CommaFirst              bool
	OperatorPosition        jsrt.String

	// TestOutputRaw starts the output in raw mode, as upstream's tests of the
	// preserve:start directive do.
	TestOutputRaw bool
}

// NewOptions reads the JavaScript options, with the "js" section merged over the top level.
// The error is the original's "Invalid Option Value" message for a bad selection.
func NewOptions(options map[string]any) (*Options, error) {
	base, err := core.NewOptions(options, "js")
	if err != nil {
		return nil, err
	}
	o := &Options{Options: base}

	// Deprecated spellings of brace_style are rewritten in the raw options. Only the string
	// values themselves are recognised (the original compares with ===).
	rawBraceStyle, _ := o.Raw["brace_style"].(string)
	if rawBraceStyle == "expand-strict" {
		o.Raw["brace_style"] = "expand"
	} else if rawBraceStyle == "collapse-preserve-inline" {
		o.Raw["brace_style"] = "collapse,preserve-inline"
	} else if ownLine, ok := o.Raw["braces_on_own_line"]; ok {
		if jsrt.Truthy(ownLine) {
			o.Raw["brace_style"] = "expand"
		} else {
			o.Raw["brace_style"] = "collapse"
		}
	}

	// "preserve-inline" anywhere in the list sets BracePreserveInline; of the other styles the
	// last one listed wins.
	braceStyleSplit, err := o.GetSelectionList("brace_style",
		[]jsrt.String{"collapse", "expand", "end-expand", "none", "preserve-inline"}, nil)
	if err != nil {
		return nil, err
	}
	o.BraceStyle = "collapse"
	for _, bs := range braceStyleSplit {
		if bs == "preserve-inline" {
			o.BracePreserveInline = true
		} else {
			o.BraceStyle = bs
		}
	}

	o.UnindentChainedMethods = o.GetBoolean("unindent_chained_methods", false)
	o.BreakChainedMethods = o.GetBoolean("break_chained_methods", false)
	o.SpaceInParen = o.GetBoolean("space_in_paren", false)
	o.SpaceInEmptyParen = o.GetBoolean("space_in_empty_paren", false)
	o.JslintHappy = o.GetBoolean("jslint_happy", false)
	o.SpaceAfterAnonFunction = o.GetBoolean("space_after_anon_function", false)
	o.SpaceAfterNamedFunction = o.GetBoolean("space_after_named_function", false)
	o.KeepArrayIndentation = o.GetBoolean("keep_array_indentation", false)
	o.SpaceBeforeConditional = o.GetBoolean("space_before_conditional", true)
	o.UnescapeStrings = o.GetBoolean("unescape_strings", false)
	o.E4X = o.GetBoolean("e4x", false)
	o.CommaFirst = o.GetBoolean("comma_first", false)
	o.OperatorPosition, err = o.GetSelection("operator_position", validPositionValues, nil)
	if err != nil {
		return nil, err
	}

	o.TestOutputRaw = o.GetBoolean("test_output_raw", false)

	// jslint_happy implies a space after anonymous functions.
	if o.JslintHappy {
		o.SpaceAfterAnonFunction = true
	}
	return o, nil
}
