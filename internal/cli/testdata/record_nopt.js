// Records how js-beautify's command line parses generated argument lists with nopt:
//   cd /tmp/jsb/run && HOME=/tmp/jsb/home node record_nopt.js /path/to/js/src/cli.js | gzip > nopt.jsonl.gz
// The option tables are taken from cli.js itself. Each line is {"args": [...], "parsed": JSON of
// nopt's result} or {"args", "error": String(exception)}.
'use strict';
const fs = require('fs');
const path = require('path');
const cliPath = process.argv[2];
const src = fs.readFileSync(cliPath, 'utf8');
const tables = src.slice(src.indexOf('nopt.invalidHandler'), src.indexOf('function verifyExists'));
const helpers = src.slice(src.indexOf('function dasherizeFlag'), src.indexOf('function getOutputType'));
const req = (m) => require(require.resolve(m, { paths: [path.dirname(cliPath)] }));
const { knownOpts, shortHands, nopt } = new Function('require', `
  var nopt = require('nopt'), glob = require('glob');
  ${helpers}
  ${tables}
  return { knownOpts, shortHands, nopt };`)(req);
let seed = 0x1234567;
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return (((t ^ (t >>> 14)) >>> 0) / 4294967296 * n) | 0;
}
const words = ['-s', '2', '-c', ' ', '\t', '-t', 'true', 'false', '--no-indent-size', '--indent-size=3', '--indent_size', 'abc',
  '-b', 'expand', 'collapse,preserve-inline', 'bogus', '--brace-style=', '-A', 'force', 'auto', '--no-wrap-attributes',
  '--templating', 'erb', '-f', '-', 'a.js', '*.js', '{a,b}.js', '--files=x', '-o', 'out.js', '~/x', '--outfile', '-r', '-q',
  '--type', 'css', 'xml', '--css', '--js', '--html', '--good-stuff', '--', '---', '--foo', 'bar', '--foo=1', '--no-foo',
  '--constructor', '--no-constructor', '--toString', '--__proto__', '--no-', '--=x', '-=x', '-he', '--he', '--hel', '-sp',
  '--sp', '-LN', '--no-no-quiet', '--NO-quiet', '--no-s', '--ver', '-v', '-h', '--config', 'c.json', '--conf', '--edit',
  '--editorconfig', 'null', '1e1', '0x10', ' 5 ', '-1', '--inline', '-U', 'p', '--indent-inner-html', 'yes', '-I', '-M', '',
  '--wrap-attributes-min-attrs', '--max-char', 'Infinity', '-e', '\\n', '--eol=\\r\\n', '--operator-position', 'after-newline',
  '-O', '--indent-scripts', 'keep', '-S', 'x', '--no-templating', '--no-type', 'js', '--quiet=false', '--replace=0'];
const out = [];
const seen = new Set();
function rec(args) {
  const key = JSON.stringify(args);
  if (seen.has(key)) return;
  seen.add(key);
  try {
    out.push(JSON.stringify({ args, parsed: JSON.stringify(nopt(knownOpts, shortHands, args.slice(), 0)) }));
  } catch (e) {
    out.push(JSON.stringify({ args, error: String(e) }));
  }
}
for (const w of words) rec([w]);
for (let i = 0; i < 6000; i++) {
  const n = 1 + rnd(5);
  const args = [];
  for (let j = 0; j < n; j++) args.push(words[rnd(words.length)]);
  rec(args);
}
process.stdout.write(out.join('\n') + '\n');
