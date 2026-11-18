package nopt

import (
	"reflect"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
)

func TestAbbrev(t *testing.T) {
	got := abbrev([]string{"foo", "fool", "bar"})
	want := map[string]string{"b": "bar", "ba": "bar", "bar": "bar", "foo": "foo", "fool": "fool"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("abbrev = %v, want %v", got, want)
	}
}

// The expectations below were produced by nopt 10.0.1 with the same tables.
func TestParse(t *testing.T) {
	p := &Parser{
		Types: map[string]TypeSpec{
			"size": One(Number), "name": One(String), "flag": One(Boolean), "list": List(String, Array),
			"mode": List(Literal("a"), Literal("b")), "out": One(Path),
		},
		Shorthands: map[string][]string{"s": {"--size"}, "f": {"--flag"}, "x": {"--mode", "b"}},
		TypeDefs:   StandardTypeDefs("/cwd", "/home"),
	}
	tests := []struct {
		args []string
		want string
		err  string
	}{
		{[]string{"-s", "3", "file"}, `{"size":3,"argv":{"remain":["file"],"cooked":["--size","3","file"],"original":["-s","3","file"]}}`, ""},
		{[]string{"-sf", "--name"}, "", `size was invalid with value "--flag"`},
		{[]string{"--name", "-fs"}, `{"name":"","flag":true,"size":1,"argv":{"remain":[],"cooked":["--name","--flag","--size"],"original":["--name","-fs"]}}`, ""},
		{[]string{"--no-flag", "--li", "a", "--list=b"}, `{"flag":false,"list":["a","b"],"argv":{"remain":[],"cooked":["--no-flag","--li","a","--list","b"],"original":["--no-flag","--li","a","--list=b"]}}`, ""},
		{[]string{"-x", "--out", "~/y", "--", "-s"}, `{"mode":"b","out":"/home/y","argv":{"remain":["-s"],"cooked":["--mode","b","--out","~/y","--","-s"],"original":["-x","--out","~/y","--","-s"]}}`, ""},
		{[]string{"--mode", "c"}, "", `mode was invalid with value "c"`},
		{[]string{"--size", "abc"}, "", `size was invalid with value "abc"`},
		{[]string{"--constructor"}, "", "shorthands[arg].split is not a function"},
		{[]string{"--unknown", "v", "--other=null"}, `{"unknown":true,"other":null,"argv":{"remain":["v"],"cooked":["--unknown","v","--other","null"],"original":["--unknown","v","--other=null"]}}`, ""},
	}
	for _, tt := range tests {
		got, err := p.Parse(tt.args)
		switch {
		case tt.err != "":
			if err == nil || err.Error() != tt.err {
				t.Errorf("Parse(%q) error = %v, want %q", tt.args, err, tt.err)
			}
		case err != nil:
			t.Errorf("Parse(%q): %v", tt.args, err)
		case jsvalue.Stringify(got) != tt.want:
			t.Errorf("Parse(%q) = %v\nwant %s", tt.args, jsvalue.Stringify(got), tt.want)
		}
	}
}
