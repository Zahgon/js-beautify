package jsrt

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
)

// jsString is a JSON string decoded with UnquoteJSON, so that lone surrogates survive.
type jsString String

func (s *jsString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	v, err := UnquoteJSON(b)
	*s = jsString(v)
	return err
}

type regexCase struct {
	Pattern, Input jsString
	Flags          string
	LastIndex      int
	Index          int
	Groups         *[]*jsString
	CanonicalFlags string
	Err            string
}

type fixture struct {
	Regex   []regexCase
	Replace []struct {
		Pattern, Input, Replacement, Out jsString
		Flags                            string
	}
	Split []struct {
		Pattern, Input jsString
		Flags          string
		Out            []jsString
	}
	Numbers []struct {
		Value   any
		NegZero bool
		Out     string
	}
	ParseInt []struct {
		Input   jsString
		Out     *float64
		NegZero bool
	}
	Lower []struct{ Input, Out jsString }
}

func loadFixture(t *testing.T) *fixture {
	t.Helper()
	data, err := os.ReadFile("testdata/jsrt.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

func TestRegexpAgainstV8(t *testing.T) {
	checkRegexCases(t, loadFixture(t).Regex)
}

// TestBeautifierPatternsAgainstV8 runs the regexes js-beautify's tokenizers build on generated
// inputs (testdata/record_patterns.js).
func TestBeautifierPatternsAgainstV8(t *testing.T) {
	f, err := os.Open("testdata/patterns.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var data struct{ Regex []regexCase }
	if err := json.NewDecoder(zr).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Regex) < 1000 {
		t.Fatalf("only %d cases", len(data.Regex))
	}
	checkRegexCases(t, data.Regex)
}

func checkRegexCases(t *testing.T, cases []regexCase) {
	t.Helper()
	for _, c := range cases {
		name := fmt.Sprintf("/%s/%s on %q", c.Pattern, c.Flags, c.Input)
		re, err := Compile(String(c.Pattern), c.Flags)
		if c.Err != "" {
			if err == nil || err.Error() != c.Err {
				t.Errorf("%s: error %v, want %q", name, err, c.Err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
			continue
		}
		if re.Flags() != c.CanonicalFlags {
			t.Errorf("%s: flags %q, want %q", name, re.Flags(), c.CanonicalFlags)
		}
		m := re.ExecUnits(String(c.Input).Units(), c.LastIndex)
		if c.Groups == nil {
			if m != nil {
				t.Errorf("%s: matched %q, want no match", name, m.Text())
			}
			continue
		}
		if m == nil {
			t.Errorf("%s: no match, want %q", name, *(*c.Groups)[0])
			continue
		}
		if m.Index() != c.Index {
			t.Errorf("%s: index %d, want %d", name, m.Index(), c.Index)
		}
		if m.NumGroups()+1 != len(*c.Groups) {
			t.Errorf("%s: %d groups, want %d", name, m.NumGroups()+1, len(*c.Groups))
			continue
		}
		for i, want := range *c.Groups {
			got, ok := m.Group(i)
			if (want == nil) == ok || (want != nil && got != String(*want)) {
				t.Errorf("%s: group %d = %q (%v), want %v", name, i, got, ok, want)
			}
		}
	}
}

func TestReplaceAndSplitAgainstV8(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.Replace {
		re := MustCompile(String(c.Pattern), c.Flags)
		if got := re.Replace(String(c.Input), String(c.Replacement)); got != String(c.Out) {
			t.Errorf("%q.replace(/%s/%s, %q) = %q, want %q", c.Input, c.Pattern, c.Flags, c.Replacement, got, c.Out)
		}
	}
	for _, c := range f.Split {
		re := MustCompile(String(c.Pattern), c.Flags)
		got := re.Split(String(c.Input))
		if len(got) != len(c.Out) {
			t.Errorf("%q.split(/%s/) = %q, want %q", c.Input, c.Pattern, got, c.Out)
			continue
		}
		for i := range got {
			if got[i] != String(c.Out[i]) {
				t.Errorf("%q.split(/%s/)[%d] = %q, want %q", c.Input, c.Pattern, i, got[i], c.Out[i])
			}
		}
	}
}

func TestNumbersAgainstV8(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.Numbers {
		var x float64
		switch v := c.Value.(type) {
		case float64:
			x = v
			if c.NegZero {
				x = math.Copysign(0, -1)
			}
		case string:
			x, _ = strconv.ParseFloat(v, 64)
		}
		if got := NumberToString(x); got != String(c.Out) {
			t.Errorf("NumberToString(%v) = %q, want %q", x, got, c.Out)
		}
	}
	for _, c := range f.ParseInt {
		got := ParseInt(String(c.Input), 10)
		switch {
		case c.Out == nil:
			if !math.IsNaN(got) {
				t.Errorf("ParseInt(%q) = %v, want NaN", c.Input, got)
			}
		case got != *c.Out || (got == 0 && math.Signbit(got) != c.NegZero):
			t.Errorf("ParseInt(%q) = %v, want %v (negative zero %v)", c.Input, got, *c.Out, c.NegZero)
		}
	}
	for _, c := range f.Lower {
		if got := String(c.Input).ToLowerCase(); got != String(c.Out) {
			t.Errorf("%q.toLowerCase() = %q, want %q", c.Input, got, c.Out)
		}
	}
}

func TestStringUnits(t *testing.T) {
	s := FromUTF8("aé\U0001F600b")
	if s.Len() != 5 {
		t.Fatalf("Len = %d, want 5", s.Len())
	}
	if got := s.CharCodeAt(2); got != 0xD83D {
		t.Errorf("CharCodeAt(2) = %#x, want 0xd83d", got)
	}
	if got := s.Substring(0, 3).UTF8(); got != "aé�" {
		t.Errorf("lone surrogate written as %q", got)
	}
	if got := (s.Substring(0, 3) + s.SubstringFrom(3)).UTF8(); got != "aé\U0001F600b" {
		t.Errorf("rejoined halves give %q", got)
	}
	if got := s.IndexOf("b"); got != 4 {
		t.Errorf("IndexOf = %d, want 4", got)
	}
	if got := s.Slice(-2, 5); got != s.Substring(3, 5) {
		t.Errorf("Slice(-2) = %q", got)
	}
	if got := FromUTF8("  x ").Trim(); got != "x" {
		t.Errorf("Trim = %q", got)
	}
	if got := String("a,b,,c").Split(","); len(got) != 4 || got[2] != "" {
		t.Errorf("Split = %q", got)
	}
	if got := String("abcb").Replace("b", "[$&$$]"); got != "a[b$]cb" {
		t.Errorf("Replace = %q", got)
	}
	if got := String("abcb").ReplaceAll("b", "-"); got != "a-c-" {
		t.Errorf("ReplaceAll = %q", got)
	}
}

func TestValues(t *testing.T) {
	cases := []struct {
		v      any
		truthy bool
		str    String
	}{
		{nil, false, "null"},
		{true, true, "true"},
		{0.0, false, "0"},
		{math.NaN(), false, "NaN"},
		{2, true, "2"},
		{"", false, ""},
		{"false", true, "false"},
		{[]any{}, true, ""},
		{[]any{1.0, []any{2.0, nil}, "x"}, true, "1,2,,x"},
		{map[string]any{}, true, "[object Object]"},
	}
	for _, c := range cases {
		if got := Truthy(c.v); got != c.truthy {
			t.Errorf("Truthy(%v) = %v", c.v, got)
		}
		if got := ToString(c.v); got != c.str {
			t.Errorf("ToString(%v) = %q, want %q", c.v, got, c.str)
		}
	}
	if !ObjectPrototypeHas("constructor") || ObjectPrototypeHas("media") {
		t.Error("ObjectPrototypeHas")
	}
	if s, err := ArrayJoinRepeat(3, "ab"); err != nil || s != "abab" {
		t.Errorf("ArrayJoinRepeat(3) = %q, %v", s, err)
	}
	if _, err := ArrayJoinRepeat(-1, " "); err != ErrInvalidArrayLength {
		t.Errorf("ArrayJoinRepeat(-1) error %v", err)
	}
	if _, err := ArrayJoinRepeat(600000001, " "); err != ErrInvalidStringLength {
		t.Errorf("ArrayJoinRepeat(6e8) error %v", err)
	}
}

func TestUnquoteJSON(t *testing.T) {
	got, err := UnquoteJSON([]byte(`"a\u00e9\ud83d\n\"😀"`))
	if err != nil {
		t.Fatal(err)
	}
	want := FromUnits([]uint16{'a', 0xE9, 0xD83D, '\n', '"', 0xD83D, 0xDE00})
	if got != want {
		t.Errorf("UnquoteJSON = %q, want %q", got, want)
	}
	for _, bad := range []string{`abc`, `"\x"`, `"\u12"`, `"\"`} {
		if _, err := UnquoteJSON([]byte(bad)); err == nil {
			t.Errorf("UnquoteJSON(%s) should fail", bad)
		}
	}
}

func TestRegexpHelpers(t *testing.T) {
	re := MustCompile(`(?<d>\d)(x)?`, "g")
	if got := re.FindAll("a1b22"); len(got) != 3 || got[2] != "2" {
		t.Errorf("FindAll = %q", got)
	}
	if got := MustCompile("x*", "g").FindAll("ab"); len(got) != 3 {
		t.Errorf("FindAll of empty matches = %q", got)
	}
	got := re.ReplaceFunc("a1b2", func(m *Match) String {
		d, _ := m.NamedGroup("d")
		_, hasX := m.Group(2)
		if hasX {
			return "?"
		}
		return "<" + d + ">"
	})
	if got != "a<1>b<2>" {
		t.Errorf("ReplaceFunc = %q", got)
	}
	if got := MustCompile("b", "").Search("aab"); got != 2 {
		t.Errorf("Search = %d", got)
	}
	if m := MustCompile("b", "").SearchFrom(String("babb").Units(), 1); m == nil || m.Index() != 2 {
		t.Errorf("SearchFrom = %v", m)
	}
	sticky, err := MustCompile("a", "i").WithFlags("y")
	if err != nil || sticky.Test("A") || !sticky.Test("a") || !sticky.Sticky() {
		t.Errorf("WithFlags(y) should drop the i flag and match only at lastIndex: %v %v", sticky, err)
	}
	if got := MustCompile("", "").Source(); got != "(?:)" {
		t.Errorf("Source of an empty pattern = %q", got)
	}
}
