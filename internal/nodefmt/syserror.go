package nodefmt

import (
	"errors"
	"math"
	"strconv"
	"syscall"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
	"github.com/beautifier/js-beautify-go/internal/jsvalue"
)

// errnoNames maps the error numbers the command line can meet to libuv's code and description,
// which Node puts in the message of the exceptions its fs functions throw.
var errnoNames = map[syscall.Errno][2]string{
	syscall.EACCES:       {"EACCES", "permission denied"},
	syscall.EBADF:        {"EBADF", "bad file descriptor"},
	syscall.EBUSY:        {"EBUSY", "resource busy or locked"},
	syscall.EEXIST:       {"EEXIST", "file already exists"},
	syscall.EFBIG:        {"EFBIG", "file too large"},
	syscall.EINVAL:       {"EINVAL", "invalid argument"},
	syscall.EIO:          {"EIO", "i/o error"},
	syscall.EISDIR:       {"EISDIR", "illegal operation on a directory"},
	syscall.ELOOP:        {"ELOOP", "too many symbolic links encountered"},
	syscall.EMFILE:       {"EMFILE", "too many open files"},
	syscall.ENAMETOOLONG: {"ENAMETOOLONG", "name too long"},
	syscall.ENFILE:       {"ENFILE", "file table overflow"},
	syscall.ENOENT:       {"ENOENT", "no such file or directory"},
	syscall.ENOSPC:       {"ENOSPC", "no space left on device"},
	syscall.ENOTDIR:      {"ENOTDIR", "not a directory"},
	syscall.ENOTEMPTY:    {"ENOTEMPTY", "directory not empty"},
	syscall.EPERM:        {"EPERM", "operation not permitted"},
	syscall.EROFS:        {"EROFS", "read-only file system"},
	syscall.ETXTBSY:      {"ETXTBSY", "text file is busy"},
	syscall.EXDEV:        {"EXDEV", "cross-device link not permitted"},
}

// Errno returns the system error number err carries, if any.
func Errno(err error) (syscall.Errno, bool) {
	var errno syscall.Errno
	ok := errors.As(err, &errno)
	return errno, ok
}

// Code returns the libuv code ("ENOENT"...) of the system error err carries, or "".
func Code(err error) string {
	if errno, ok := Errno(err); ok {
		if names, known := errnoNames[errno]; known {
			return names[0]
		}
	}
	return ""
}

// SysError returns the exception Node's fs throws when the system call syscall fails on path
// with err: "Error: CODE: description, syscall 'path'" with the errno, code, syscall and path
// properties. An empty path is left out, as for a read on an open file.
func SysError(err error, syscall, path string) *Error {
	errno, ok := Errno(err)
	names, known := errnoNames[errno]
	if !ok || !known {
		return &Error{Name: "Error", Message: err.Error(), Frame: "fs." + syscall}
	}
	msg := names[0] + ": " + names[1] + ", " + syscall
	props := []Prop{{"errno", -int(errno)}, {"code", names[0]}, {"syscall", syscall}}
	if path != "" {
		msg += " '" + path + "'"
		props = append(props, Prop{"path", path})
	}
	return &Error{Name: "Error", Message: msg, Props: props, Frame: "fs." + syscall}
}

// InvalidArgType returns Node's ERR_INVALID_ARG_TYPE exception for an argument that must be a
// string but is the JavaScript value v, which is not a string.
func InvalidArgType(argName string, v any, frame string) *Error {
	return &Error{
		Name:    "TypeError [ERR_INVALID_ARG_TYPE]",
		Message: `The "` + argName + `" argument must be of type string. Received ` + describeValue(v),
		Props:   []Prop{{"code", "ERR_INVALID_ARG_TYPE"}},
		Frame:   frame,
	}
}

// describeValue is Node's determineSpecificType, which names the type (and value) of a wrong
// argument, for the non-string values a JavaScript value model holds.
func describeValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "type boolean (" + strconv.FormatBool(x) + ")"
	case float64:
		if x == 0 && math.Signbit(x) {
			return "type number (-0)"
		}
		return "type number (" + string(jsrt.NumberToString(x)) + ")"
	case []any:
		return "an instance of Array"
	case *jsvalue.Object:
		return "an instance of Object"
	}
	if jsvalue.IsUndefined(v) {
		return "undefined"
	}
	return "an instance of Object"
}
