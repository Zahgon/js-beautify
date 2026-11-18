// Records minimatch 10.2.5 results for generated patterns and paths:
//   node record.js /path/to/minimatch/dist/commonjs/index.js /path/to/brace-expansion | gzip > cases.jsonl.gz
// Each line is {"pattern", "opts", "magic", "braces", "matches": [[path, bool], ...]} or
// {"pattern", "opts", "error"} when the pattern does not compile.
'use strict';
const { Minimatch } = require(process.argv[2]);
const { expand } = require(process.argv[3]);
let seed = 0x9e3779b9;
// mulberry32
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const atoms = ['a', 'b', 'c', '.', '/', '*', '**', '?', '[', ']', '!', '^', '-', '{', '}', ',', '(', ')', '|', '+', '@', '\\',
  'js', '.js', 'x', '..', '[a-c]', '[!a]', '{a,b}', '{1..3}', '+(a|b)', '!(a)', '*(x)', '?(a|b)', '@(js|css)', '[[:alpha:]]',
  '[[:digit:]]', 'é', ' ', '#', '$', '~', '{,a}', '{a}', '{a..c}', '0'];
const segs = ['a', 'b', 'c', 'ab', 'abc', '.a', '..', '.', 'a.js', 'b.css', '.js', 'x', 'aa', 'ba', '1', '2', 'a.b', '', 'js', 'é', 'a b', '!', '#', 'x.js', 'a(b', '[a]', 'a{b}', 'a,b'];
function genPattern() {
  let s = '';
  const n = 1 + rnd(7);
  for (let i = 0; i < n; i++) s += atoms[rnd(atoms.length)];
  if (rnd(4) === 0) s = '/tmp/run/' + s;
  return s;
}
function genPath() {
  const n = 1 + rnd(4);
  const parts = [];
  for (let i = 0; i < n; i++) parts.push(segs[rnd(segs.length)]);
  let p = parts.join('/');
  if (rnd(4) === 0) p = '/tmp/run/' + p;
  return p;
}
const optSets = [
  {},
  { matchBase: true, dot: true },
  { dot: true, nobrace: false, noext: false, optimizationLevel: 2, nocomment: true, nonegate: true, braceExpandMax: 10000 },
];
const fixed = ['*', '*.js', '**/*.js', '/tmp/run/**/*', '/tmp/run/{*,**/**/**}/*.js', '/tmp/run/{*,**/**/**}/*.{js,css}',
  'a/**/b', '**', '?.js', '??', '[ab].js', '[!a]*', '+(a|b).js', '!(a).js', '!*.js', '#a', '', '\\*', 'a\\', '{a,b}{c,d}',
  '{1..10..3}', '{a..e}', '{z..u..2}', '{01..3}', '{-3..3}', 'a{,b}', '{}a', 'x{{a,b}}y', '{a},b}', '[[:alpha:]]x',
  '/tmp/run/**/../a', '**/node_modules/**', '**/.git/**', 'a/./b', './*.js', '../*.js', 'a/b/..', '[]', '[a-]', '[-a]',
  '[z-a]', '[\\]]', '*(a|b|)', '!()', '@()', '+(a|*(b|c))', '!(a|!(b))'];
const out = [];
function rec(pattern) {
  for (const opts of optSets) {
    let mm;
    try { mm = new Minimatch(pattern, opts); } catch (e) { out.push(JSON.stringify({ pattern, opts, error: e.message })); continue; }
    const matches = [];
    for (let i = 0; i < 12; i++) { const p = genPath(); matches.push([p, mm.match(p)]); }
    for (const p of ['a.js', 'x.js', '/tmp/run/a.js', '/tmp/run/sub/b.css', 'a/b', '.a', 'a', 'ab', '/tmp/run/x/node_modules/y']) matches.push([p, mm.match(p)]);
    out.push(JSON.stringify({ pattern, opts, magic: mm.hasMagic(), braces: opts.nobrace === undefined ? expand(pattern) : undefined, matches }));
  }
}
fixed.forEach(rec);
for (let i = 0; i < 1500; i++) rec(genPattern());
process.stdout.write(out.join('\n') + '\n');
