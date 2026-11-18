package minimatch

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type recordedCase struct {
	Pattern string `json:"pattern"`
	Opts    struct {
		MatchBase         bool `json:"matchBase"`
		Dot               bool `json:"dot"`
		NoNegate          bool `json:"nonegate"`
		NoComment         bool `json:"nocomment"`
		OptimizationLevel int  `json:"optimizationLevel"`
		BraceExpandMax    int  `json:"braceExpandMax"`
	} `json:"opts"`
	Error   *string              `json:"error"`
	Magic   bool                 `json:"magic"`
	Braces  []string             `json:"braces"`
	Matches [][2]json.RawMessage `json:"matches"`
}

// TestMatchesNode replays patterns and paths recorded from minimatch 10.2.5 (testdata/record.js).
func TestMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/cases.jsonl.gz")
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
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	cases, bad := 0, 0
	report := func(format string, args ...any) {
		bad++
		if bad <= 25 {
			t.Errorf(format, args...)
		}
	}
	for sc.Scan() {
		var c recordedCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases++
		opts := Options{
			Dot: c.Opts.Dot, MatchBase: c.Opts.MatchBase, NoNegate: c.Opts.NoNegate, NoComment: c.Opts.NoComment,
			OptimizationLevel: c.Opts.OptimizationLevel, BraceExpandMax: c.Opts.BraceExpandMax,
		}
		if c.Braces != nil {
			if got := braceExpand(c.Pattern, expansionMax); !reflect.DeepEqual(got, c.Braces) && !(len(got) == 0 && len(c.Braces) == 0) {
				report("expand(%q) = %q, want %q", c.Pattern, got, c.Braces)
			}
		}
		m, err := New(c.Pattern, opts)
		if c.Error != nil {
			if err == nil || err.Error() != *c.Error {
				report("New(%q, %+v) error = %v, want %q", c.Pattern, opts, err, *c.Error)
			}
			continue
		}
		if err != nil {
			report("New(%q, %+v): %v", c.Pattern, opts, err)
			continue
		}
		if m.HasMagic() != c.Magic {
			report("New(%q, %+v).HasMagic() = %v, want %v", c.Pattern, opts, m.HasMagic(), c.Magic)
		}
		for _, pm := range c.Matches {
			var path string
			var want bool
			if err := json.Unmarshal(pm[0], &path); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(pm[1], &want); err != nil {
				t.Fatal(err)
			}
			if got := m.Match(path); got != want {
				report("New(%q, %+v).Match(%q) = %v, want %v", c.Pattern, opts, path, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if bad > 0 {
		t.Errorf("%d mismatches over %d recorded patterns", bad, cases)
	}
}

func TestEscape(t *testing.T) {
	tests := map[string]string{
		"/tmp/run":   "/tmp/run",
		"/a(b)/[c]*": "/a[(]b[)]/[[]c[]][*]",
		"what?":      "what[?]",
	}
	for in, want := range tests {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}
