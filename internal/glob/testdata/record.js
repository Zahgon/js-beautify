// Records glob 13 results (as js-beautify calls it) on generated trees:
//   node record.js /path/to/node_modules/glob | gzip > glob.jsonl.gz
// Each line is {"tree": [paths, directories ending in "/"], "results": [[pattern, magic, matches
// relative to the tree's directory, in order]]}.
'use strict';
const fs = require('fs');
const path = require('path');
const { globSync, hasMagic } = require(process.argv[2]);
let seed = 0x51ed;
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const names = ['a.js', 'b.js', 'c.css', 'A.js', '.h.js', 'x.min.js', 'd/', 'd/e.js', 'd/f/g.js', 'node_modules/n.js',
  '.git/x.js', 'sub/node_modules/m.js', 'sub/a.js', 'sub/.dot/z.js', 'z.html', 'a b.js', 'é.js', '-', '[x].js', 'dir/'];
const patterns = ['*.js', '**/*.js', '*', '**', 'd/**', 'd/*', '?.js', '[ab].js', '{a,b}.js', '*.{js,css}', '!(a).js',
  './*.js', '../TREE/*.js', 'TREEABS/*.js', 'sub/**/*.js', '**/node_modules/**', '.*', '**/.*', 'd/**/g.js', 'nomatch*',
  '*/', '**/', 'd/../*.js', '+(a|b).js', '*.JS', '[[:alpha:]].js', 'a\\ b.js', 'a b*', 'é*', '[x]*', '\\[x\\]*', 'd/f/*',
  '**/f/**', '*/*/*.js', '**/../*.css'];
const base = '/tmp/globrec';
const out = [];
for (let c = 0; c < 150; c++) {
  const dir = path.join(base, String(c));
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
  const tree = names.filter(() => rnd(2));
  for (const n of tree) {
    const p = path.join(dir, n);
    if (n.endsWith('/')) fs.mkdirSync(p, { recursive: true });
    else { fs.mkdirSync(path.dirname(p), { recursive: true }); fs.writeFileSync(p, 'x'); }
  }
  process.chdir(dir);
  const results = [];
  for (let k = 0; k < 8; k++) {
    let p = patterns[rnd(patterns.length)].replace('TREEABS', dir).replace('TREE', String(c));
    const magic = hasMagic(p);
    const found = globSync(p, { absolute: true, ignore: ['**/node_modules/**', '**/.git/**'] });
    results.push([p.replace(dir, 'TREEABS'), magic, found.map((f) => path.relative(dir, f))]);
  }
  out.push(JSON.stringify({ tree, results }));
}
process.stdout.write(out.join('\n') + '\n');
