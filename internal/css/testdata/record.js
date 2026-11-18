// Records css_beautify results for the Go tests.
// Usage: node record.js JS_BEAUTIFY_DIR REQUESTS.jsonl... | gzip -9 > css.jsonl.gz
// REQUESTS rows are {"id","source","options"} (js-beautify's upstream test cases and rows from
// tools/agent-core/gen_css.py); each output row adds the original's "out" or "err".
'use strict';
var fs = require('fs');
var path = require('path');
var cssBeautify = require(path.resolve(process.argv[2], 'js')).css;

process.argv.slice(3).forEach(function(file) {
  fs.readFileSync(file, 'utf8').split('\n').filter(Boolean).forEach(function(line) {
    var r = JSON.parse(line), row = { id: r.id, source: r.source, options: r.options };
    try {
      row.out = cssBeautify(r.source, r.options === null ? undefined : r.options);
    } catch (e) {
      row.err = String(e && e.message !== undefined ? e.message : e);
    }
    process.stdout.write(JSON.stringify(row) + '\n');
  });
});
