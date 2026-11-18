// Records Node path.posix results: node record.js > path.json
const path = require('path');
const ins = ['', '.', '..', '/', '//', '///a', 'a', 'a/', 'a//b', 'a/./b', 'a/../b', '../a', '../../a/b/', '/a/b/c', '/a/b/../../..', 'a/b/..', '.a', 'a.b.c', '..a', '/x/y.tar.gz', 'dir/', '/..', 'a/..', '~/x'];
const res = [];
for (const p of ins) res.push(['dirname', p, path.dirname(p)], ['basename', p, path.basename(p)], ['normalize', p, path.normalize(p)], ['resolve', p, path.resolve('/tmp/jsb/run', p)], ['parse', p, JSON.stringify(path.parse(p))], ['join', p, path.join(p, 'x')], ['relative', p, path.relative('/tmp/jsb/run', path.resolve('/tmp/jsb/run', p))]);
console.log(JSON.stringify(res));
