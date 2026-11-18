// Package core is a port of js-beautify's js/src/core: the pieces the JavaScript, CSS and HTML
// beautifiers share. Text is jsrt.String (UTF-16 semantics) and regexes are *jsrt.Regexp.
//
// # Mapping from the original
//
//	options.js           Options, NewOptions(raw, "js"|"css"|"html"), MergeOpts, NormalizeOpts;
//	                     _get_boolean/_get_number/_get_characters/_get_array/_get_selection/
//	                     _get_selection_list are GetBoolean, GetNumber, GetCharacters, GetArray,
//	                     GetSelection, GetSelectionList. Language options embed *Options.
//	inputscanner.js      InputScanner: Next, Peek(i), Back, HasNext, Restart, Test, TestChar,
//	                     Match, Read(start, until, untilAfter), ReadUntil, ReadUntilAfter,
//	                     PeekUntilAfter, LookBack, GetRegexp, GetLiteralRegexp. null is "".
//	output.js            Output and OutputLine; the IndentStringCache is internal. Fields that
//	                     the beautifiers assign (space_before_token, non_breaking_space, raw,
//	                     current_line, previous_line, next_line, previous_token_wrapped) are
//	                     exported fields; GetCode returns the deferred RangeError, if any.
//	token.js             Token (Type is a string such as "TK_WORD").
//	tokenstream.js       TokenStream.
//	tokenizer.js         Tokenizer, Lexer and the TokenStart/TokenRaw/TokenEOF types.
//	pattern.js           Pattern: StartingWith, Matching, Until, UntilAfter, Read, ReadMatch.
//	whitespacepattern.js WhitespacePattern: Matching(whitespaceChars, newlineChars), Read,
//	                     NewlineCount, WhitespaceBeforeToken.
//	templatablepattern.js TemplatablePattern: the Pattern builders plus Disable, Exclude and
//	                     ReadOptions, taking TemplateLanguage constants (Django, ERB, Handlebars,
//	                     PHP, Smarty, Angular).
//	directives.js        Directives: GetDirectives, ReadIgnored.
//
// Method names are the original's in Go case (add_new_line is AddNewLine, read_match is
// ReadMatch). Optional JavaScript arguments are explicit: Peek(0) is peek(), SetIndent(i, 0) is
// set_indent(i), AddNewLine(false) is add_new_line().
//
// # Extension points
//
// Subclassing becomes embedding:
//
//   - A tokenizer embeds *Tokenizer (created by NewTokenizer), redefines the Lexer methods it
//     overrides (GetNextToken, IsComment, IsOpening, IsClosing, Reset) and calls
//     t.Tokenize(t) so that the base loop dispatches to them. CreateToken, ReadWhitespace,
//     IsFirstToken and the Input and Whitespace fields are the protected helpers of the original.
//   - Patterns are immutable values: every builder returns a modified copy of the receiver's
//     own type, so `templatable.Until(re).Exclude(Handlebars)` reads as in the original.
//   - Language options embed *Options and read their own keys from Raw after NewOptions.
package core
