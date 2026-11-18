package html

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/css"
	"github.com/beautifier/js-beautify-go/internal/javascript"
)

// TestRecordedFixtures compares Beautify with html_beautify results recorded from js-beautify
// 2.0.3 on Node 24 (testdata/record.js): upstream's own test cases, generated documents with odd
// options and inputs, and the quirks below.
func TestRecordedFixtures(t *testing.T) {
	f, err := os.Open("testdata/html.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	n := 0
	for sc.Scan() {
		var row struct {
			ID      string
			Source  string
			Options map[string]any
			Out     *string
			Err     *string
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		n++
		out, err := Beautify(row.Source, row.Options, javascript.Beautify, css.Beautify)
		switch {
		case row.Err != nil:
			if err == nil || err.Error() != *row.Err {
				t.Errorf("%s: got (%q, %v), want error %q", row.ID, out, err, *row.Err)
			}
		case err != nil:
			t.Errorf("%s: unexpected error %v", row.ID, err)
		case out != *row.Out:
			t.Errorf("%s: source %q options %v\n got: %q\nwant: %q", row.ID, row.Source, row.Options, out, *row.Out)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n < 10000 {
		t.Fatalf("only %d recorded cases", n)
	}
}

func TestQuirks(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		options map[string]any
		want    string
		err     string
	}{
		{"unknown first token leaves last_token null", "{<div>x</div>", map[string]any{"templating": []any{"smarty", "django"}},
			"", "Cannot read properties of null (reading 'type')"},
		{"an inline tag after it does not read last_token", "{<span>x</span>",
			map[string]any{"templating": []any{"smarty", "django"}}, "{<span>x</span>", ""},
		{"tag names go into the closing-tag regex unescaped", "<p(re>x</p(re>",
			map[string]any{"content_unformatted": []any{"p(re"}}, "",
			"Invalid regular expression: /</p(re[\\n\\r\\t ]*?>/gi: Unterminated group"},
		{"test/null scripts are only indented", "<div><script type=\"test/null\">\n  a  =  1;\n    b\n</script></div>", nil,
			"<div>\n    <script type=\"test/null\">\n        a  =  1;\n          b\n    </script>\n</div>", ""},
		{"dashed keys of the html section reach only the child beautifiers", "<div><script>if(a){b()}</script></div>",
			map[string]any{"html": map[string]any{"indent-size": 2.0}},
			"<div>\n    <script>\n        if (a) {\n          b()\n        }\n    </script>\n</div>", ""},
		{"child beautifier errors propagate", "<script>a</script>", map[string]any{"js": map[string]any{"brace_style": "bogus"}}, "",
			"Invalid Option Value: The option 'brace_style' can contain only the following values:\n" +
				"collapse,expand,end-expand,none,preserve-inline\nYou passed in: 'bogus'"},
		{"angular-only templating splits {{ in attributes", "<div {{a}}>x</div>", map[string]any{"templating": []any{"angular"}},
			"<div { {a}}>x</div>", ""},
		{"comment and CDATA wrappers", "<div><script><!--\nvar a=1;\n--></script><style><![CDATA[\na{b:c}\n]]></style></div>", nil,
			"<div>\n    <script>\n        <!--\n        var a = 1;\n        -->\n    </script>\n    <style>\n        <![CDATA[\n" +
				"        a {\n            b: c\n        }\n        ]]>\n    </style>\n</div>", ""},
		{"indent_char $ is literal in the replacement", "<div><script>\n  a=1\n</script></div>",
			map[string]any{"indent_char": "$", "indent_size": 2.0}, "<div>\n$$<script>\n$$$$a = 1\n$$</script>\n</div>", ""},
		{"optional end tags", "<ul><li>a<li>b</ul><table><tr><td>1<td>2<tr><td>3</table>", nil,
			"<ul>\n    <li>a\n    <li>b\n</ul>\n<table>\n    <tr>\n        <td>1\n        <td>2\n    <tr>\n        <td>3\n</table>", ""},
		{"force-expand-multiline", "<div a=1 b=2 c=3>x</div>",
			map[string]any{"wrap_attributes": "force-expand-multiline"}, "<div\n    a=1\n    b=2\n    c=3\n>x</div>", ""},
		{"angular control flow", "@if (a) {<p>x</p>} @else {<p>y</p>}", map[string]any{"templating": "angular"},
			"@if (a) {\n    <p>x</p>} @else {\n    <p>y</p>}", ""},
		{"invalid wrap_attributes", "<a>", map[string]any{"wrap_attributes": "bogus"}, "",
			"Invalid Option Value: The option 'wrap_attributes' can contain only the following values:\n" +
				"auto,force,force-aligned,force-expand-multiline,aligned-multiple,preserve,preserve-aligned\nYou passed in: 'bogus'"},
		{"disabled returns the source", "<div><p>x</div>", map[string]any{"disabled": true}, "<div><p>x</div>", ""},
		{"eol auto takes the first line break", "<div>\r\n<p>x</p></div>", nil, "<div>\r\n    <p>x</p>\r\n</div>", ""},
	}
	for _, c := range cases {
		got, err := Beautify(c.source, c.options, javascript.Beautify, css.Beautify)
		if c.err != "" {
			if err == nil || err.Error() != c.err {
				t.Errorf("%s: got (%q, %v), want error %q", c.name, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got (%q, %v), want %q", c.name, got, err, c.want)
		}
	}
}

// TestWithoutChildBeautifiers checks that without a JavaScript or CSS beautifier the content of
// script and style elements is only re-indented, as for a type with no beautifier.
func TestWithoutChildBeautifiers(t *testing.T) {
	source := "<div><script>\n  a  =  1;\n    b\n</script><style>a{b:c}</style></div>"
	want := "<div>\n    <script>\n        a  =  1;\n          b\n    </script>\n    <style>\n        a{b:c}\n    </style>\n</div>"
	got, err := Beautify(source, nil, nil, nil)
	if err != nil || got != want {
		t.Errorf("got (%q, %v), want %q", got, err, want)
	}
}
