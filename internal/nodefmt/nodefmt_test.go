package nodefmt

import (
	"syscall"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsvalue"
)

func TestQuoteString(t *testing.T) {
	// Expected values printed by Node 24's util.inspect.
	tests := map[string]string{
		"/tmp/it's":              `"/tmp/it's"`,
		`/tmp/a"b'c`:             "`/tmp/a\"b'c`",
		"/tmp/a\"b'c`d":          `'/tmp/a"b\'c` + "`" + `d'`,
		"/tmp/tab\there\x01\x7f": `'/tmp/tab\there\x01\x7F'`,
		"\u0085é😀":               `'\x85é😀'`,
		`back\slash`:             `'back\\slash'`,
		"new\nline":              `'new\nline'`,
	}
	for in, want := range tests {
		if got := QuoteString(in); got != want {
			t.Errorf("QuoteString(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSysErrorInspect(t *testing.T) {
	err := SysError(&syscallError{syscall.EEXIST}, "mkdir", "/tmp/jsb/run/d/a.js")
	want := "Error: EEXIST: file already exists, mkdir '/tmp/jsb/run/d/a.js'\n    at fs.mkdir {\n  errno: -17,\n  code: 'EEXIST',\n  syscall: 'mkdir',\n  path: '/tmp/jsb/run/d/a.js'\n}"
	if got := err.Inspect(); got != want {
		t.Errorf("Inspect() = %q, want %q", got, want)
	}
	if err.Prop("code") != "EEXIST" || Code(&syscallError{syscall.EISDIR}) != "EISDIR" {
		t.Errorf("unexpected code")
	}
	read := SysError(syscall.EISDIR, "read", "")
	if read.Message != "EISDIR: illegal operation on a directory, read" || len(read.Props) != 3 {
		t.Errorf("read error = %q %v", read.Message, read.Props)
	}
}

type syscallError struct{ errno syscall.Errno }

func (e *syscallError) Error() string { return e.errno.Error() }
func (e *syscallError) Unwrap() error { return e.errno }

func TestInvalidArgType(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{5.0, `The "path" argument must be of type string. Received type number (5)`},
		{true, `The "path" argument must be of type string. Received type boolean (true)`},
		{nil, `The "path" argument must be of type string. Received null`},
		{[]any{}, `The "path" argument must be of type string. Received an instance of Array`},
		{jsvalue.NewObject(), `The "path" argument must be of type string. Received an instance of Object`},
	}
	for _, tt := range tests {
		e := InvalidArgType("path", tt.v, "")
		if e.Message != tt.want || e.Name != "TypeError [ERR_INVALID_ARG_TYPE]" {
			t.Errorf("InvalidArgType(%v) = %s: %s", tt.v, e.Name, e.Message)
		}
	}
}

func TestDecodeUTF8(t *testing.T) {
	// Expected values from Node's buffer.toString('utf8').
	tests := []struct {
		in   []byte
		want string
	}{
		{[]byte("a\xffb"), "a�b"},
		{[]byte("\xe2\x82a"), "�a"},
		{[]byte("\xf0\x9f\x98"), "�"},
		{[]byte("\xed\xa0\x80"), "���"},
		{[]byte("\xc0\x80"), "��"},
		{[]byte("\xef\xbb\xbfok"), "\uFEFFok"},
		{[]byte("\xf4\x90\x80\x80"), "����"},
	}
	for _, tt := range tests {
		if got := DecodeUTF8(tt.in); got != tt.want {
			t.Errorf("DecodeUTF8(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
