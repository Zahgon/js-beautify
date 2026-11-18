package glob

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestMatchesNode rebuilds the trees of testdata/record.js and compares hasMagic and the
// results of globSync with absolute paths and js-beautify's ignore list, in order.
func TestMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/glob.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	base := t.TempDir()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n, bad := 0, 0
	for c := 0; sc.Scan(); c++ {
		var rec struct {
			Tree    []string             `json:"tree"`
			Results [][3]json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(base, strconv.Itoa(c))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range rec.Tree {
			p := filepath.Join(dir, name)
			if strings.HasSuffix(name, "/") {
				err = os.MkdirAll(p, 0o755)
			} else if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
				err = os.WriteFile(p, []byte("x"), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, r := range rec.Results {
			var pattern string
			var magic bool
			var want []string
			if json.Unmarshal(r[0], &pattern) != nil || json.Unmarshal(r[1], &magic) != nil || json.Unmarshal(r[2], &want) != nil {
				t.Fatal("bad record")
			}
			pattern = strings.Replace(pattern, "TREEABS", dir, 1)
			n++
			gotMagic, err := HasMagic(pattern)
			if err != nil || gotMagic != magic {
				bad++
				t.Errorf("HasMagic(%q) = %v, %v; want %v", pattern, gotMagic, err, magic)
				continue
			}
			found, err := Sync(pattern, Options{Cwd: dir, Absolute: true, Ignore: []string{"**/node_modules/**", "**/.git/**"}})
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, p := range found {
				rel, err := filepath.Rel(dir, p)
				if err != nil {
					t.Fatal(err)
				}
				if rel == "." {
					rel = ""
				}
				got = append(got, rel)
			}
			if !reflect.DeepEqual(got, want) {
				bad++
				if bad <= 20 {
					t.Errorf("tree %q: globSync(%q) = %q, want %q", rec.Tree, pattern, got, want)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if bad > 0 || n < 1000 {
		t.Errorf("%d of %d patterns differ", bad, n)
	}
}

func TestInvalidPattern(t *testing.T) {
	for _, v := range []any{5.0, nil, []any{"a", true}} {
		if _, err := HasMagic(v); err == nil || err.Error() != "invalid pattern" {
			t.Errorf("HasMagic(%v) error = %v", v, err)
		}
	}
	if magic, err := HasMagic([]any{"a", "b*"}); err != nil || !magic {
		t.Errorf("HasMagic of an array = %v, %v", magic, err)
	}
}
