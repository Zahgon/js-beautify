package cli

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
	"github.com/beautifier/js-beautify-go/internal/nopt"
)

// TestNoptMatchesNode replays argument lists parsed by nopt with cli.js's own option tables
// (testdata/record_nopt.js), recorded in /tmp/jsb/run with HOME=/tmp/jsb/home.
func TestNoptMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/nopt.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	p := &nopt.Parser{Types: knownOpts, Shorthands: shortHands, TypeDefs: typeDefs("/tmp/jsb/run", "/tmp/jsb/home")}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n, bad := 0, 0
	for sc.Scan() {
		var rec struct {
			Args   []string `json:"args"`
			Parsed *string  `json:"parsed"`
			Error  *string  `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		n++
		parsed, err := p.Parse(rec.Args)
		var got, want string
		if err != nil {
			got = "error: " + errorString(err)
		} else {
			got = jsvalue.Stringify(parsed)
		}
		if rec.Error != nil {
			want = "error: " + *rec.Error
		} else {
			want = *rec.Parsed
		}
		if got != want {
			bad++
			if bad <= 20 {
				t.Errorf("nopt(%q)\n got %s\nwant %s", rec.Args, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if bad > 0 || n < 1000 {
		t.Errorf("%d of %d argument lists differ", bad, n)
	}
}

func TestSplitBraceStyle(t *testing.T) {
	tests := map[string][]string{
		"collapse":                 {"collapse"},
		"collapse,preserve-inline": {"collapse", "preserve-inline"},
		"":                         {""},
		", expand,":                {"", "expand", ""},
		"a é_b":                    {"a", "_b"},
	}
	for in, want := range tests {
		got := splitBraceStyle(in)
		if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
			t.Errorf("splitBraceStyle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOutputType(t *testing.T) {
	tests := []struct {
		outfile  any
		filepath string
		config   any
		want     string
		err      string
	}{
		{nil, "/a/b.css", "js", "css", ""},
		{"x.html", "/a/b.css", "js", "html", ""},
		{"x.txt", "-", "css", "css", ""},
		{true, "-", 5.0, "5", ""},
		{[]any{"a.js"}, "/a/b.css", "js", "", "outfile.split is not a function"},
		{nil, "/a/b.min.js", nil, "js", ""},
		{nil, "/a/b", nil, "", "Could not determine appropriate beautifier from file paths: /a/b"},
	}
	for _, tt := range tests {
		got, err := outputType(tt.outfile, tt.filepath, tt.config)
		switch {
		case tt.err != "":
			if err == nil || err.Error() != tt.err {
				t.Errorf("outputType(%v, %q, %v) error = %v, want %q", tt.outfile, tt.filepath, tt.config, err, tt.err)
			}
		case err != nil || jsvalue.ToString(got) != tt.want:
			t.Errorf("outputType(%v, %q, %v) = %v, %v; want %q", tt.outfile, tt.filepath, tt.config, got, err, tt.want)
		}
	}
}

func TestBeautifierOptionsKeepsLastSpelling(t *testing.T) {
	cfg := jsvalue.NewObject()
	cfg.Set("indent-size", 5.0)
	cfg.Set("indent_size", 4.0)
	cfg.Set("end_with_newline", true)
	cfg.Set("end-with-newline", false)
	cfg.Set("valueOf", jsvalue.Undefined)
	got := beautifierOptions(cfg)
	if got["indent_size"] != 4.0 || got["end_with_newline"] != false || len(got) != 2 {
		t.Errorf("beautifierOptions = %v", got)
	}
}

// run runs a command in dir with a CSS "beautifier" that upper-cases its input and appends the
// indent_size it got when that is not the default 4.
func run(t *testing.T, dir, name string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	css := func(src string, options map[string]any) (string, error) {
		if size := jsvalue.ToString(options["indent_size"]); size != "4" {
			return strings.ToUpper(src) + "/" + size, nil
		}
		return strings.ToUpper(src), nil
	}
	var out, errOut bytes.Buffer
	rc := Run(name, args, strings.NewReader(stdin), &out, &errOut, Beautifiers{CSS: css})
	return rc, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.css", "a{}")
	write("sub/b.css", "b{}")
	write("same.css", "SAME{}")

	tests := []struct {
		name, stdin string
		args        []string
		rc          int
		out, err    string
	}{
		{"version", "", []string{"--version"}, 0, "2.0.3\n", ""},
		{"stdin", "x{}", nil, 0, "X{}", ""},
		{"stdin with option", "x{}", []string{"-s", "2"}, 0, "X{}/2", ""},
		{"file", "", []string{"a.css"}, 0, "A{}", ""},
		{"missing", "", []string{"nope.css"}, 1, "", "Unable to open path \"nope.css\"\nRun `css-beautify -h` for help.\n"},
		{"invalid value", "", []string{"-s", "x"}, 1, "", "Error: indent_size was invalid with value \"x\"\n\n"},
		{"glob", "", []string{"-q", "**/*.css"}, 0, "", ""},
		{"unchanged", "", []string{"-r", "same.css"}, 0, "beautified same.css - unchanged\n", ""},
		{"outfile in new directory", "", []string{"a.css", "-o", "out/new/a.css"}, 0, "beautified out/new/a.css\n", ""},
		{"directory", "", []string{"sub"}, 1, "", "Error: EISDIR: illegal operation on a directory, read\n    at fs.read {\n  errno: -21,\n  code: 'EISDIR',\n  syscall: 'read'\n}\nRun `css-beautify -h` for help.\n"},
	}
	for _, tt := range tests {
		rc, out, errOut := run(t, dir, "css-beautify", tt.stdin, tt.args...)
		if tt.name == "invalid value" {
			errOut = errOut[strings.LastIndex(errOut[:len(errOut)-2], "\n")+1:]
		}
		if rc != tt.rc || out != tt.out || errOut != tt.err {
			t.Errorf("%s: css-beautify %q = %d, %q, %q; want %d, %q, %q", tt.name, tt.args, rc, out, errOut, tt.rc, tt.out, tt.err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "sub/b.css"))
	if err != nil || string(data) != "B{}" {
		t.Errorf("glob replaced sub/b.css with %q, %v", data, err)
	}
}

func TestConfigurationChainReport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".jsbeautifyrc"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc, _, errOut := run(t, dir, "css-beautify", "a{}")
	lines := strings.Split(errOut, "\n")
	if rc != 1 || len(lines) < 6 ||
		lines[0] != "SyntaxError: Expected property name or '}' in JSON at position 1 (line 1 column 2)" ||
		lines[2] != "Error while loading beautifier configuration." ||
		lines[4] != filepath.Join(dir, ".jsbeautifyrc") ||
		!strings.HasSuffix(lines[5], "/config/defaults.json") {
		t.Errorf("chain report = %d, %q", rc, errOut)
	}
}
