// Records exec results of the regexes js-beautify's tokenizers build, on generated inputs.
// Usage (Node 24): node record_patterns.js JS_BEAUTIFY_DIR | gzip -9 > patterns.json.gz
'use strict';
var path = require('path');
var acorn = require(path.resolve(process.argv[2], 'js/src/javascript/acorn'));

var punct = ('>>>= ... >>= <<= === >>> !== **= &&= ??= ||= => ^= :: /= << <= == && -= >= >> != -- += ** || ?? ++ %= &= *= |= |> ' +
  '= ! ? > < : / ^ - + * & % ~ |').replace(/[-[\]{}()*+?.,\\^$|#]/g, '\\$&');
punct = ('\\?\\.(?!\\d) ' + punct).replace(/ /g, '|');

var patterns = [
  acorn.identifier.source, acorn.identifierMatch.source, acorn.lineBreak.source, acorn.newline.source,
  /0[xX][0123456789abcdefABCDEF_]*n?|0[oO][01234567_]*n?|0[bB][01_]*n?|\d[\d_]*n|(?:\.\d[\d_]*|\d[\d_]*\.?[\d_]*)(?:[eE][+-]?[\d_]+)?/.source,
  punct,
  '^(?:continue|try|throw|return|var|let|const|if|switch|case|default|for|while|break|function|import|export|do|in|of|else|get|set|new|catch|finally|typeof|yield|async|await|from|as|class|extends)$',
  /[\s\S]*?<(\/?)([-a-zA-Z:0-9_.]+|{[^}]+?}|!\[CDATA\[[^\]]*?\]\]|)(\s*{[^}]+?}|\s+[-a-zA-Z:0-9_.]+|\s+[-a-zA-Z:0-9_.]+\s*=\s*('[^']*'|"[^"]*"|{([^{}]|{[^}]+?})+?}))*\s*(\/?)\s*>/.source,
  /([\s]|[^\\]|\\\\)+/.source, /x([0-9A-Fa-f]{2})/.source, /u\{([0-9A-Fa-f]+)\}/.source,
  /[\t \n\r\u00a0\u1680\u180e\u2000-\u200a\u202f\u205f\u3000\ufeff\u2028\u2029]+/.source,
  '\\r\\n|[\\n\\r\\u2028\\u2029]',
  /(?:<\?(?:[= ]|php)|{{|{{|<%[^%]|{%|{{|{#|{(?=[^}{\s\n])|[\n\r\t <])/.source,
  /\@[a-zA-Z]+[^({]*[({]/.source, /^(<!--[^\n]*|<!\[CDATA\[)(\n?)([ \t\n]*)([\s\S]*)(-->|]]>)$/.source,
  /^{{~?(?:[\^]|#\*?)?([^\s}]+)/.source, /module|importmap|((text|application|dojo)\/(x-)?(javascript|ecmascript|jscript|livescript|(ld\+)?json|method|aspect))/.source,
  '</sc+ript[\\n\\r\\t ]*?>', /[\n\r\t =>]|\/>/.source, /^[\t ]*/.source, /\/\* beautify( \w+[:]\w+)+ \*\//.source
];

var seed = 99;
function rand() {
  seed |= 0; seed = seed + 0x6D2B79F5 | 0;
  var t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
}
var PIECES = ['a', 'Z', '_', '$', '@', '#', '0', '7', '9', '.', 'x', 'e', 'E', '+', '-', 'n', '0x1F', '0b10', '1_000', '.5e-3',
  ' ', '\t', '\n', '\r\n', '\u2028', '\u00a0', '\ufeff', '<', '>', '/', '</', '/>', '=', '"', "'", '{', '}', '[', ']', '(', ')',
  '\\', '\\u0041', '\\u{1F600}', '\\x41', 'div', '<a b="c">', '<!--', '-->', '<![CDATA[x]]>', '{{', '}}', '{{#if x}}', '{%',
  '<?php', '?>', '<%', '%>', '?.', '?.5', '>>>=', '=>', '===', '**', '|>', 'return', 'var', 'class', '\u00e9', '\u0661',
  '\ud83d\ude00', 'text/javascript', 'application/ld+json', '</SCRIPT >', '</sccript>', '@if (', '/* beautify a:b */'];
function text() { var s = ''; for (var i = Math.floor(rand() * 10); i >= 0; i--) { s += PIECES[Math.floor(rand() * PIECES.length)]; } return s; }

var cases = [];
patterns.forEach(function(source) {
  ['', 'g', 'y', 'i', 'ig'].forEach(function(flags) {
    for (var n = 0; n < 30; n++) {
      var input = text(), re = new RegExp(source, flags);
      re.lastIndex = Math.floor(rand() * (input.length + 2));
      var c = { pattern: source, flags: flags, input: input, lastIndex: re.lastIndex };
      var m = re.exec(input);
      c.index = m ? m.index : -1;
      c.groups = m ? Array.prototype.map.call(m, function(g) { return g === undefined ? null : g; }) : null;
      c.canonicalFlags = re.flags;
      cases.push(c);
    }
  });
});
process.stdout.write(JSON.stringify({ regex: cases }).replace(/[\u007f-\uffff]/g, function(c) {
  return '\\u' + ('000' + c.charCodeAt(0).toString(16)).slice(-4);
}) + '\n');
