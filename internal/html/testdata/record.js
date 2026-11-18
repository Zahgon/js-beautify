// Records html_beautify results (embedded scripts and style sheets formatted by js_beautify and
// css_beautify) for the Go tests.
// Usage: node record.js JS_BEAUTIFY_DIR REQUESTS.jsonl... | gzip -9 > html.jsonl.gz
// REQUESTS rows are {"id","source","options"} (js-beautify's upstream test cases and rows from
// tools/agent-html/gen_html.py); each output row adds the original's "out" or "err". Output is
// re-encoded through UTF-8, so a lone surrogate becomes U+FFFD as when Node writes to stdout.
'use strict';
var fs = require('fs');
var path = require('path');
var htmlBeautify = require(path.resolve(process.argv[2], 'js')).html;

process.argv.slice(3).forEach(function(file) {
  fs.readFileSync(file, 'utf8').split('\n').filter(Boolean).forEach(function(line) {
    var r = JSON.parse(line), row = { id: r.id, source: r.source, options: r.options };
    try {
      var out = htmlBeautify(r.source, r.options === null ? undefined : r.options);
      row.out = Buffer.from(out, 'utf8').toString('utf8');
    } catch (e) {
      row.err = String(e && e.message !== undefined ? e.message : e);
    }
    process.stdout.write(JSON.stringify(row) + '\n');
  });
});
