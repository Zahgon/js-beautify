// Records how V8 evaluates the cases below, for the jsrt tests.
// Usage (Node 24): node record.js > jsrt.json
'use strict';

var regexCases = [
  // [pattern, flags, input, lastIndex]
  ['a', '', 'banana', 0], ['an', 'g', 'banana', 2], ['an', 'y', 'banana', 1], ['an', 'y', 'banana', 2],
  ['(a|b)*c', '', 'abac', 0], ['(z)((a+)?(b+)?(c))*', '', 'zaacbbbcac', 0], ['(a*)*b', '', 'aaab', 0],
  ['(a*)+?b', '', 'aab', 0], ['(?:a|ab)(?:c|bcd)(d*)', '', 'abcd', 0], ['a[a-z]{2,4}', '', 'abcdefghi', 0],
  ['a[a-z]{2,4}?', '', 'abcdefghi', 0], ['(aa|aabaac|ba|b|c)*', '', 'aabaac', 0], ['^(a+)\\1*,\\1+$', '', 'aaaaaaaaaa,aaaaaaaaaaaaaaa', 0],
  ['(?=(a+))', '', 'baaabac', 0], ['(?=(a+))a*b\\1', '', 'baaabac', 0], ['(.*?)a(?!(a+)b\\2c)\\2(.*)', '', 'baaabaac', 0],
  ['\\bfoo\\b', '', 'a foo_ foo.', 0], ['\\Boo', 'g', 'foo oo', 3], ['[\\s\\S]*?x', '', 'ab\ncx', 0],
  ['.+', '', 'ab cd', 0], ['.+', 's', 'ab cd', 0], ['^b', 'm', 'a\nb', 0], ['a$', 'm', 'a\nb', 0], ['a$', '', 'a\nb', 0],
  ['\\s+', 'g', 'x ﻿ 　᠎y', 1], ['\\w+', '', 'été_12', 0], ['\\d+', '', '١٢ 42', 0],
  ['k', 'i', 'K', 0], ['[a-z]+', 'i', 'ABCK', 0], ['ſ', 'i', 'S', 0], ['ß', 'i', 'ẞ', 0],
  ['σ', 'i', 'Σς', 0], ['[^a]', 'i', 'A', 0], ['\\w', 'i', 'ſ', 0], ['(a)\\1', 'i', 'aA', 0],
  ['\\101\\0\\08\\8\\9\\400', '', 'A\u0000\u00008 89  0', 0], ['\\1(a)', '', 'aa', 0], ['\\2(a)', '', '\u0002a', 0],
  ['\\cA\\c1\\c', '', '\u0001\\c1\\c', 0], ['[\\cA\\c1\\c_]', 'g', '\u0001\u0011\u001f\\c', 0], ['[\\b]', '', 'a\bb', 0],
  ['[a-\\d]+', '', 'a-5z', 0], ['[\\d-z]+', '', '5-z', 0], ['[-a]+', '', '-a', 0], ['[a-]+', '', 'a-', 0],
  ['a{', '', 'a{', 0], ['a{1', '', 'a{1', 0], ['a{,5}', '', 'a{,5}', 0], ['x{2}', '', 'xxx', 0], ['}]', '', '}]', 0],
  ['\\x4g\\u12', '', 'x4gu12', 0], ['\\u0041\\x41', '', 'AA', 0], ['\\u{41}', '', 'uuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuu', 0],
  ['\\k', '', 'k', 0], ['(?<y>\\d{4})-\\k<y>', '', '2020-2020', 0], ['(?<a>.)(?<b>.)', '', 'xy', 0],
  ['(?i:a)b', '', 'Ab AB', 0], ['(?-i:a)b', 'i', 'AB ab aB', 0], ['(?<=\\$)\\d+', '', 'cost $42', 0], ['(?<!\\$)\\b\\d+', '', '$42 17', 0],
  ['(?=a)*a', '', 'a', 0], ['()', 'g', 'ab', 0], ['a|ab', '', 'abc', 0], ['((a)|b)+', '', 'ab', 0],
  ['\\/\\*(?:[\\s\\S]*?)((?:\\*\\/)|$)', 'g', '/* abc', 0], ['(?:\\s|\\n)+', 'g', ' \n\t x', 0],
  ['[😀]', '', '\ude00', 0], ['.', 'g', '😀', 1], ['[^\\n]+', '', '😀x', 0],
  // errors
  ['(', 'g', '', 0], ['a**', 'ig', '', 0], ['[z-a]', '', '', 0], ['a{2,1}', '', '', 0], ['\\', '', '', 0],
  ['(?<=a)*', '', '', 0], ['(?x)', '', '', 0], ['a)', '', '', 0], ['*', '', '', 0], ['{1}', '', '', 0],
  ['(?<a>x)(?<a>y)', '', '', 0], ['(?<1a>x)', '', '', 0], ['\\k<b>(?<a>x)', '', '', 0], ['[', '', '', 0],
  ['^*', '', '', 0], ['\\b+', '', '', 0], ['a', 'gg', '', 0], ['(?ii:a)', '', '', 0], ['(?:', '', '', 0],
  ['a', 'yg', 'a', 0], ['', '', 'x', 0]
];

