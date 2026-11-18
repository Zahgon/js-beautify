package nodepath

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// TestMatchesNode compares with Node's path functions on the inputs of testdata/record.js.
func TestMatchesNode(t *testing.T) {
	data, err := os.ReadFile("testdata/path.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases [][3]string
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	const cwd = "/tmp/jsb/run"
	for _, c := range cases {
		fn, in, want := c[0], c[1], c[2]
		if fn == "basename" {
			continue
		}
		var got string
		switch fn {
		case "dirname":
			got = Dirname(in)
		case "normalize":
			got = Normalize(in)
		case "resolve":
			got = Resolve(cwd, in)
		case "join":
			got = Join(in, "x")
		case "relative":
			got = Relative(cwd, cwd, Resolve(cwd, in))
		case "parse":
			p := Parse(in)
			got = fmt.Sprintf(`{"root":%q,"dir":%q,"base":%q,"ext":%q,"name":%q}`, p.Root, p.Dir, p.Base, p.Ext, p.Name)
		}
		if got != want {
			t.Errorf("path.%s(%q) = %q, want %q", fn, in, got, want)
		}
	}
	if len(cases) < 100 {
		t.Errorf("only %d cases", len(cases))
	}
}
