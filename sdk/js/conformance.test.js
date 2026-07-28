// The JS SDK is checked against the same fixtures as the Go reference.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

import { canonical, canonicalIssues, extract, inlineHTML, inlineText, parse, validate } from './xtxt.js';

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
