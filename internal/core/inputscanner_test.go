package core

import (
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// Ported from js/test/core/test_inputscanner.js.

func re(source jsrt.String) *jsrt.Regexp { return jsrt.MustCompile(source, "") }

func TestInputScannerNext(t *testing.T) {
	in := NewInputScanner("howdy")
	if got := in.Next(); got != "h" {
		t.Errorf("Next = %q, want h", got)
	}
	in.ReadUntilAfter(re("howdy"))
	if got := in.Next(); got != "" {
		t.Errorf("Next at the end = %q, want null (\"\")", got)
	}
}

func TestInputScannerPeek(t *testing.T) {
	in := NewInputScanner("howdy")
	if got := in.Peek(3); got != "d" {
		t.Errorf("Peek(3) = %q", got)
	}
	in.Next()
	if got := in.Peek(3); got != "y" {
		t.Errorf("Peek(3) after Next = %q", got)
	}
	in.Restart()
	if in.Peek(-2) != "" || in.Peek(5) != "" {
		t.Error("Peek outside the input should return null (\"\")")
	}
	if got := in.Peek(0); got != "h" {
		t.Errorf("Peek(0) = %q", got)
	}
}

func TestInputScannerTest(t *testing.T) {
	in := NewInputScanner("howdy")
	if !in.Test(re("how"), 0) {
		t.Error("Test(/how/) at 0 should match")
	}
	in.Next()
	if in.Test(re("how"), 0) {
		t.Error("Test(/how/) at 1 should not match")
	}
	in.Restart()
	if !in.TestChar(re("o"), 1) {
		t.Error("TestChar(/o/, 1) should match")
	}
}

func TestInputScannerRestartAndBack(t *testing.T) {
	in := NewInputScanner("howdy")
	in.Next()
	if in.Peek(0) != "o" {
		t.Fatal("Peek after Next")
	}
	in.Restart()
	if in.Peek(0) != "h" {
		t.Error("Restart should go back to 0")
	}
	in.Next()
	in.Back()
	if in.Peek(0) != "h" {
		t.Error("Back should move one place back")
	}
	in.Back()
	if in.Peek(0) != "h" {
		t.Error("Back at 0 should not move")
	}
}

func TestInputScannerHasNext(t *testing.T) {
	in := NewInputScanner("howdy")
	in.ReadUntilAfter(re("howd"))
	if !in.HasNext() {
		t.Error("HasNext before the last unit")
	}
	in.ReadUntilAfter(re("howdy"))
	if in.HasNext() {
		t.Error("HasNext at the end")
	}
}

func TestInputScannerMatch(t *testing.T) {
	in := NewInputScanner("howdy")
	m := in.Match(re("how"))
	if m == nil || m.Text() != "how" || in.Peek(0) != "d" {
		t.Errorf("Match(/how/) = %v, position at %q", m, in.Peek(0))
	}
	in = NewInputScanner("howdy")
	if m := in.Match(re("test")); m != nil || in.Peek(0) != "h" {
		t.Error("a failed Match should return nil and not move")
	}
}

func TestInputScannerRead(t *testing.T) {
	cases := []struct {
		start, until *jsrt.Regexp
		after        bool
		want         jsrt.String
	}{
		{re("how"), nil, false, "how"},
		{re("ow"), nil, false, ""},
		{re("how"), re("dy"), true, "howdy"},
		{re("how"), re("dy"), false, "how"},
		{nil, re("how"), true, "how"},
		{nil, re("how"), false, ""},
	}
	for i, c := range cases {
		if got := NewInputScanner("howdy").Read(c.start, c.until, c.after); got != c.want {
			t.Errorf("case %d: Read = %q, want %q", i, got, c.want)
		}
	}
}

func TestInputScannerReadUntil(t *testing.T) {
	if got := NewInputScanner("howdy").ReadUntil(re("how"), true); got != "how" {
		t.Errorf("ReadUntil(/how/, true) = %q", got)
	}
	if got := NewInputScanner("howdy").ReadUntil(re("wd"), false); got != "ho" {
		t.Errorf("ReadUntil(/wd/) = %q", got)
	}
	if got := NewInputScanner("howdy").ReadUntil(re("how"), false); got != "" {
		t.Errorf("ReadUntil(/how/) = %q", got)
	}
	if got := NewInputScanner("howdy").ReadUntilAfter(re("how")); got != "how" {
		t.Errorf("ReadUntilAfter(/how/) = %q", got)
	}
}

func TestInputScannerGetRegexp(t *testing.T) {
	in := NewInputScanner("howdy")
	if got := in.GetRegexp(re("ow"), true).String(); got != "/ow/y" {
		t.Errorf("GetRegexp(/ow/, true) = %s", got)
	}
	if got := in.GetRegexp(re("ow"), false).String(); got != "/ow/g" {
		t.Errorf("GetRegexp(/ow/) = %s", got)
	}
	if got := in.GetLiteralRegexp("ow$/h").String(); got != `/ow\$\/h/` {
		t.Errorf("GetLiteralRegexp = %s", got)
	}
}

func TestInputScannerPeekUntilAfter(t *testing.T) {
	in := NewInputScanner("howdy")
	if got := in.PeekUntilAfter(re("how")); got != "how" || in.Peek(0) != "h" {
		t.Errorf("PeekUntilAfter = %q, position at %q", got, in.Peek(0))
	}
}

func TestInputScannerLookBack(t *testing.T) {
	in := NewInputScanner("howdy")
	in.ReadUntilAfter(re("howd"))
	if !in.LookBack("how") {
		t.Error(`LookBack("how") should be true`)
	}
	if in.LookBack("ho") {
		t.Error(`LookBack("ho") should be false`)
	}
	in = NewInputScanner("URL(")
	in.ReadUntilAfter(re(`\(`))
	if !in.LookBack("url") {
		t.Error("LookBack compares the lowercased text")
	}
}

func TestInputScannerSurrogates(t *testing.T) {
	in := NewInputScanner(jsrt.FromUTF8("\U0001F600"))
	if a, b := in.Next(), in.Next(); a.CharCodeAt(0) != 0xD83D || b.CharCodeAt(0) != 0xDE00 {
		t.Errorf("an astral character should be read as two code units, got %q %q", a, b)
	}
}
