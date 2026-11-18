package html

import (
	"github.com/beautifier/js-beautify-go/internal/core"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Options are the HTML beautifier's settings (js/src/html/options.js).
type Options struct {
	*core.Options

	IndentInnerHTML     bool
	IndentBodyInnerHTML bool
	IndentHeadInnerHTML bool
	IndentHandlebars    bool

	WrapAttributes           jsrt.String
	WrapAttributesMinAttrs   int
	WrapAttributesIndentSize int
	ExtraLiners              []jsrt.String

	// Inline lists the inline elements; InlineCustomElements makes every tag name with a "-"
	// inline as well.
	Inline               []jsrt.String
	InlineCustomElements bool
	VoidElements         []jsrt.String
	// Unformatted elements are printed exactly as written, tags included; ContentUnformatted
	// elements get their tags formatted but not their content.
	Unformatted                 []jsrt.String
	ContentUnformatted          []jsrt.String
	UnformattedContentDelimiter jsrt.String
	IndentScripts               jsrt.String
}

var (
	wrapAttributesModes = []jsrt.String{"auto", "force", "force-aligned", "force-expand-multiline",
		"aligned-multiple", "preserve", "preserve-aligned"}
	indentScriptsModes = []jsrt.String{"normal", "keep", "separate"}

	// autoTemplating is what templating "auto" means for HTML: every language but smarty and
	// angular.
	autoTemplating = []jsrt.String{"django", "erb", "handlebars", "php"}

	// Block vs inline elements:
	// https://developer.mozilla.org/en-US/docs/Web/HTML/Block-level_elements
	// https://developer.mozilla.org/en-US/docs/Web/HTML/Inline_elements
	// https://www.w3.org/TR/html5/dom.html#phrasing-content
	defaultInline = []jsrt.String{
		"a", "abbr", "area", "audio", "b", "bdi", "bdo", "br", "button", "canvas", "cite",
		"code", "data", "datalist", "del", "dfn", "em", "embed", "i", "iframe", "img",
		"input", "ins", "kbd", "keygen", "label", "map", "mark", "math", "meter", "noscript",
		"object", "output", "progress", "q", "ruby", "s", "samp", "select", "small",
		"span", "strong", "sub", "sup", "svg", "template", "textarea", "time", "u", "var",
		"video", "wbr", "text",
		// obsolete inline tags
		"acronym", "big", "strike", "tt",
	}

	defaultVoidElements = []jsrt.String{
		// HTML void elements, also known as self-closing tags or singletons:
		// https://www.w3.org/html/wg/drafts/html/master/syntax.html#void-elements
		"area", "base", "br", "col", "embed", "hr", "img", "input", "keygen",
		"link", "menuitem", "meta", "param", "source", "track", "wbr",
		// Optional end tags are handled in doOptionalEndElement.

		// Doctype and xml elements
		"!doctype", "?xml",

		// obsolete tags: basefont, isindex
		"basefont", "isindex",
	}
)

// NewOptions reads the HTML options, with the "html" section merged over the top level. The
// error is the exception the original throws for an invalid selection value.
func NewOptions(options map[string]any) (*Options, error) {
	base, err := core.NewOptions(options, "html")
	if err != nil {
		return nil, err
	}
	o := &Options{Options: base}
	if len(o.Templating) == 1 && o.Templating[0] == "auto" {
		o.Templating = autoTemplating
	}

	o.IndentInnerHTML = o.GetBoolean("indent_inner_html", false)
	o.IndentBodyInnerHTML = o.GetBoolean("indent_body_inner_html", true)
	o.IndentHeadInnerHTML = o.GetBoolean("indent_head_inner_html", true)

	o.IndentHandlebars = o.GetBoolean("indent_handlebars", true)
	if o.WrapAttributes, err = o.GetSelection("wrap_attributes", wrapAttributesModes, nil); err != nil {
		return nil, err
	}
	o.WrapAttributesMinAttrs = o.GetNumber("wrap_attributes_min_attrs", 2)
	o.WrapAttributesIndentSize = o.GetNumber("wrap_attributes_indent_size", o.IndentSize)
	o.ExtraLiners = o.GetArray("extra_liners", []jsrt.String{"head", "body", "/html"})

	o.Inline = o.GetArray("inline", defaultInline)
	o.InlineCustomElements = o.GetBoolean("inline_custom_elements", true)
	o.VoidElements = o.GetArray("void_elements", defaultVoidElements)
	o.Unformatted = o.GetArray("unformatted", nil)
	o.ContentUnformatted = o.GetArray("content_unformatted", []jsrt.String{"pre", "textarea"})
	o.UnformattedContentDelimiter = o.GetCharacters("unformatted_content_delimiter", "")
	if o.IndentScripts, err = o.GetSelection("indent_scripts", indentScriptsModes, nil); err != nil {
		return nil, err
	}
	return o, nil
}
