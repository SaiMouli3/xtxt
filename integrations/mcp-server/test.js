// Drives the server over a real stdio transport, the way a client would.
//
// One server for the whole file: spawning one per test leaves child processes
// behind and the test file never exits.

import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { after, before, test } from 'node:test';

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..', '..', 'examples');

let client;
let transport;

before(async () => {
  client = new Client({ name: 'test', version: '0' });
  transport = new StdioClientTransport({
    command: process.execPath,
    args: [path.join(here, 'src', 'index.js'), root],
    stderr: 'ignore',
  });
  await client.connect(transport);
});

after(async () => {
  await client.close();
  await transport.close();
});

const call = async (name, args = {}) => client.callTool({ name, arguments: args });
const callJSON = async (name, args = {}) => JSON.parse((await call(name, args)).content[0].text);

test('exposes the expected tools', async () => {
  const names = (await client.listTools()).tools.map((t) => t.name).sort();
  assert.deepEqual(names, [
    'xtxt_extract', 'xtxt_list', 'xtxt_records', 'xtxt_render',
    'xtxt_search', 'xtxt_tasks', 'xtxt_validate',
  ]);
});

test('lists documents with their structure', async () => {
  const out = await callJSON('xtxt_list');
  assert.ok(out.count >= 2, `expected the example documents, got ${out.count}`);
  const agent = out.documents.find((d) => d.path.includes('agent-notes'));
  assert.ok(agent, 'agent-notes.xtxt listed');
  assert.equal(agent.title, 'Project Log — XTXT Parser');
  assert.ok(agent.tasks > 0 && agent.records > 0);
});

test('extracts records rather than prose', async () => {
  const out = await callJSON('xtxt_extract', { path: 'agent-notes.xtxt' });
  assert.ok(out.outline.length > 0);
  assert.ok(out.blocks.some((b) => b.type === 'decision'));

  // A block type this build has never heard of still reaches the agent with
  // its fields intact — the whole point of SPEC §7.
  const experiment = out.blocks.find((b) => b.type === 'experiment');
  assert.ok(experiment, 'unknown record survives');
  assert.equal(experiment.fields.confidence, '0.7');
});

test('collects tasks across documents and filters them', async () => {
  const all = await callJSON('xtxt_tasks');
  assert.ok(all.count > 0);

  const open = await callJSON('xtxt_tasks', { open_only: true });
  assert.ok(open.tasks.every((t) => !t.done), 'open_only excludes done tasks');
  assert.ok(open.count < all.count, 'filtering actually filtered');

  const mine = await callJSON('xtxt_tasks', { owner: 'subbu' });
  assert.ok(mine.count > 0, 'owner filter matched');
  assert.ok(mine.tasks.every((t) => t.owner.toLowerCase() === 'subbu'));
});

test('finds records by type', async () => {
  const out = await callJSON('xtxt_records', { type: 'decision' });
  assert.ok(out.count >= 2, `expected the decisions, got ${out.count}`);
  assert.ok(out.blocks.every((b) => b.type === 'decision'));

  const every = await callJSON('xtxt_records');
  assert.ok(every.types.includes('experiment'), 'unknown types are listed too');
});

test('searches prose and record fields', async () => {
  const prose = await callJSON('xtxt_search', { query: 'conformance' });
  assert.ok(prose.count >= 1, 'matched document prose');

  // "Recursive descent" appears only as a @knowledge Topic field value, so a
  // hit here proves record fields are searched and not just the prose.
  const field = await callJSON('xtxt_search', { query: 'recursive descent' });
  assert.ok(field.count >= 1, 'matched a record field case-insensitively');
  assert.ok(field.matches.some((m) => m.records.length > 0), 'reported the matching record');
});

test('validates and reports unknown directives as warnings', async () => {
  const out = await callJSON('xtxt_validate', { path: 'agent-notes.xtxt' });
  assert.equal(out.ok, true, 'document is valid');
  assert.ok(out.issues.every((i) => i.severity === 'warning'));
});

test('renders to text and html', async () => {
  const text = (await call('xtxt_render', { path: 'agent-notes.xtxt', format: 'text' }))
    .content[0].text;
  assert.ok(text.includes('Project Log'));
  assert.ok(!text.includes('<h1>'), 'text output is not html');

  const html = (await call('xtxt_render', { path: 'agent-notes.xtxt', format: 'html' }))
    .content[0].text;
  assert.ok(html.includes('<h1>Project Log</h1>'));
});

test('refuses to read outside the document root', async () => {
  for (const bad of ['../../SPEC.md', '/etc/passwd', '../../../../../../etc/hosts']) {
    const res = await call('xtxt_extract', { path: bad });
    assert.equal(res.isError, true, `${bad} must be refused`);
    assert.match(res.content[0].text, /escapes the document root|absolute paths/);
  }
});

test('reports a missing file without crashing', async () => {
  const res = await call('xtxt_extract', { path: 'nope.xtxt' });
  assert.equal(res.isError, true);
  assert.match(res.content[0].text, /xtxt_extract failed/);
});
