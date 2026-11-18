package editorconfig

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// This file reproduces the INI dialect of @one-ini/wasm 0.2.1, the parser editorconfig 3 uses.
// It is compiled WebAssembly; its rules, as its output shows them, are:
//
//   - The input must be valid UTF-8, or parsing fails as a whole.
//   - Lines end at \r\n, \n or \r. Spaces and tabs (only those) around a line are ignored, and at
//     the very start of the file one byte order mark is skipped too.
//   - "[name]" (name not empty, anything up to the last "]" of the line) opens a section.
//   - "key = value", split at the first "=", is a pair; the key must not be empty or start
//     with "[", and both sides are trimmed. A pair wins over a comment: "#a=b" is the key "#a".
//   - A line starting with "#" or ";" is a comment, an empty line is skipped.
//   - Anything else is an error: the result is what was parsed before that line.

// section is a section of an INI file: its name ("" for the preamble before the first header)
// and its pairs in order.
type section struct {
	name  string
	pairs [][2]string
}

const byteOrderMark = "\uFEFF"

// errInvalidUTF8 reports input that is not UTF-8; one-ini refuses to parse it at all.
var errInvalidUTF8 = errors.New("invalid utf-8 sequence")

func trimBlanks(s string) string {
	return strings.Trim(s, " \t")
}

// parseINI parses an INI file into its preamble and sections.
func parseINI(data []byte) ([]section, error) {
	if !utf8.Valid(data) {
		return nil, errInvalidUTF8
	}
	text := strings.TrimLeft(string(data), " \t")
	text = strings.TrimPrefix(text, byteOrderMark)
	sections := []section{{}}
	cur := &sections[0]
	for _, raw := range splitLines(text) {
		line := trimBlanks(raw)
		switch {
		case line == "":
		case line[0] == '[':
			if len(line) < 3 || line[len(line)-1] != ']' {
				return sections, nil
			}
			sections = append(sections, section{name: line[1 : len(line)-1]})
			cur = &sections[len(sections)-1]
		case strings.Contains(line, "="):
			key, value, _ := strings.Cut(line, "=")
			key = trimBlanks(key)
			if key == "" {
				return sections, nil
			}
			cur.pairs = append(cur.pairs, [2]string{key, trimBlanks(value)})
		case line[0] == '#' || line[0] == ';':
		default:
			return sections, nil
		}
	}
	return sections, nil
}

// splitLines splits text at \r\n, \n and \r.
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			lines = append(lines, text[start:i])
			start = i + 1
		case '\r':
			lines = append(lines, text[start:i])
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	return append(lines, text[start:])
}