var replaceCases = [
  // [pattern, flags, input, replacement]
  ['a', 'g', 'banana', '[$&]'], ['(a)(n)', 'g', 'banana', '$2$1'], ['(a)', '', 'banana', '$`|$\'|$$|$0|$01|$10|$2'],
  ['x*', 'g', 'abc', '-'], ['(?<n>a)', 'g', 'aba', '<$<n>|$<m>|$<n'], ['a', 'g', 'aaa', '$<n>'], ['^\\s+', 'g', '   x ', ''],
  ['\\r\\n|[\\r\\n]', 'g', 'a\r\nb\rc\n', '\n'], ['[-\\/\\\\^$*+?.()|[\\]{}]', 'g', 'a.b*c/d', '\\$&'], ['(b)?a', 'g', 'aba', '[$1]']
];

var splitCases = [
  // [pattern, flags, input]
  ['[^a-zA-Z0-9_\\/\\-]+', '', 'collapse,preserve-inline'], ['[^a-zA-Z0-9_\\/\\-]+', '', ' expand '], ['[^a-zA-Z0-9_\\/\\-]+', '', ''],
  ['(,)', '', 'a,b,,c'], ['(x)?,', '', 'a,b'], ['', '', 'abc'], ['\\s*', '', 'a b'], ['a', '', 'a'], ['b', '', '']
];

var numbers = [0, -0, 1, -1, 2.7, 1e21, 1e-7, 1.5e-7, 123e-20, 0.1, 1 / 3, 1e20, 123456789012345680000, 5e-324,
  1.7976931348623157e308, 4294967296.5, -2.5e-10, 100, 1e6, Infinity, -Infinity, NaN, 0.000001, 1.23e+25];

var parseInts = ['42', '  -17px', '+3', '0x1A', '1e21', '', 'abc', ' ﻿ 7', '᠎8', '-0', '99999999999999999999999',
  '3.9', ' \n\t12', '1,2', 'null', 'Infinity', '-', '0b11', '  5'];

var lowers = ['ABC', 'İ', 'K', 'Σ', 'AΣ', 'AΣ B', 'AΣB', 'ÉCOLE', '𐐀', 'A\ud800B', 'URL'];

function show(re) {
  try { return { re: new RegExp(re[0], re[1]) }; } catch (e) { return { err: e.message }; }
}

var out = { regex: [], replace: [], split: [], numbers: [], parseInt: [], lower: [] };
regexCases.forEach(function(c) {
  var r = show(c), row = { pattern: c[0], flags: c[1], input: c[2], lastIndex: c[3] };
  if (r.err) { row.err = r.err; } else {
    r.re.lastIndex = c[3];
    var m = r.re.exec(c[2]);
    if (m) {
      row.index = m.index;
      row.groups = Array.prototype.map.call(m, function(g) { return g === undefined ? null : g; });
    } else { row.groups = null; }
    row.canonicalFlags = r.re.flags;
  }
  out.regex.push(row);
});
replaceCases.forEach(function(c) {
  out.replace.push({ pattern: c[0], flags: c[1], input: c[2], replacement: c[3], out: c[2].replace(new RegExp(c[0], c[1]), c[3]) });
});
splitCases.forEach(function(c) {
  out.split.push({ pattern: c[0], flags: c[1], input: c[2],
    out: c[2].split(new RegExp(c[0], c[1])).map(function(s) { return s === undefined ? '' : s; }) });
});
numbers.forEach(function(n) { out.numbers.push({ value: isFinite(n) ? n : String(n), negZero: Object.is(n, -0), out: String(n) }); });
parseInts.forEach(function(s) { var v = parseInt(s, 10); out.parseInt.push({ input: s, out: isNaN(v) ? null : v, negZero: Object.is(v, -0) }); });
lowers.forEach(function(s) { out.lower.push({ input: s, out: s.toLowerCase() }); });
// Strings are written with \u escapes so that lone surrogates survive.
process.stdout.write(JSON.stringify(out, null, 1).replace(/[\u007f-￿]/g, function(c) {
  return '\\u' + ('000' + c.charCodeAt(0).toString(16)).slice(-4);
}) + '\n');
