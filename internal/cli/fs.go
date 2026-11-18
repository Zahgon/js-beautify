package cli

import (
	"errors"
	"os"
	"strings"
	"syscall"

	"github.com/beautifier/js-beautify-go/internal/nodefmt"
)

// mkdirp is fs.mkdirSync(path, {recursive: true}) as Node implements it (MKDirpSync): missing
// ancestors are created first; an existing non-directory is EEXIST when it is the target and
// ENOTDIR when it is an ancestor. display is the path as given, which errors name; full is the
// same path resolved.
func mkdirp(display, full string) error {
	fail := func(err error) error { return nodefmt.SysError(err, "mkdir", display) }
	stack := []string{full}
	for len(stack) > 0 {
		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		err := syscall.Mkdir(next, 0o777)
		var errno syscall.Errno
		if err != nil && !errors.As(err, &errno) {
			return fail(err)
		}
		switch {
		case err == nil:
		case errno == syscall.EACCES || errno == syscall.ENOSPC || errno == syscall.ENOTDIR || errno == syscall.EPERM:
			return fail(err)
		case errno == syscall.ENOENT:
			dirname := next
			if i := strings.LastIndexByte(next, '/'); i >= 0 {
				dirname = next[:i]
			}
			if dirname == next {
				return fail(syscall.EEXIST)
			}
			stack = append(stack, next, dirname)
		default:
			st, statErr := os.Stat(next)
			if statErr == nil && !st.IsDir() {
				if errno == syscall.EEXIST && len(stack) > 0 {
					return fail(syscall.ENOTDIR)
				}
				return fail(syscall.EEXIST)
			}
			if statErr != nil {
				return fail(statErr)
			}
		}
	}
	return nil
}
