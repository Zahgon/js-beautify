// Package nodepath reproduces Node's path.posix functions (path.resolve, relative, join, normalize,
// dirname and parse) for the code paths that the js-beautify command line takes.
//
// The functions differ from Go's path/filepath in the corners that matter for output: resolve and
// join keep ".." above the root of a relative path, an empty path means ".", dirname keeps a
// leading "//" and ignores trailing separators.
package nodepath

import "strings"

// normalizeString resolves "." and ".." segments of path, as Node's normalizeString does. When
// allowAboveRoot is true, ".." segments that climb above the start are kept.
func normalizeString(path string, allowAboveRoot bool) string {
	res := ""
	lastSegmentLength := 0
	lastSlash := -1
	dots := 0
	var code byte
	for i := 0; i <= len(path); i++ {
		if i < len(path) {
			code = path[i]
		} else if code == '/' {
			break
		} else {
			code = '/'
		}
		if code == '/' {
			switch {
			case lastSlash == i-1 || dots == 1:
				// A run of slashes or a "." segment: nothing to add.
			case dots == 2:
				if len(res) < 2 || lastSegmentLength != 2 || !strings.HasSuffix(res, "..") {
					if len(res) > 2 {
						lastSlashIndex := strings.LastIndexByte(res, '/')
						if lastSlashIndex == -1 {
							res = ""
							lastSegmentLength = 0
						} else {
							res = res[:lastSlashIndex]
							lastSegmentLength = len(res) - 1 - strings.LastIndexByte(res, '/')
						}
						lastSlash = i
						dots = 0
						continue
					} else if len(res) != 0 {
						res = ""
						lastSegmentLength = 0
						lastSlash = i
						dots = 0
						continue
					}
				}
				if allowAboveRoot {
					if len(res) > 0 {
						res += "/.."
					} else {
						res = ".."
					}
					lastSegmentLength = 2
				}
			default:
				if len(res) > 0 {
					res += "/" + path[lastSlash+1:i]
				} else {
					res = path[lastSlash+1 : i]
				}
				lastSegmentLength = i - lastSlash - 1
			}
			lastSlash = i
			dots = 0
		} else if code == '.' && dots != -1 {
			dots++
		} else {
			dots = -1
		}
	}
	return res
}

// Resolve returns path.resolve(cwd, segments...): the segments are joined from right to left
// until an absolute path is formed, starting from cwd, then normalized without a trailing slash.
func Resolve(cwd string, segments ...string) string {
	resolved := ""
	absolute := false
	for i := len(segments) - 1; i >= -1 && !absolute; i-- {
		p := cwd
		if i >= 0 {
			p = segments[i]
		}
		if p == "" {
			continue
		}
		resolved = p + "/" + resolved
		absolute = p[0] == '/'
	}
	resolved = normalizeString(resolved, !absolute)
	if absolute {
		return "/" + resolved
	}
	if resolved == "" {
		return "."
	}
	return resolved
}

// Normalize returns path.normalize(p).
func Normalize(p string) string {
	if p == "" {
		return "."
	}
	absolute := p[0] == '/'
	trailing := p[len(p)-1] == '/'
	p = normalizeString(p, !absolute)
	if p == "" {
		if absolute {
			return "/"
		}
		if trailing {
			return "./"
		}
		return "."
	}
	if trailing {
		p += "/"
	}
	if absolute {
		return "/" + p
	}
	return p
}

// Join returns path.join(segments...): the non-empty segments joined with "/" and normalized.
func Join(segments ...string) string {
	joined := ""
	for _, s := range segments {
		if s == "" {
			continue
		}
		if joined == "" {
			joined = s
		} else {
			joined += "/" + s
		}
	}
	if joined == "" {
		return "."
	}
	return Normalize(joined)
}

// Relative returns path.relative(from, to), both resolved against cwd first.
func Relative(cwd, from, to string) string {
	if from == to {
		return ""
	}
	from = Resolve(cwd, from)
	to = Resolve(cwd, to)
	if from == to {
		return ""
	}
	const fromStart = 1
	fromEnd := len(from)
	fromLen := fromEnd - fromStart
	const toStart = 1
	toLen := len(to) - toStart

	length := fromLen
	if toLen < length {
		length = toLen
	}
	lastCommonSep := -1
	i := 0
	for ; i < length; i++ {
		fromCode := from[fromStart+i]
		if fromCode != to[toStart+i] {
			break
		} else if fromCode == '/' {
			lastCommonSep = i
		}
	}
	if i == length {
		if toLen > length {
			if to[toStart+i] == '/' {
				// from is the exact base path of to.
				return to[toStart+i+1:]
			}
			if i == 0 {
				// from is the root.
				return to[toStart+i:]
			}
		} else if fromLen > length {
			if from[fromStart+i] == '/' {
				// to is the exact base path of from.
				lastCommonSep = i
			} else if i == 0 {
				// to is the root.
				lastCommonSep = 0
			}
		}
	}
	out := ""
	for i = fromStart + lastCommonSep + 1; i <= fromEnd; i++ {
		if i == fromEnd || from[i] == '/' {
			if out == "" {
				out += ".."
			} else {
				out += "/.."
			}
		}
	}
	return out + to[toStart+lastCommonSep:]
}

// Dirname returns path.dirname(p).
func Dirname(p string) string {
	if p == "" {
		return "."
	}
	hasRoot := p[0] == '/'
	end := -1
	matchedSlash := true
	for i := len(p) - 1; i >= 1; i-- {
		if p[i] == '/' {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return p[:end]
}

// Parsed is the result of path.parse.
type Parsed struct {
	Root, Dir, Base, Ext, Name string
}

// Parse returns path.parse(p).
func Parse(p string) Parsed {
	var ret Parsed
	if p == "" {
		return ret
	}
	isAbsolute := p[0] == '/'
	start := 0
	if isAbsolute {
		ret.Root = "/"
		start = 1
	}
	startDot := -1
	startPart := 0
	end := -1
	matchedSlash := true
	i := len(p) - 1
	// Track the state of characters (if any) we see before our first dot and after any path
	// separator we find.
	preDotState := 0
	for ; i >= start; i-- {
		code := p[i]
		if code == '/' {
			if !matchedSlash {
				startPart = i + 1
				break
			}
			continue
		}
		if end == -1 {
			matchedSlash = false
			end = i + 1
		}
		if code == '.' {
			if startDot == -1 {
				startDot = i
			} else if preDotState != 1 {
				preDotState = 1
			}
		} else if startDot != -1 {
			preDotState = -1
		}
	}
	if end != -1 {
		startBase := startPart
		if startPart == 0 && isAbsolute {
			startBase = 1
		}
		if startDot == -1 ||
			// We saw a non-dot character immediately before the dot.
			preDotState == 0 ||
			// The (right-most) trimmed path component is exactly "..".
			(preDotState == 1 && startDot == end-1 && startDot == startPart+1) {
			ret.Base = p[startBase:end]
			ret.Name = ret.Base
		} else {
			ret.Name = p[startBase:startDot]
			ret.Base = p[startBase:end]
			ret.Ext = p[startDot:end]
		}
	}
	if startPart > 0 {
		ret.Dir = p[:startPart-1]
	} else if isAbsolute {
		ret.Dir = "/"
	}
	return ret
}
