package jsvalue

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func TestParseJSONMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/json.jsonl.gz")
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
	n, bad := 0, 0
	for sc.Scan() {
		var rec struct {
			Src string  `json:"src"`
			OK  *string `json:"ok"`
			Err *string `json:"err"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rec.Src, `\ud8`) {
			// A lone surrogate escape cannot be held in a Go string.
			continue
		}
		n++
		v, err := ParseJSON(rec.Src)
		var got string
		if err != nil {
			got = "err: " + err.Error()
		} else {
			got = "ok: " + Stringify(v)
		}
		want := ""
		if rec.OK != nil {
			want = "ok: " + *rec.OK
		} else {
			want = "err: " + *rec.Err
		}
		if got != want {
			bad++
			if bad <= 20 {
				t.Errorf("JSON.parse(%q)\n got %s\nwant %s", rec.Src, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if bad > 0 {
		t.Errorf("%d of %d inputs differ", bad, n)
	}
	if n < 1000 {
		t.Errorf("only %d recorded inputs", n)
	}
}

func TestObjectKeyOrder(t *testing.T) {
	o := NewObject()
	for _, k := range []string{"b", "10", "a", "2", "01", "4294967295", "4294967294"} {
		o.Set(k, true)
	}
	o.Set("b", false)
	got := strings.Join(o.Keys(), ",")
	want := "2,10,4294967294,b,a,01,4294967295"
	if got != want {
		t.Errorf("Keys() = %s, want %s", got, want)
	}
	o.Delete("a")
	if o.Has("a") || o.Len() != 6 {
		t.Errorf("Delete left %v", o.Keys())
	}
}

func TestConversions(t *testing.T) {
	tests := []struct {
		v      any
		str    string
		num    float64
		truthy bool
	}{
		{Undefined, "undefined", math.NaN(), false},
		{nil, "null", 0, false},
		{true, "true", 1, true},
		{"", "", 0, false},
		{" 0x1F ", " 0x1F ", 31, true},
		{"1e3", "1e3", 1000, true},
		{"abc", "abc", math.NaN(), true},
		{[]any{}, "", 0, true},
		{[]any{"7"}, "7", 7, true},
		{[]any{1.0, nil, "a"}, "1,,a", math.NaN(), true},
		{NewObject(), "[object Object]", math.NaN(), true},
		{1e21, "1e+21", 1e21, true},
		{math.NaN(), "NaN", math.NaN(), false},
	}
	for _, tt := range tests {
		if got := ToString(tt.v); got != tt.str {
			t.Errorf("ToString(%v) = %q, want %q", tt.v, got, tt.str)
		}
		if got := ToNumber(tt.v); got != tt.num && !(math.IsNaN(got) && math.IsNaN(tt.num)) {
			t.Errorf("ToNumber(%v) = %v, want %v", tt.v, got, tt.num)
		}
		if got := Truthy(tt.v); got != tt.truthy {
			t.Errorf("Truthy(%v) = %v, want %v", tt.v, got, tt.truthy)
		}
	}
}
