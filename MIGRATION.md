# Migration from the JavaScript release

This repository was the JavaScript implementation of js-beautify 2.0.3 (with an older Python port of
the JavaScript and CSS beautifiers). It is now a Go module, `github.com/beautifier/js-beautify-go`,
that reproduces the JavaScript release's output byte for byte.

| JavaScript | Go |
| :-- | :-- |
| `require('js-beautify').js(source, options)` | `beautify.JS(source, options)` |
| `require('js-beautify').css(source, options)` | `beautify.CSS(source, options)` |
| `require('js-beautify').html(source, options)` | `beautify.HTML(source, options)` |
| `js-beautify`, `css-beautify`, `html-beautify` (Node) | `cmd/js-beautify`, `cmd/css-beautify`, `cmd/html-beautify`, or `beautify.CLI(name, …)` |
| a thrown `Error` | a returned `error` with the same message |

## Layout

| Package | Ports |
| :-- | :-- |
| `internal/jsrt` | the JavaScript semantics the beautifiers rely on: UTF-16 strings, ECMAScript regular expressions as V8 runs them, number formatting and parsing, truthiness |
| `internal/core` | `js/src/core`: options, input scanner, output, tokens, tokenizer, patterns, directives |
| `internal/javascript`, `internal/css`, `internal/html` | `js/src/javascript`, `js/src/css`, `js/src/html` |
| `internal/cli` | `js/src/cli.js` |
| `internal/nopt` | nopt 10.0.1 with abbrev 5.0.0 |
| `internal/configchain` | config-chain 1.1.13 with proto-list 1.2.4 |
| `internal/editorconfig` | editorconfig 3.0.2 (`parseSync`) and the INI dialect of @one-ini/wasm 0.2.1 |
| `internal/minimatch`, `internal/glob` | minimatch 10.2.5 with brace-expansion 5.0.6, glob 13.0.6 (sync walk) |
| `internal/jsvalue`, `internal/nodefmt`, `internal/nodepath` | JavaScript objects and `JSON.parse` with V8's messages, Node's `util.inspect` of errors and its fs errors, `path.posix` |

Each package's tests replay results recorded from the JavaScript release on Node.js 24; the recorders
sit beside the fixtures under `testdata/`.

## Differences

- Option objects are Go maps, which have no key order. When an object holds the same option spelled
  with dashes and with underscores, the dashed spelling is treated as the later key.
- Strings are UTF-8 at the API boundary. Where the JavaScript release's result would hold a lone
  surrogate, the Go result holds U+FFFD, which is what Node writes for it.
- The commands ignore `DEBUG_JSBEAUTIFY`/`JSBEAUTIFY_DEBUG`.
- File-name comparison in globbing uses the names as stored; the JavaScript release compares them after
  Unicode NFKD normalisation, which differs only for names that are not in NFC.
- POSIX character classes in glob patterns cover the Basic Multilingual Plane.
- The unpackers (`js/src/unpackers`) and the web demo are not part of the port.
