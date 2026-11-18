// Records JSON.parse results for generated inputs: node record.js | gzip > json.jsonl.gz
// Each line is {"src": text, "ok": canonical value} or {"src": text, "err": message}.
'use strict';
let seed = 0x9e3779b9;
// mulberry32
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const atoms = ['{', '}', '[', ']', ':', ',', '"', '\\', 'a', 'b', '1', '0', '-', '.', 'e', 'E', '+', ' ', '\n', '\r', '\t',
  'true', 'false', 'null', 'tru', 'nul', 'fals', '"x"', '"k":', '\\u', '\\u12', '\\x', '\\n', 'é', '😀', '﻿',
  '\u0001', '/', '*', '#', '12', '1e5', '-0', '0x', 'NaN', ' ', ' ', "'", 'x'];
function gen() {
  const n = 1 + rnd(14);
  let s = '';
  for (let i = 0; i < n; i++) s += atoms[rnd(atoms.length)];
  return s;
}
function valid() {
  const d = rnd(3);
  const v = d === 0 ? { indent_size: rnd(9), x: [1, 'a', null, true] } : d === 1 ? [rnd(100) / 7, 'é\n', {}] : 'str';
  let s = JSON.stringify(v, null, rnd(2) ? 2 : 0);
  const cut = rnd(s.length + 1);
  return rnd(2) ? s : s.slice(0, cut) + atoms[rnd(atoms.length)] + s.slice(cut);
}
const fixed = ['', ' ', 'NaN', 'undefined', 'Infinity', '[object Object]', '01', '-01', '1.', '1e', '1e+', '-', '"\\u12"',
  '{"a":1,}', '[1,]', '{"aaaaaaaaaaaaaaaaaaa": bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb}', 'abcdefghijklmnopqrstuvwxyz',
  '{\n  "a": 1,\n  b: 2\n}', '\r\n\r\n x', '1e400', '-1e400', '1e-400', '-0', '123456789012', '{"__proto__": 1}',
  '{"a":1,"a":2,"1":3,"0":4}', '"\\ud800"'];
const out = [];
const seen = new Set();
function add(src) {
  if (seen.has(src)) return;
  seen.add(src);
  try { out.push(JSON.stringify({ src, ok: JSON.stringify(JSON.parse(src)) })); }
  catch (e) { out.push(JSON.stringify({ src, err: e.message })); }
}
fixed.forEach(add);
for (let i = 0; i < 6000; i++) add(gen());
for (let i = 0; i < 3000; i++) add(valid());
process.stdout.write(out.join('\n') + '\n');
