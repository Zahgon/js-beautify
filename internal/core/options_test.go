package core

import (
	"reflect"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Ported from js/test/core/test_options.js, plus the coercions of JSON-typed values.

func mustOptions(t *testing.T, raw map[string]any) *Options {
	t.Helper()
	o, err := NewOptions(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestMergeOpts(t *testing.T) {
	got := MergeOpts(map[string]any{"a": 1, "b": map[string]any{"a": 2, "c": 3}}, "b")
	if want := map[string]any{"a": 2, "c": 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("MergeOpts = %v, want %v", got, want)
	}
	got = MergeOpts(map[string]any{"a": 1, "b_c": 2}, "b")
	if want := map[string]any{"a": 1, "b_c": 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("MergeOpts without the section = %v, want %v", got, want)
	}
	got = MergeOpts(map[string]any{"css": map[string]any{"indent-size": 2}}, "css")
	if want := map[string]any{"indent-size": 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("section keys are not normalized: got %v", got)
	}
}

func TestNormalizeOpts(t *testing.T) {
	got := NormalizeOpts(map[string]any{"a-b": 1})
	if want := map[string]any{"a_b": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeOpts = %v, want %v", got, want)
	}
	got = NormalizeOpts(map[string]any{"end-with-newline": true, "end_with_newline": false})
	if got["end_with_newline"] != true {
		t.Errorf("the dashed spelling should win, got %v", got)
	}
}

func TestGetBoolean(t *testing.T) {
	if mustOptions(t, nil).GetBoolean("", false) {
		t.Error("no option and no default should be false")
	}
	if !mustOptions(t, nil).GetBoolean("a", true) {
		t.Error("the default should apply")
	}
	if mustOptions(t, map[string]any{"a": false}).GetBoolean("a", true) {
		t.Error("the option should win")
	}
	if !mustOptions(t, map[string]any{"a": "false"}).GetBoolean("a", false) {
		t.Error(`"false" is truthy`)
	}
}

func TestGetCharacters(t *testing.T) {
	if got := mustOptions(t, nil).GetCharacters("", ""); got != "" {
		t.Errorf("got %q", got)
	}
	if got := mustOptions(t, nil).GetCharacters("a", "character"); got != "character" {
		t.Errorf("got %q", got)
	}
	if got := mustOptions(t, map[string]any{"a": "char"}).GetCharacters("a", "character"); got != "char" {
		t.Errorf("got %q", got)
	}
	if got := mustOptions(t, map[string]any{"a": `\r\n\r\n`}).GetCharacters("a", ""); got != "\r\n"+`\r\n` {
		t.Errorf("only the first escape of each kind is replaced, got %q", got)
	}
	if got := mustOptions(t, map[string]any{"a": 5.0}).GetCharacters("a", "x"); got != "x" {
		t.Errorf("a number gives the default, got %q", got)
	}
}

func TestGetNumber(t *testing.T) {
	cases := []struct {
		value any
		def   int
		want  int
	}{
		{nil, 0, 0},
		{10.0, 1, 10},
		{"abc", 0, 0},
		{2.7, 4, 2},
		{1e21, 4, 1},
		{-3.0, 4, -3},
		{" 12px", 4, 12},
		{[]any{8.0}, 4, 8},
		{[]any{}, 4, 4},
		{true, 4, 4},
		{map[string]any{}, 4, 4},
	}
	for _, c := range cases {
		o := mustOptions(t, map[string]any{"a": c.value})
		if got := o.GetNumber("a", c.def); got != c.want {
			t.Errorf("GetNumber(%#v) = %d, want %d", c.value, got, c.want)
		}
	}
	if got := mustOptions(t, nil).GetNumber("a", 1); got != 1 {
		t.Errorf("default = %d", got)
	}
}

func TestGetArray(t *testing.T) {
	if got := mustOptions(t, nil).GetArray("", nil); len(got) != 0 {
		t.Errorf("got %q", got)
	}
	ab := []jsrt.String{"a", "b"}
	if got := mustOptions(t, nil).GetArray("a", ab); !reflect.DeepEqual(got, ab) {
		t.Errorf("got %q", got)
	}
	cd := []jsrt.String{"c", "d"}
	if got := mustOptions(t, map[string]any{"a": []any{"c", "d"}}).GetArray("a", ab); !reflect.DeepEqual(got, cd) {
		t.Errorf("got %q", got)
	}
	if got := mustOptions(t, map[string]any{"a": "c,d"}).GetArray("a", ab); !reflect.DeepEqual(got, cd) {
		t.Errorf("got %q", got)
	}
}

func TestIsValidSelection(t *testing.T) {
	ab := []any{jsrt.String("a"), jsrt.String("b")}
	if IsValidSelection(ab, nil) {
		t.Error("an empty selection list is invalid")
	}
	if IsValidSelection(ab, []jsrt.String{"c"}) {
		t.Error("a missing selection is invalid")
	}
	if !IsValidSelection(ab, []jsrt.String{"a", "b"}) {
		t.Error("a present selection is valid")
	}
}

func TestGetSelectionList(t *testing.T) {
	o := mustOptions(t, nil)
	if _, err := o.GetSelectionList("a", nil, nil); err == nil || err.Error() != "Selection list cannot be empty." {
		t.Errorf("empty list: %v", err)
	}
	if _, err := o.GetSelectionList("a", []jsrt.String{"a", "b"}, []jsrt.String{"c"}); err == nil || err.Error() != "Invalid Default Value!" {
		t.Errorf("invalid default: %v", err)
	}
	o = mustOptions(t, map[string]any{"a": []any{"c", "d"}})
	_, err := o.GetSelectionList("a", []jsrt.String{"a", "b"}, []jsrt.String{"a"})
	want := "Invalid Option Value: The option 'a' can contain only the following values:\na,b\nYou passed in: 'c,d'"
	if err == nil || err.Error() != want {
		t.Errorf("invalid option: %v", err)
	}
	o = mustOptions(t, map[string]any{"a": []any{"a"}})
	if got, err := o.GetSelectionList("a", []jsrt.String{"a", "b"}, []jsrt.String{"a"}); err != nil || !reflect.DeepEqual(got, []jsrt.String{"a"}) {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestGetSelection(t *testing.T) {
	o := mustOptions(t, map[string]any{"a": []any{"a", "b"}})
	_, err := o.GetSelection("a", []jsrt.String{"a", "b"}, []jsrt.String{"a"})
	want := "Invalid Option Value: The option 'a' can only be one of the following values:\na,b\nYou passed in: 'a,b'"
	if err == nil || err.Error() != want {
		t.Errorf("multiple selection: %v", err)
	}
	o = mustOptions(t, map[string]any{"a": []any{"a"}})
	if got, err := o.GetSelection("a", []jsrt.String{"a", "b"}, []jsrt.String{"a"}); err != nil || got != "a" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestSharedOptions(t *testing.T) {
	o := mustOptions(t, map[string]any{"indent_char": "\t", "indent_size": 1.0, "preserve_newlines": false, "max_char": 30.0})
	if !o.IndentWithTabs || o.IndentSize != 4 || o.MaxPreserveNewlines != 0 || o.WrapLineLength != 30 {
		t.Errorf("got %+v", o)
	}
	if _, err := NewOptions(map[string]any{"templating": "bogus"}, ""); err == nil {
		t.Error("an invalid templating value should fail")
	}
}
