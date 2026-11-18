package css

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Options are the CSS beautifier's settings (js/src/css/options.js).
type Options struct {
	*core.Options

	SelectorSeparatorNewline bool
	NewlineBetweenRules      bool
	SpaceAroundCombinator    bool
	// BraceStyle is "collapse" or "expand": the only styles the CSS beautifier implements.
	BraceStyle jsrt.String
}

// NewOptions reads the CSS options, with the "css" section merged over the top level.
func NewOptions(options map[string]any) (*Options, error) {
	base, err := core.NewOptions(options, "css")
	if err != nil {
		return nil, err
	}
	o := &Options{Options: base}

	o.SelectorSeparatorNewline = o.GetBoolean("selector_separator_newline", true)
	o.NewlineBetweenRules = o.GetBoolean("newline_between_rules", true)
	spaceAroundSelectorSeparator := o.GetBoolean("space_around_selector_separator", false)
	o.SpaceAroundCombinator = o.GetBoolean("space_around_combinator", false) || spaceAroundSelectorSeparator

	braceStyleSplit, err := o.GetSelectionList("brace_style",
		[]jsrt.String{"collapse", "expand", "end-expand", "none", "preserve-inline"}, nil)
	if err != nil {
		return nil, err
	}
	// The last style listed wins; every style but expand means collapse.
	o.BraceStyle = "collapse"
	for _, bs := range braceStyleSplit {
		if bs != "expand" {
			o.BraceStyle = "collapse"
		} else {
			o.BraceStyle = bs
		}
	}
	return o, nil
}
