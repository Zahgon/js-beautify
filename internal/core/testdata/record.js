// Records how js-beautify's core classes behave on generated scenarios, for the Go tests.
// Usage (Node 24): node record.js JS_BEAUTIFY_DIR | gzip -9 > core.json.gz
// Every scenario holds its inputs and the original's results; core_test.go replays them.
'use strict';
var path = require('path');
var src = path.resolve(process.argv[2], 'js/src/core');
var InputScanner = require(src + '/inputscanner').InputScanner;
var Pattern = require(src + '/pattern').Pattern;
var TemplatablePattern = require(src + '/templatablepattern').TemplatablePattern;
var WhitespacePattern = require(src + '/whitespacepattern').WhitespacePattern;
var Tokenizer = require(src + '/tokenizer').Tokenizer;
var Directives = require(src + '/directives').Directives;
var Output = require(src + '/output').Output;
var Options = require(src + '/options').Options;

var seed = 12345;
function rand() { // mulberry32
  seed |= 0; seed = seed + 0x6D2B79F5 | 0;
  var t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
}
function pick(a) { return a[Math.floor(rand() * a.length)]; }
function int(n) { return Math.floor(rand() * n); }

var PIECES = ['a', 'word', ' ', '\t', '\n', '\r\n', '\r', '<', '>', '{{', '}}', '{{{x}}}', '{{!-- c --}}', '{%', '%}',
  '{% if %}', '{{ v }}', '{# c #}', '<?php echo 1; ?>', '<?= x ?>', '<% e %>', '<%= x %>', '{*c*}', '{literal}x{/literal}',
  '{$smarty}', '"', "'", '=', '/>', ' ', ' ', '😀', 'xé', '}', '{', '%>', '?>', '--}}'];
function text() { var s = ''; for (var i = int(12); i >= 0; i--) { s += pick(PIECES); } return s; }

var TEMPLATING = [['auto'], ['none'], ['django', 'erb', 'handlebars', 'php'], ['angular'], ['smarty'], ['handlebars'],
  ['django'], ['php', 'erb'], ['smarty', 'php']];
var UNTILS = ['[\\n\\r\\t <]', '[\\n\\r\\t <}]', "'", '"', '[\\n\\r\\t =>]|\\/>', '[\\n\\r\\t >\\/]', '}}', '[`\\\\$]', null];
var STARTS = [null, null, '{{', '<', 'a', '[a-z]+'];
var MATCHES = [null, null, null, '[a-z]+', '\\w*'];
var LANGS = ['django', 'erb', 'handlebars', 'php', 'smarty', 'angular'];

function drain(input, read) {
  var out = [];
  for (var guard = 0; guard < 500; guard++) {
    var s = read();
    if (s === '') {
      s = input.next();
      if (s === null) { break; }
      s = { next: s };
    }
    out.push(s);
  }
  return out;
}

var scenarios = [];

for (var i = 0; i < 400; i++) {
  var sc = { kind: pick(['templatable', 'templatable', 'pattern']), input: text(), templating: pick(TEMPLATING),
    until: pick(UNTILS), untilAfter: rand() < 0.3, starting: pick(STARTS), matching: pick(MATCHES),
    exclude: rand() < 0.3 ? [pick(LANGS)] : [], disable: rand() < 0.2 ? [pick(LANGS)] : [] };
  var input = new InputScanner(sc.input);
  var p = sc.kind === 'pattern' ? new Pattern(input) : new TemplatablePattern(input).read_options(new Options({ templating: sc.templating }));
  if (sc.starting) { p = p.starting_with(new RegExp(sc.starting)); }
  if (sc.matching) { p = p.matching(new RegExp(sc.matching)); }
  if (sc.until) { p = sc.untilAfter ? p.until_after(new RegExp(sc.until)) : p.until(new RegExp(sc.until)); }
  if (sc.kind !== 'pattern') {
    sc.exclude.forEach(function(l) { p = p.exclude(l); });
    sc.disable.forEach(function(l) { p = p.disable(l); });
  }
  sc.out = drain(input, function() { return p.read(); });
  scenarios.push(sc);
}

for (i = 0; i < 150; i++) {
  sc = { kind: 'whitespace', input: text(), ws: pick([null, '\\u00A0\\u1680\\u180e\\u2000-\\u200a\\u202f\\u205f\\u3000\\ufeff']),
    nl: pick(['\\u2028\\u2029', '']) };
  input = new InputScanner(sc.input);
  var ws = new WhitespacePattern(input);
  if (sc.ws !== null) { ws = ws.matching(sc.ws, sc.nl); }
  sc.out = drain(input, function() {
    var r = ws.read();
    return r === '' ? '' : { ws: r, newlines: ws.newline_count, before: ws.whitespace_before_token };
  });
  scenarios.push(sc);
}

