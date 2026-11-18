package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// Directives recognises "beautify" directives inside comments, such as
// /* beautify ignore:start */ or <!-- beautify preserve:end --> (js/src/core/directives.js).
type Directives struct {
	blockPattern     *jsrt.Regexp
	endIgnorePattern *jsrt.Regexp
}

var directivePattern = jsrt.MustCompile(` (\w+)[:](\w+)`, "g")

// NewDirectives returns the directive reader for comments delimited by the regex sources
// startBlockPattern and endBlockPattern (e.g. `\/\*` and `\*\/`).
func NewDirectives(startBlockPattern, endBlockPattern jsrt.String) *Directives {
	return &Directives{
		blockPattern:     jsrt.MustCompile(startBlockPattern+` beautify( \w+[:]\w+)+ `+endBlockPattern, "g"),
		endIgnorePattern: jsrt.MustCompile(startBlockPattern+`\sbeautify\signore:end\s`+endBlockPattern, "g"),
	}
}

// GetDirectives returns the name:value pairs of a directive comment, or nil when text holds no
// directive block.
func (d *Directives) GetDirectives(text jsrt.String) map[jsrt.String]jsrt.String {
	if d.blockPattern.Search(text) < 0 {
		return nil
	}
	directives := map[jsrt.String]jsrt.String{}
	units := text.Units()
	for m := directivePattern.ExecUnits(units, 0); m != nil; m = directivePattern.ExecUnits(units, m.End()) {
		directives[m.GroupText(1)] = m.GroupText(2)
	}
	return directives
}

// ReadIgnored reads the input up to and including the comment that ends an ignored section.
func (d *Directives) ReadIgnored(input *InputScanner) jsrt.String {
	return input.ReadUntilAfter(d.endIgnorePattern)
}
