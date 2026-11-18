package javascript

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

// TestRecordedFixtures compares Beautify with js_beautify results recorded from js-beautify
// 2.0.3 on Node 24 (testdata/record.js): upstream's own test cases and generated requests with
// odd options and inputs (tools/agent-js/gen_js.py).
func TestRecordedFixtures(t *testing.T) {
	f, err := os.Open("testdata/js.jsonl.gz")
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
	n, failed := 0, 0
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
		var ok bool
		switch {
		case row.Err != nil:
			ok = err != nil && err.Error() == *row.Err
		case err != nil:
			ok = false
		default:
			ok = out == *row.Out
		}
		if !ok {
			if failed++; failed <= 20 {
				t.Errorf("%s: source %q options %v\n got: (%q, %v)\nwant: out %q err %q",
					row.ID, row.Source, row.Options, out, err, deref(row.Out), deref(row.Err))
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Errorf("%d of %d recorded cases differ", failed, n)
	}
	if n < 30000 {
		t.Fatalf("only %d recorded cases", n)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}

// TestQuirks pins behaviours of js-beautify that look surprising but are part of its output,
// with the results the original gives.
func TestQuirks(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		options map[string]any
		want    string
		err     string
	}{
		{"code point escapes keep the low 16 bits", "a = '\\u{1F600}\\x41'", map[string]any{"unescape_strings": true}, "a = '\uf600A'", ""},
		{"a high \\x escape leaves the string escaped", "a = '\\x41\\x7f'", map[string]any{"unescape_strings": true}, "a = '\\x41\\x7f'", ""},
		{"quotes and control characters stay escaped", "a = '\\x27\\x22\\x5c\\x0a\\u0041'", map[string]any{"unescape_strings": true}, "a = '\\'\\\"\\\\\\x0aA'", ""},
		{"eol auto takes the first line break", "a;\u2028b;\r\nc", nil, "a;\u2028b;\u2028c", ""},
		{"expand-strict is expand", "if (a) { b }", map[string]any{"brace_style": "expand-strict"}, "if (a)\n{\n    b\n}", ""},
		{"braces_on_own_line overrides brace_style", "if (a) { b } else { c }", map[string]any{"brace_style": "end-expand", "braces_on_own_line": false}, "if (a) {\n    b\n} else {\n    c\n}", ""},
		{"collapse-preserve-inline", "if (a) { b }\nx = {c: 1}", map[string]any{"brace_style": "collapse-preserve-inline"}, "if (a) { b }\nx = { c: 1 }", ""},
		{"astral identifiers are split into two unknown tokens", "var \U0001d465=3;f(\U0001f600)", nil, "var\U0001d465 = 3;\nf(\U0001f600)", ""},
		{"shebang", "#!/usr/bin/env node\nfoo()", nil, "#!/usr/bin/env node\n\nfoo()", ""},
		{"html comment start and end", "<!-- hide\nvar a = 1;\n-->", nil, "<!-- hide\nvar a = 1;\n-->", ""},
		{"sharp variables", "#1={a:#1#};x=#2=[1]", nil, "#1= {\n    a: #1#\n};\nx = #2=[1]", ""},
		{"a dot at the end of the input", "a.", nil, "a.", ""},
		{"templating inside strings", "a = '<?php echo 'x'; ?>'+b", map[string]any{"templating": []any{"php"}}, "a = '<?php echo 'x'; ?>' + b", ""},
		{"jslint_happy implies space_after_anon_function", "x = function(){}", map[string]any{"jslint_happy": true}, "x = function () {}", ""},
		{"e4x with an expression root", "x = <{tag}><b>t</b></{tag}>; y=1", map[string]any{"e4x": true}, "x = <{tag}><b>t</b></{tag}>;\ny = 1", ""},
		{"unclosed e4x keeps the rest", "x = <a><b>\nif(c){d}", map[string]any{"e4x": true}, "x = <a><b>\nif(c){d}", ""},
		{"ignore directive", "/* beautify ignore:start */\nvar   a=1;\n/* beautify ignore:end */\nvar  b=2", nil, "/* beautify ignore:start */\nvar   a=1;\n/* beautify ignore:end */\nvar b = 2", ""},
		{"preserve directive", "/* beautify preserve:start */\nvar   a=[\n 1];\n/* beautify preserve:end */\nb( )", nil, "/* beautify preserve:start */\nvar   a=[\n 1];\n/* beautify preserve:end */\nb()", ""},
		{"invalid operator_position", "a", map[string]any{"operator_position": "bogus"}, "", "Invalid Option Value: The option 'operator_position' can contain only the following values:\nbefore-newline,after-newline,preserve-newline\nYou passed in: 'bogus'"},
		{"invalid brace_style", "a", map[string]any{"brace_style": "collapse,bogus"}, "", "Invalid Option Value: The option 'brace_style' can contain only the following values:\ncollapse,expand,end-expand,none,preserve-inline\nYou passed in: 'collapse,bogus'"},
		{"negative indent_size", "a", map[string]any{"indent_size": -2.0}, "", "Invalid array length"},
		{"disabled returns the source", "a  =  1", map[string]any{"disabled": "false"}, "a  =  1", ""},
		{"comma first", "var a = 1,\n  b = 2;\nf(x,\ny)", map[string]any{"comma_first": true}, "var a = 1\n    , b = 2;\nf(x\n    , y)", ""},
		{"operator after newline", "x = aaaa + bbbb + cccc + dddd", map[string]any{"operator_position": "after-newline", "wrap_line_length": 12.0}, "x = aaaa +\n    bbbb +\n    cccc +\n    dddd", ""},
		{"regex after a closing paren of if", "if (a) /b/.test(c)", nil, "if (a) /b/.test(c)", ""},
		{"division after a closing paren", "x = (a) /b/ c", nil, "x = (a) / b / c", ""},
		{"template literal nesting", "x = `a${`b${c}`}d`", nil, "x = `a${`b${c}`}d`", ""},
		{"keep_array_indentation", "x = [\n      1,\n   2]", map[string]any{"keep_array_indentation": true}, "x = [\n      1,\n   2]", ""},
		{"indent_level replaces the source indentation", "    a;\n    b", map[string]any{"indent_level": 1.0}, "    a;\n    b", ""},
		{"source indentation is kept", "  a;\n b", nil, "  a;\n  b", ""},
		{"lone CR line ends", "a;\rb;\r", nil, "a;\rb;", ""},
		{"indent_empty_lines", "{\na;\n\nb}", map[string]any{"indent_empty_lines": true}, "{\n    a;\n    \n    b\n}", ""},
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

// TestRepeatedCalls checks that no tokenizer state leaks from one call into the next: an HTML
// comment opened by one source does not make "-->" a comment in the next.
func TestRepeatedCalls(t *testing.T) {
	if _, err := Beautify("<!-- open\na", nil); err != nil {
		t.Fatal(err)
	}
	got, err := Beautify("a-->b", nil)
	if want := "a-- > b"; err != nil || got != want {
		t.Errorf("got (%q, %v), want %q", got, err, want)
	}
}

// TestTerminates feeds unbalanced and unterminated constructs; every call must return.
func TestTerminates(t *testing.T) {
	sources := []string{"`${", "`${`${`${", "/[", "'\\", "#", "#1", "#1=", "<!--", "/*", "{{", "<?php", "a = <a ",
		"((((", "))))", "}}}}", "]]]]", "\\u{", "?.", "...", "#{", "\ufeff", "\u2028\u2029"}
	options := []map[string]any{nil, {"e4x": true, "templating": "php,django,erb,handlebars"}, {"templating": "smarty"}}
	for _, s := range sources {
		for _, o := range options {
			if _, err := Beautify(s, o); err != nil {
				t.Errorf("%q: %v", s, err)
			}
		}
	}
}
