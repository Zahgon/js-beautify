package editorconfig

import (
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
)

func eachLine(t *testing.T, name string, f func([]byte)) {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f(sc.Bytes())
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestParseINIMatchesOneIni replays files tokenized by @one-ini/wasm (testdata/record_ini.js).
func TestParseINIMatchesOneIni(t *testing.T) {
	n, bad := 0, 0
	eachLine(t, "testdata/ini.jsonl.gz", func(line []byte) {
		var rec struct {
			B64    string   `json:"b64"`
			Tokens []string `json:"tokens"`
			Error  bool     `json:"error"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		data, err := base64.StdEncoding.DecodeString(rec.B64)
		if err != nil {
			t.Fatal(err)
		}
		n++
		sections, err := parseINI(data)
		if rec.Error {
			if err == nil {
				bad++
				t.Errorf("parseINI(%q) succeeded, one-ini fails", data)
			}
			return
		}
		got := []string{}
		for i, s := range sections {
			if i > 0 {
				got = append(got, "S:"+s.name)
			}
			for _, kv := range s.pairs {
				got = append(got, "K:"+kv[0], "V:"+kv[1])
			}
		}
		if err != nil || !reflect.DeepEqual(got, rec.Tokens) {
			bad++
			if bad <= 20 {
				t.Errorf("parseINI(%q) = %q, %v; want %q", data, got, err, rec.Tokens)
			}
		}
	})
	if bad > 0 || n < 1000 {
		t.Errorf("%d of %d files differ", bad, n)
	}
}

// TestParseSyncMatchesNode rebuilds the .editorconfig trees recorded by testdata/record_parse.js
// (each rooted with root = true) and compares parseSync's results.
func TestParseSyncMatchesNode(t *testing.T) {
	base := t.TempDir()
	n, bad := 0, 0
	c := 0
	eachLine(t, "testdata/parse.jsonl.gz", func(line []byte) {
		var rec struct {
			Files   map[string]string `json:"files"`
			Queries [][2]*string      `json:"queries"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(base, strconv.Itoa(c))
		c++
		for rel, content := range rec.Files {
			p := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, q := range rec.Queries {
			n++
			props, err := ParseSync(filepath.Join(dir, *q[0]))
			got := "null"
			if err == nil {
				got = jsvalue.Stringify(props)
			}
			want := "null"
			if q[1] != nil {
				want = *q[1]
			}
			if got != want {
				bad++
				if bad <= 20 {
					t.Errorf("files %q: ParseSync(%s) = %s, want %s", rec.Files, *q[0], got, want)
				}
			}
		}
	})
	if bad > 0 || n < 1000 {
		t.Errorf("%d of %d queries differ", bad, n)
	}
}
