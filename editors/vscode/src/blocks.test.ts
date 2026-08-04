import assert from 'node:assert/strict';
import { test } from 'node:test';

import { blockRanges, headerCells } from './blocks';
import { isUnder } from './paths';

test('pairs a block and leaves a single-line directive alone', () => {
  assert.deepEqual(
    blockRanges([
      '@image(src="cnn.png")', // 0 — no terminator, must not open a region
      '@task', // 1
      'Title: Ship it', // 2
      '@endtask', // 3
    ]),
    [{ name: 'task', start: 1, end: 3 }],
  );
});

test('reads no structure out of a fenced payload', () => {
  assert.deepEqual(
    blockRanges([
      '@code(lang="python")', // 0
      '@app.route("/")', // 1 — a decorator, not a directive
      '@endtask', // 2 — not this block's terminator
      '@endcode', // 3
    ]),
    [{ name: 'code', start: 0, end: 3 }],
  );
});

test('nests, innermost first', () => {
  assert.deepEqual(
    blockRanges(['@doc', '@task', '@endtask', '@enddoc']),
    [
      { name: 'task', start: 1, end: 2 },
      { name: 'doc', start: 0, end: 3 },
    ],
  );
});

test('an unclosed block yields no range', () => {
  assert.deepEqual(blockRanges(['@task', 'Title: Ship it']), []);
});

test('isUnder accepts the root and what is inside it', () => {
  assert.equal(isUnder('/work/notes', '/work/notes'), true);
  assert.equal(isUnder('/work/notes', '/work/notes/img/a.png'), true);
  assert.equal(isUnder('/work/notes/', '/work/notes/a.png'), true);
});

test('isUnder rejects an escape and a shared prefix', () => {
  // What `xtxt.paste.folder: "../../evil"` normalises to.
  assert.equal(isUnder('/work/notes', '/work/evil'), false);
  assert.equal(isUnder('/work/notes', '/'), false);
  // The reason a bare startsWith is not enough.
  assert.equal(isUnder('/work/notes', '/work/notes-evil/a.png'), false);
});

test('headerCells skips the separator row, not the header', () => {
  const lines = ['@table(chart="bar")', 'Month | Signups', '------|--------', 'Jan | 20', '@endtable'];
  const [block] = blockRanges(lines);
  assert.deepEqual(headerCells(lines, block), ['Month', 'Signups']);
});

test('headerCells reads a table that has no separator', () => {
  const lines = ['@table', 'Name | Age', 'John | 20', '@endtable'];
  const [block] = blockRanges(lines);
  assert.deepEqual(headerCells(lines, block), ['Name', 'Age']);
});

test('headerCells gives nothing for an empty table', () => {
  const lines = ['@table', '@endtable'];
  const [block] = blockRanges(lines);
  assert.deepEqual(headerCells(lines, block), []);
});
