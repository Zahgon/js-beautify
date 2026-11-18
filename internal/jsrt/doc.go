// Package jsrt reproduces the pieces of JavaScript runtime semantics that js-beautify depends on,
// so that the ports of its beautifiers can be written line for line against the original.
//
// # Text
//
// JavaScript strings are sequences of UTF-16 code units. js-beautify measures, indexes, slices and
// regex-matches them unit by unit, and its scanners can split an astral character into two lone
// surrogates. Two representations are used:
//
//   - []uint16 for scanning: InputScanner and the regex engine work on code-unit slices with
//     code-unit indexes (String.Units and FromUnits convert).
//   - String for values: tokens, output fragments and option values. A String is a Go string in
//     which every code unit is encoded on its own (CESU-8: surrogates, paired or not, take three
//     bytes each). Every code-unit sequence therefore has exactly one encoding, so ==, map keys,
//     switch statements, + and comparisons with ASCII or BMP Go literals behave as in JavaScript.
//     len() counts bytes, not code units: use Len, At, Substring and the other methods wherever
//     the original uses .length, charAt, indexOf or substring.
//
// Conversions at the API boundary: FromUTF8 turns Go (UTF-8) text into a String, String.UTF8 turns a
// String back into UTF-8 the way Node writes it out (each lone surrogate becomes U+FFFD).
//
// # Where things live
//
//   - string.go: String, its conversions and the String.prototype methods (Len, At, CharAt,
//     Substring, Slice, IndexOf, LastIndexOf, StartsWith/EndsWith, Split, Replace, Trim,
//     ToLowerCase) and Join.
//   - unicode.go: the JS \s set (IsWhitespace), line terminators, \w and \d, ToLowerCase with
//     JavaScript's full case mapping, and the case canonicalisation used by /i regexes.
//   - number.go: Number::toString (NumberToString) and parseInt (ParseInt).
//   - stringtonumber.go: Number(s) for strings (StringToNumber).
//   - value.go: JSON-typed values as JavaScript sees them: Truthy (!!v), ToString (String(v)),
//     AsNumber, ObjectPrototypeHas (the `in` operator on a plain object), ArrayJoinRepeat
//     (new Array(n).join(s), with V8's RangeErrors).
//   - json.go: UnquoteJSON, a JSON string decoder that keeps lone surrogates as JSON.parse does.
//   - regexp.go, regexp_parse.go, regexp_match.go: a backtracking engine with ECMAScript
//     (non-unicode, Annex B) syntax and semantics over UTF-16, plus the RegExp.prototype.exec and
//     String.prototype.match/replace/split/search algorithms built on it.
package jsrt
