// The JS SDK is checked against the same fixtures as the Go reference.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

import {
  canonical, canonicalIssues, extract, inlineHTML, inlineText, parse, parseChart, renderHTML, validate,
} from './xtxt.js';

const here = path.dirname(fileURLToPath(import.meta.url));
const casesDir = path.join(here, '..', '..', 'conformance', 'cases');
const cases = fs.readdirSync(casesDir).filter((f) => f.endsWith('.xtxt')).sort();
assert.ok(cases.length > 0, 'conformance cases not found');

for (const name of cases) {
  test(`conformance: ${name}`, () => {
    const src = fs.readFileSync(path.join(casesDir, name), 'utf8');
    const expected = JSON.parse(
      fs.readFileSync(path.join(casesDir, name.replace(/\.xtxt$/, '.json')), 'utf8'));
    const res = parse(src);
    assert.deepEqual(canonical(res.doc), expected.ast);
    assert.deepEqual(canonicalIssues([...res.issues, ...validate(res.doc)]), expected.issues);
  });
}

test('inline formatting', () => {
  assert.equal(inlineHTML('**b** and `a<b`'), '<strong>b</strong> and <code>a&lt;b</code>');
  assert.equal(inlineText('**b** and [x](y)'), 'b and x');
});

test('extraction finds tasks from both spellings', () => {
  const { doc } = parse('# T\n\n- [x] done\n\n@task\nTitle: Ship it\nStatus: Done\n@endtask\n');
  const got = extract(doc);
  assert.deepEqual(got.tasks.map((t) => t.title), ['done', 'Ship it']);
  assert.ok(got.tasks.every((t) => t.done));
  assert.deepEqual(got.outline, [{ level: 1, text: 'T', line: 1 }]);
});

test('charts render as script-free SVG with a table view', () => {
  const { doc } = parse('@chart(type="bar", title="Monthly")\nJan 20\nFeb 35\n@endchart\n');
  const html = renderHTML(doc);
  for (const want of ['<svg', 'var(--chart-1)', '<title>Jan: 20</title>', 'chart-data', '<td>Feb</td>']) {
    assert.ok(html.includes(want), `missing ${want}`);
  }
  assert.ok(!html.includes('<script'), 'chart output must stay script-free');
});

test('charts agree with the Go renderer on parsing', () => {
  const { doc } = parse('@chart\nX | A | B | C | D\n1 | 1 | 2 | 3 | 4\n@endchart\n');
  const c = parseChart(doc.nodes[0]);
  assert.equal(c.series.length, 4, 'excess series fold into Other');
  assert.equal(c.series[3].name, 'Other');
  assert.equal(c.series[3].values[0], 4);
  assert.ok(c.warnings.length > 0, 'folding must be reported, not silent');
});

test('multi-word chart labels survive', () => {
  const { doc } = parse('@chart\nNew York 20\nSan Francisco 35\n@endchart\n');
  assert.deepEqual(parseChart(doc.nodes[0]).labels, ['New York', 'San Francisco']);
});

test('record blocks render, and field order survives extraction', () => {
  // Regression: Fields extends Array, so a method named `map` shadowed
  // Array.prototype.map and broke both of these at once.
  const { doc } = parse('@decision\nTitle: Use one pass\nWhy: simplicity\n@enddecision\n');
  const html = renderHTML(doc);
  assert.ok(html.includes('data-type="decision"'), html);
  assert.ok(html.includes('<dt>Title</dt><dd>Use one pass</dd>'), html);

  const block = extract(doc).blocks[0];
  assert.deepEqual(block.order, ['Title', 'Why']);
  assert.deepEqual(block.fields, { title: 'Use one pass', why: 'simplicity' });
});

test('every example on the demo page renders without throwing', async () => {
  const html = await import('node:fs').then((fs) =>
    fs.readFileSync(new URL('../../docs/index.html', import.meta.url), 'utf8'));
  const sources = [...html.matchAll(/^  (\w+): `([\s\S]*?)`,$/gm)].map((m) => m[2]);
  assert.ok(sources.length >= 4, `expected the demo examples, found ${sources.length}`);
  for (const src of sources) {
    const res = parse(src.replace(/\\`/g, '`').replace(/\\\\/g, '\\'));
    assert.doesNotThrow(() => renderHTML(res.doc));
    assert.doesNotThrow(() => extract(res.doc));
    assert.ok(!res.issues.some((i) => i.severity === 'error'), 'demo examples must parse cleanly');
  }
});
