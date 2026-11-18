// Records how @one-ini/wasm tokenizes generated INI files:
//   node record_ini.js /path/to/@one-ini/wasm | gzip > ini.jsonl.gz
// Each line is {"b64": file bytes, "tokens": ["S:name", "K:key", "V:value", ...]} (comments left
// out) or {"b64", "error": true} when the file does not parse.
'use strict';
const w = require(process.argv[2]);
let seed = 0x9e3779b9;
// mulberry32
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const atoms = ['[', ']', '=', '#', ';', ' ', '\t', '\n', '\r\n', '\r', 'a', 'b', 'key', 'value', 'root', 'true', '*', '.js',
  '{', '}', ',', '﻿', ' ', '\u000b', '\u0000', 'é', '"', "'", '\\', '[*]', '[*.js]', 'indent_size = 2', '; c',
  '# c', ' = ', '==', 'x=y', ' ', '　'];
function gen() {
  let s = '';
  const n = 1 + rnd(16);
  for (let i = 0; i < n; i++) s += atoms[rnd(atoms.length)];
  return Buffer.from(s, 'utf8');
}
const out = [];
function rec(buf) {
  let tokens;
  try {
    const a = w.parse_to_uint32array(buf);
    tokens = [];
    for (let i = 0; i < a.length; i += 3) {
      const text = buf.toString('utf8', a[i + 1], a[i + 2]);
      if (a[i] === w.TokenTypes.Section) tokens.push('S:' + text);
      else if (a[i] === w.TokenTypes.Key) tokens.push('K:' + text);
      else if (a[i] === w.TokenTypes.Value) tokens.push('V:' + text);
    }
  } catch (e) {
    out.push(JSON.stringify({ b64: buf.toString('base64'), error: true }));
    return;
  }
  out.push(JSON.stringify({ b64: buf.toString('base64'), tokens }));
}
for (const s of ['root = true\n[*]\nindent_style = tab\n', ' ﻿a=b', '﻿﻿ a=b', '[a][b]', 'a=b\nc']) rec(Buffer.from(s));
rec(Buffer.from([0x61, 0x3d, 0xff]));
rec(Buffer.from([0x61, 0x3d, 0xe2, 0x82]));
for (let i = 0; i < 6000; i++) rec(gen());
process.stdout.write(out.join('\n') + '\n');
