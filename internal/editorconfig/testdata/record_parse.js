// Records editorconfig.parseSync results for generated .editorconfig trees:
//   node record_parse.js /path/to/editorconfig | gzip > parse.jsonl.gz
// Each line is {"files": {relative path: content}, "queries": [[relative path, JSON of props
// or null when parseSync throws]]}. The top .editorconfig always has root = true.
'use strict';
const fs = require('fs');
const path = require('path');
const editorconfig = require(process.argv[2]);
let seed = 0x9e3779b9;
// mulberry32
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const pick = (a) => a[rnd(a.length)];
const globs = ['*', '*.js', '*.css', '*.{js,css}', '{a,b}.js', 'a.js', 'sub/*.js', '/a.js', '/sub/**', '**.js', '**/b.js',
  'sub/**/c.html', '*.[ch]tml', '[ab].js', '[!a]*.js', '?.js', '*.{1..3}', 'x{1..3}.js', '*.JS', 'sub', 'sub/', '**',
  '*.min.js', '.*', 'a\\*b.js', '{*.js,*.css}', '*.{js}', '[*.js]', '*.+(js|css)', '!(a).js', 'a,b.js'];
const keys = ['indent_style', 'indent_size', 'tab_width', 'max_line_length', 'insert_final_newline', 'end_of_line',
  'charset', 'Indent_Size', 'INDENT_STYLE', 'custom', 'root', 'trim_trailing_whitespace', '__proto__', 'constructor'];
const values = ['tab', 'space', 'TAB', '2', '4', '0', '1e1', 'off', 'OFF', 'true', 'false', 'TRUE', 'lf', 'crlf', 'CR', 'cr',
  'unset', 'null', '[1,2]', '{"a":1}', '"q"', '-1', '8', 'x', '', '07', '2.5'];
function genConfig(top) {
  let s = top ? 'root = true\n' : (rnd(3) === 0 ? 'root = ' + pick(['true', 'false', 'TRUE', 'no']) + '\n' : '');
  const n = rnd(4);
  for (let i = 0; i < n; i++) {
    s += '[' + pick(globs) + ']\n';
    const m = 1 + rnd(4);
    for (let j = 0; j < m; j++) s += pick(keys) + ' = ' + pick(values) + '\n';
    if (rnd(8) === 0) s += pick(['# comment\n', 'bad line\n', '\n', '; x = y\n']);
  }
  return s;
}
const base = '/tmp/ecrec';
const out = [];
for (let c = 0; c < 400; c++) {
  const dir = path.join(base, String(c));
  fs.rmSync(dir, { recursive: true, force: true });
  const files = {};
  files['.editorconfig'] = genConfig(true);
  if (rnd(2)) files['sub/.editorconfig'] = genConfig(false);
  if (rnd(3) === 0) files['sub/deep/.editorconfig'] = genConfig(false);
  for (const [rel, content] of Object.entries(files)) {
    fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
    fs.writeFileSync(path.join(dir, rel), content);
  }
  const queries = [];
  for (const rel of ['a.js', 'b.css', 'x.html', 'sub/a.js', 'sub/b.js', 'sub/deep/c.html', 'sub/deep/x2.js', '.hidden',
    'a.min.js', 'stdin.js', 'sub/a.JS', 'a,b.js', 'x1.js', 'f.3']) {
    let r;
    try { r = JSON.stringify(editorconfig.parseSync(path.join(dir, rel))); } catch (e) { r = null; }
    queries.push([rel, r]);
  }
  out.push(JSON.stringify({ files, queries }));
}
process.stdout.write(out.join('\n') + '\n');
