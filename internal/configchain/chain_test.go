package configchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
)

func object(kv ...any) *jsvalue.Object {
	o := jsvalue.NewObject()
	for i := 0; i < len(kv); i += 2 {
		o.Set(kv[i].(string), kv[i+1])
	}
	return o
}

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name    string
		sources []Source
		want    string
		err     string
	}{
		{"precedence", []Source{object("a", 1.0), object("a", 2.0, "b", 3.0)}, `{"a":1,"b":3}`, ""},
		{"file", []Source{object("a", 1.0), write("rc", `{"b": [1], "a": 5}`)}, `{"a":1,"b":[1]}`, ""},
		{"skipped", []Source{nil, "", object("x", true)}, `{"x":true}`, ""},
		{"unreadable", []Source{dir, object("x", true)}, `{"x":true}`, ""},
		{"number", []Source{write("n", "5")}, `{"valueOf":5}`, ""},
		{"array", []Source{write("arr", "[7,8]"), object("x", 1.0)}, `{"0":7,"1":8,"x":1}`, ""},
		{"null", []Source{write("null", "null")}, ``, "Object.setPrototypeOf called on null or undefined"},
		{"broken", []Source{write("bad", "{bad")}, ``, "Expected property name or '}' in JSON at position 1 (line 1 column 2)"},
	}
	for _, tt := range tests {
		c, err := New(dir, tt.sources...)
		if tt.err != "" {
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s: error %v, want %q", tt.name, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got := jsvalue.Stringify(c.Snapshot()); got != tt.want {
			t.Errorf("%s: snapshot %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestProtoKey(t *testing.T) {
	inner := object("indent_size", 2.0)
	c, err := New("/", object("__proto__", inner, "a", 1.0))
	if err != nil {
		t.Fatal(err)
	}
	s := c.Snapshot()
	if v, _ := s.Lookup("indent_size"); v != 2.0 || s.Has("indent_size") || s.Has("__proto__") {
		t.Errorf("snapshot %s, inherited indent_size %v", jsvalue.Stringify(s), v)
	}
	if Copy(s).Has("indent_size") {
		t.Errorf("Copy kept the inherited property")
	}
}

func TestEnv(t *testing.T) {
	env := Env("jsbeautify_", []string{"HOME=/x", "jsbeautify_b=1", "jsbeautify_a=2", "JSBEAUTIFY_c=3", "jsbeautify___proto__=4", "jsbeautify_=5", "jsbeautify_0=6"})
	if got := jsvalue.Stringify(env); got != `{"0":"6","b":"1","a":"2","":"5"}` {
		t.Errorf("Env = %s", got)
	}
}
