package css

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

// TestRecordedFixtures compares Beautify with css_beautify results recorded from js-beautify
// 2.0.3 on Node 24 (testdata/record.js): upstream's own test cases and generated requests with
// odd options and inputs.
func TestRecordedFixtures(t *testing.T) {
	f, err := os.Open("testdata/css.jsonl.gz")
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
		out, err := Beautify(row.Source, row.Options)
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
	if n < 1000 {
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
		{"defaults", "a{color:red;b:c}", nil, "a {\n    color: red;\n    b: c\n}", ""},
		{"expand", "a{b:c}", map[string]any{"brace_style": "expand"}, "a\n{\n    b: c\n}", ""},
		{"escape at end of input appends null", "a{content:\"x\\", nil, "a {\n    content: \"x\\null", ""},
		{"inherited property names are nesting at-rules", "@constructor{a{b:c}}", nil,
			"@constructor {\n    a {\n        b: c\n    }\n}", ""},
		{"negative indent with tabs indents with undefined", "a{b:c}", map[string]any{"indent_size": -2.0, "indent_with_tabs": true},
			"a {\nundefinedb: c\n}", ""},
		{"negative indent_size", "a{}", map[string]any{"indent_size": -3.0}, "", "Invalid array length"},
		{"invalid brace_style", "a{}", map[string]any{"brace_style": []any{}}, "",
			"Invalid Option Value: The option 'brace_style' can contain only the following values:\n" +
				"collapse,expand,end-expand,none,preserve-inline\nYou passed in: ''"},
		{"eol auto takes the first line break", "a{b:c}\r\nd{}", nil, "a {\r\n    b: c\r\n}\r\n\r\nd {}", ""},
		{"astral characters survive", "a{content:\"\U0001F600\"}", map[string]any{"wrap_line_length": 5.0},
			"a {\n    content: \"\U0001F600\"\n}", ""},
		{"disabled returns the source", "a{b:c}", map[string]any{"disabled": "yes"}, "a{b:c}", ""},
	}
	for _, c := range cases {
		got, err := Beautify(c.source, c.options)
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
