package beautify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The expected results below were produced by js-beautify 2.0.3 on Node.js 24.
func TestBeautifiers(t *testing.T) {
	cases := []struct {
		name    string
		fn      func(string, map[string]any) (string, error)
		source  string
		options map[string]any
		want    string
		wantErr string
	}{
		{
			name:    "js end-expand",
			fn:      JS,
			source:  "function f(a,b){if(a){return b}else{return [1,2,3].map(x=>x*2)}}",
			options: map[string]any{"indent_size": 2.0, "brace_style": "end-expand"},
			want:    "function f(a, b) {\n  if (a) {\n    return b\n  }\n  else {\n    return [1, 2, 3].map(x => x * 2)\n  }\n}",
		},
		{
			name:    "css rules",
			fn:      CSS,
			source:  "a,b>c{color:red;margin:0 auto}@media screen{.x{top:0}}",
			options: map[string]any{"newline_between_rules": true, "selector_separator_newline": true},
			want:    "a,\nb>c {\n    color: red;\n    margin: 0 auto\n}\n\n@media screen {\n    .x {\n        top: 0\n    }\n}",
		},
		{
			name:   "html with embedded script and style",
			fn:     HTML,
			source: "<div class=a id=b><p>Hi <b>there</b></p><script>var a={b:1}</script><style>a{b:c}</style></div>",
			options: map[string]any{"indent_inner_html": true, "wrap_attributes": "force",
				"js": map[string]any{"indent_size": 2.0}},
			want: "<div class=a\n    id=b>\n    <p>Hi <b>there</b></p>\n    <script>\n        var a = {\n          b: 1\n        }\n" +
				"    </script>\n    <style>\n        a {\n            b: c\n        }\n    </style>\n</div>",
		},
		{
			name:    "invalid selection",
			fn:      JS,
			source:  "a",
			options: map[string]any{"brace_style": "sideways"},
			wantErr: "Invalid Option Value: The option 'brace_style' can contain only the following values:\n" +
				"collapse,expand,end-expand,none,preserve-inline\nYou passed in: 'sideways'",
		},
		{
			name:    "coerced option values",
			fn:      JS,
			source:  "if(a){b()}",
			options: map[string]any{"indent-size": "3", "end_with_newline": "false", "eol": `\r\n`},
			want:    "if (a) {\r\n   b()\r\n}\r\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.fn(c.source, c.options)
			if c.wantErr != "" {
				if err == nil || err.Error() != c.wantErr {
					t.Fatalf("error = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", filepath.Join(dir, "home"))
	source := "var a={b:1}"
	if err := os.WriteFile("a.js", []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := CLI("js-beautify", []string{"-s", "2", "-r", "a.js"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if stdout.String() != "beautified a.js\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
	data, err := os.ReadFile("a.js")
	if err != nil {
		t.Fatal(err)
	}
	want, err := JS(source, map[string]any{"indent_size": 2.0})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("file %q, want %q", data, want)
	}

	stdout.Reset()
	if code := CLI("css-beautify", []string{"--version"}, strings.NewReader(""), &stdout, &stderr); code != 0 || stdout.String() != Version+"\n" {
		t.Fatalf("--version: exit %d, stdout %q", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := CLI("js-beautify", []string{"missing.js"}, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("missing file: exit %d", code)
	}
	if want := "Unable to open path \"missing.js\"\nRun `js-beautify -h` for help.\n"; stderr.String() != want {
		t.Fatalf("missing file: stderr %q, want %q", stderr.String(), want)
	}
}