for (i = 0; i < 100; i++) {
  sc = { kind: 'tokenizer', input: text() };
  var stream = new Tokenizer(sc.input).tokenize();
  sc.out = [];
  while (stream.hasNext()) {
    var t = stream.next();
    sc.out.push([t.type, t.text, t.newlines, t.whitespace_before]);
  }
  scenarios.push(sc);
}

var COMMENTS = ['/* beautify ignore:start */', '/* beautify preserve:end */', '/*beautify ignore:start*/',
  '/* beautify a:b c:d */', '/* beautify  x:y */', '/* nothing */', '/* beautify ignore:start ok:1 */'];
for (i = 0; i < 40; i++) {
  sc = { kind: 'directives', text: pick(COMMENTS), input: text() + pick(['/* beautify ignore:end */', '/*beautify ignore:end*/', '']) + text() };
  var d = new Directives(/\/\*/, /\*\//);
  sc.directives = d.get_directives(sc.text);
  input = new InputScanner(sc.input);
  sc.ignored = d.readIgnored(input);
  scenarios.push(sc);
}

var TOKENS = ['a', 'bb', 'ccc', '{', '}', ';', 'word', 'x\n', 'line\nbreak', '\n', '', 'longer_token_here', '😀', '/', ','];
for (i = 0; i < 400; i++) {
  sc = { kind: 'output', options: {}, base: pick([null, '', '  ', '\t']), ops: [] };
  if (rand() < 0.7) { sc.options.indent_size = pick([0, 1, 2, 4, -1, -2, 3]); }
  if (rand() < 0.3) { sc.options.indent_char = pick([' ', '\t', '-', 'ab']); }
  if (rand() < 0.3) { sc.options.indent_with_tabs = rand() < 0.5; }
  if (rand() < 0.3) { sc.options.indent_level = pick([0, 1, 2]); }
  if (rand() < 0.6) { sc.options.wrap_line_length = pick([0, 5, 10, 20, -1]); }
  if (rand() < 0.3) { sc.options.indent_empty_lines = true; }
  if (rand() < 0.3) { sc.options.end_with_newline = true; }
  sc.eol = pick(['\n', '\r\n', '']);
  var o;
  try { o = new Output(new Options(sc.options), sc.base); } catch (e) { sc.err = e.message; scenarios.push(sc); continue; }
  for (var n = int(40); n >= 0; n--) {
    var op;
    switch (int(12)) {
      case 0: op = ['set_indent', int(4) - 1, int(3)]; break;
      case 1: op = ['newline', rand() < 0.3]; break;
      case 2: op = ['space']; break;
      case 3: op = ['nbsp']; break;
      case 4: op = ['trim', rand() < 0.5]; break;
      case 5: op = ['raw', int(3), pick(['', ' ', '\t']), pick(TOKENS)]; break;
      case 6: op = ['remove_indent', int(4)]; break;
      case 7: op = ['ensure_empty_line_above', pick(['/', 'a', '{']), pick([',', '}', ';'])]; break;
      case 8: op = ['wrap_point']; break;
      case 9: op = ['raw_mode', rand() < 0.3]; break;
      default: op = ['token', pick(TOKENS)];
    }
    sc.ops.push(op);
    var r = null;
    switch (op[0]) {
      case 'set_indent': r = o.set_indent(op[1], op[2]); break;
      case 'newline': r = o.add_new_line(op[1]); break;
      case 'space': o.space_before_token = true; break;
      case 'nbsp': o.non_breaking_space = true; break;
      case 'trim': o.trim(op[1]); break;
      case 'raw': o.add_raw_token({ newlines: op[1], whitespace_before: op[2], text: op[3] }); break;
      case 'remove_indent': o.remove_indent(op[1]); break;
      case 'ensure_empty_line_above': o.ensure_empty_line_above(op[1], op[2]); break;
      case 'wrap_point': o.set_wrap_point(); break;
      case 'raw_mode': o.raw = op[1]; break;
      case 'token': o.add_token(op[1]); r = o.previous_token_wrapped; break;
    }
    op.push(r, o.just_added_newline(), o.just_added_blankline(), o.get_line_number());
  }
  try { sc.code = o.get_code(sc.eol); } catch (e) { sc.err = e.message; }
  scenarios.push(sc);
}

process.stdout.write(JSON.stringify(scenarios).replace(/[\u007f-￿]/g, function(c) {
  return '\\u' + ('000' + c.charCodeAt(0).toString(16)).slice(-4);
}) + '\n');
