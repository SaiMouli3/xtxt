/**
 * XTXT — a plain-text document format with structure.
 *
 * A dependency-free implementation of the format defined in SPEC.md. This is a
 * port, not a binding: it agrees with the Go reference implementation because
 * both are checked against the same conformance suite.
 *
 * @module xtxt
 */

import { renderChart, renderTableChart } from './chart.js';

export { parseChart, renderChart, renderChartSVG, chartTableHTML,
  tableChart, chartData, renderTableChart } from './chart.js';

const NAME_START = /[A-Za-z_]/;
const NAME_BYTE = /[A-Za-z0-9_-]/;

export const FENCED_BY_DEFAULT = new Set([
  'code', 'table', 'math', 'mermaid', 'metadata', 'comment', 'raw',
]);

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

/** A directive's arguments, in source order. */
export class Args extends Array {
  get(key) {
    for (const a of this) if (a.key === key) return a.value;
    return '';
  }
  has(key) {
    return this.some((a) => a.key === key);
  }
  positional(i) {
    let n = 0;
    for (const a of this) {
      if (a.key === '') {
        if (n === i) return a.value;
        n++;
      }
    }
    return '';
  }
  /** The named argument, falling back to the first positional one. */
  resolve(key) {
    return this.get(key) || this.positional(0);
  }
}

/** One block in the document tree. */
export class Node {
  constructor(kind, opts = {}) {
    this.kind = kind;
    this.name = opts.name ?? '';
    this.level = opts.level ?? 0;
    this.text = opts.text ?? '';
    this.args = opts.args ?? new Args();
    this.items = opts.items ?? [];
    this.line = opts.line ?? 0;
  }
  /** The payload read as an ordered `Key: value` record. */
  fields() {
    return this.kind === 'block' ? parseFields(this.text) : new Fields();
  }
}

/** A parsed document. */
export class Document {
  constructor() {
    this.version = '';
    this.nodes = [];
  }
  metadata() {
    for (const n of this.nodes) {
      if (n.kind === 'block' && n.name === 'metadata') return parseMetadata(n.text);
    }
    return {};
  }
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

function readLines(src) {
  const lines = src.replace(/\r\n/g, '\n').split('\n');
  if (lines.length && lines[lines.length - 1] === '') lines.pop();
  if (lines.length) lines[0] = lines[0].replace(/^﻿/, '');
  return lines;
}

const isBlank = (s) => s.trim() === '';

function headingLevel(s) {
  s = s.replace(/^[ \t]+/, '');
  let n = 0;
  while (n < s.length && s[n] === '#') n++;
  if (n === 0 || n > 6 || n >= s.length || s[n] !== ' ') return 0;
  return n;
}

function isDirective(s) {
  const t = s.replace(/^[ \t]+/, '');
  return t.length >= 2 && t[0] === '@' && NAME_START.test(t[1]);
}

function directiveName(s) {
  const t = s.replace(/^[ \t]+/, '');
  let i = 1;
  while (i < t.length && NAME_BYTE.test(t[i])) i++;
  return [t.slice(1, i), t.slice(i)];
}

function itemPrefix(s) {
  const t = s.replace(/^[ \t]+/, '');
  if (t.length >= 2 && (t[0] === '-' || t[0] === '*') && t[1] === ' ') return t.slice(0, 2);
  let i = 0;
  while (i < t.length && t[i] >= '0' && t[i] <= '9') i++;
  if (i > 0 && i + 1 < t.length && t[i] === '.' && t[i + 1] === ' ') return t.slice(0, i + 2);
  return '';
}

/** Parse an XTXT document. Returns `{doc, issues}`; parsing never throws. */
export function parse(src) {
  const lines = readLines(src);
  const doc = new Document();
  const issues = [];
  let i = 0;

  const err = (line, message) => issues.push({ severity: 'error', line, message });

  function findFence(name, from) {
    const closer = '@end' + name;
    for (let j = from; j < lines.length; j++) {
      if (lines[j].replace(/[ \t]+$/, '') === closer) return j;
    }
    return -1;
  }

  function parseArgs(rest) {
    const trimmed = rest.trim();
    if (!trimmed.startsWith('(')) return [new Args(), true, i];
    let buf = trimmed;
    let line = i;
    for (;;) {
      const [inner, ok] = balanced(buf);
      if (ok) return [splitArgs(inner), true, line];
      line++;
      if (line >= lines.length) return [new Args(), false, i];
      buf += '\n' + lines[line];
    }
  }

  function directive() {
    const start = i;
    const [name, rest] = directiveName(lines[i]);
    const [args, ok, argEnd] = parseArgs(rest);
    if (!ok) {
      err(start + 1, `unclosed argument list for @${name}`);
      i = start + 1;
      return;
    }
    i = argEnd;

    if (name.startsWith('end')) {
      err(start + 1, `@${name} has no matching opening fence`);
      i++;
      return;
    }
    if (name === 'xtxt' && doc.nodes.length === 0) {
      doc.version = args.resolve('version');
      i++;
      return;
    }

    const end = findFence(name, i + 1);
    if (end >= 0) {
      const body = lines.slice(i + 1, end);
      i = end + 1;
      doc.nodes.push(new Node('block', {
        name, args, text: trimFenceBody(body), line: start + 1,
      }));
      return;
    }
    if (FENCED_BY_DEFAULT.has(name)) {
      err(start + 1, `unclosed @${name} block: no matching @end${name}`);
    }
    i++;
    doc.nodes.push(new Node('directive', { name, args, line: start + 1 }));
  }

  while (i < lines.length) {
    const line = lines[i];
    if (isBlank(line)) {
      i++;
    } else if (isDirective(line)) {
      directive();
    } else if (headingLevel(line) > 0) {
      const lvl = headingLevel(line);
      doc.nodes.push(new Node('heading', {
        level: lvl, text: line.replace(/^[ \t]+/, '').slice(lvl).trim(), line: i + 1,
      }));
      i++;
    } else if (line.trim().startsWith('>')) {
      const start = i;
      const parts = [];
      while (i < lines.length && lines[i].trim().startsWith('>')) {
        parts.push(lines[i].trim().slice(1).trim());
        i++;
      }
      doc.nodes.push(new Node('quote', { text: parts.join(' '), line: start + 1 }));
    } else if (itemPrefix(line)) {
      const start = i;
      const items = [];
      for (;;) {
        if (i >= lines.length) break;
        const pre = itemPrefix(lines[i]);
        if (!pre) break;
        let body = lines[i].replace(/^[ \t]+/, '').slice(pre.length).trim();
        const item = { text: '', ordered: pre[0] >= '0' && pre[0] <= '9', checked: null };
        if (body.length >= 3 && body[0] === '[' && body[2] === ']' && ' xX'.includes(body[1])) {
          item.checked = body[1] !== ' ';
          body = body.slice(3).trim();
        }
        item.text = body;
        items.push(item);
        i++;
      }
      doc.nodes.push(new Node('list', { items, line: start + 1 }));
    } else {
      const start = i;
      const parts = [];
      while (i < lines.length) {
        const l = lines[i];
        if (isBlank(l) || headingLevel(l) > 0 || isDirective(l) || itemPrefix(l)
            || l.trim().startsWith('>')) break;
        let t = l.trim();
        if (t.startsWith('\\@')) t = t.slice(1);
        parts.push(t);
        i++;
      }
      doc.nodes.push(new Node('paragraph', { text: parts.join(' '), line: start + 1 }));
    }
  }

  return { doc, issues, hasErrors: () => issues.some((x) => x.severity === 'error') };
}

/** Returns the text inside the outermost parens if `s` closes them. */
function balanced(s) {
  let depth = 0, inQuote = false, esc = false;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (esc) esc = false;
    else if (c === '\\') esc = true;
    else if (inQuote) { if (c === '"') inQuote = false; }
    else if (c === '"') inQuote = true;
    else if (c === '(') depth++;
    else if (c === ')') {
      depth--;
      if (depth === 0) return [s.slice(1, i), true];
    }
  }
  return ['', false];
}

function splitTop(s, sep) {
  const out = [];
  let depth = 0, inQuote = false, esc = false, start = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (esc) esc = false;
    else if (c === '\\') esc = true;
    else if (inQuote) { if (c === '"') inQuote = false; }
    else if (c === '"') inQuote = true;
    else if (c === '(') depth++;
    else if (c === ')') depth--;
    else if (c === sep && depth === 0) {
      out.push(s.slice(start, i));
      start = i + 1;
    }
  }
  out.push(s.slice(start));
  return out;
}

const isName = (s) => {
  s = s.trim();
  return s.length > 0 && NAME_START.test(s[0]) && [...s.slice(1)].every((c) => NAME_BYTE.test(c));
};

function splitArgs(s) {
  const args = new Args();
  for (const raw of splitTop(s, ',')) {
    const f = raw.trim();
    if (!f) continue;
    const parts = splitTop(f, '=');
    let key = '', val = f;
    if (parts.length >= 2 && isName(parts[0])) {
      key = parts[0].trim();
      val = f.slice(parts[0].length + 1).trim();
    }
    args.push({ key, value: unquote(val) });
  }
  return args;
}

function unquote(s) {
  s = s.trim();
  if (s.length >= 2 && s[0] === '"' && s[s.length - 1] === '"') return unescape_(s.slice(1, -1));
  return s;
}

function unescape_(s) {
  if (!s.includes('\\')) return s;
  let out = '';
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && i + 1 < s.length) {
      i++;
      out += s[i] === 'n' ? '\n' : s[i] === 't' ? '\t' : s[i];
    } else out += s[i];
  }
  return out;
}

function trimFenceBody(body) {
  body = body.slice();
  if (body.length && isBlank(body[0])) body.shift();
  if (body.length && isBlank(body[body.length - 1])) body.pop();
  return body
    .map((l) => (l.replace(/^[ \t]+/, '').startsWith('\\@end') ? l.replace('\\@end', '@end') : l))
    .join('\n');
}

function parseMetadata(payload) {
  const out = {};
  for (const line of payload.split('\n')) {
    if (isBlank(line)) continue;
    const eq = line.indexOf('=');
    if (eq < 0) continue;
    out[line.slice(0, eq).trim().toLowerCase()] = line.slice(eq + 1).trim();
  }
  return out;
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

const splitCells = (line) => line.trim().replace(/^\||\|$/g, '').split('|').map((c) => c.trim());
const isSeparatorRow = (cells) => cells.length > 0 && cells.every((c) => c && !c.replace(/[-:]/g, ''));

/** Interpret the payload of an `@table` block. */
export function parseTable(n) {
  const rows = [];
  let sepAt = -1, align = [];
  for (const line of n.text.split('\n')) {
    if (isBlank(line)) continue;
    const cells = splitCells(line);
    if (sepAt < 0 && isSeparatorRow(cells)) {
      sepAt = rows.length;
      align = cells.map((c) => (c.startsWith(':') && c.endsWith(':') ? 'center'
        : c.endsWith(':') ? 'right' : 'left'));
      continue;
    }
    rows.push(cells);
  }
  if (!rows.length) return { header: [], rows: [], align: [] };
  if (sepAt <= 0) sepAt = 1;
  return { header: sepAt > 1 ? rows[sepAt - 1] : rows[0], rows: rows.slice(sepAt), align };
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

/**
 * An ordered `Key: value` record; order is meaningful (see `@chat`).
 *
 * Note the name `toObject` rather than `map`: this extends Array, so a method
 * called `map` would shadow `Array.prototype.map` and quietly break every
 * `fields.map(fn)` in the codebase. Go and Python spell the same method `Map`
 * and `map` because their list types have no such collision.
 */
export class Fields extends Array {
  get(key) {
    for (const f of this) if (f.key.toLowerCase() === key.toLowerCase()) return f.value;
    return '';
  }
  /** Flatten to a lowercase-keyed object, keeping the first of any duplicate. */
  toObject() {
    const out = {};
    for (const f of this) {
      const k = f.key.toLowerCase();
      if (!(k in out)) out[k] = f.value;
    }
    return out;
  }
}

// A field key is a label, not a sentence: these caps are what keep ordinary
// prose containing a colon from being read as a record field.
const MAX_FIELD_KEY_LEN = 32;
const MAX_FIELD_KEY_WORDS = 3;

function isFieldLine(line) {
  for (let i = 0; i < line.length && i <= MAX_FIELD_KEY_LEN; i++) {
    const c = line[i];
    if (c === ':' || c === '=') {
      const k = line.slice(0, i).trim();
      if (!k || !NAME_START.test(k[0]) || k.split(/\s+/).length > MAX_FIELD_KEY_WORDS) return null;
      return { key: k, value: line.slice(i + 1).trim() };
    }
    if (!(c === ' ' || c === '_' || c === '-' || c === '.' || NAME_BYTE.test(c))) return null;
  }
  return null;
}

/** Interpret a block payload as an ordered record. */
export function parseFields(payload) {
  const out = new Fields();
  let cur = null;
  const preamble = [];
  for (const line of payload.split('\n')) {
    const f = isFieldLine(line);
    if (f) {
      out.push(f);
      cur = out[out.length - 1];
      continue;
    }
    if (!cur) { preamble.push(line); continue; }
    cur.value = cur.value === '' ? line.trim() : cur.value + '\n' + line;
  }
  for (const f of out) f.value = f.value.trim();
  const text = preamble.join('\n').trim();
  if (text) out.unshift({ key: '', value: text });
  return out;
}

// ---------------------------------------------------------------------------
// Inline formatting
// ---------------------------------------------------------------------------

const escapeHTML = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;')
  .replace(/>/g, '&gt;').replace(/"/g, '&#34;').replace(/'/g, '&#39;');

function footnoteRef(s, i) {
  if (i + 2 >= s.length || s[i + 1] !== '^') return null;
  const close = s.indexOf(']', i + 2);
  if (close < 0 || close === i + 2) return null;
  const id = s.slice(i + 2, close);
  if (/[ \t]/.test(id)) return null;
  return { id, end: close };
}

function findClose(s, from, mark) {
  for (let i = from; i + mark.length <= s.length; i++) {
    if (s[i] === '\\') { i++; continue; }
    if (s.startsWith(mark, i)) return i === from ? -1 : i;
  }
  return -1;
}

function parseLink(s, i) {
  const close = findClose(s, i + 1, ']');
  if (close < 0 || close + 1 >= s.length || s[close + 1] !== '(') return null;
  const [inner, ok] = balanced(s.slice(close + 1));
  if (!ok) return null;
  return { label: s.slice(i + 1, close), target: inner.trim(), end: close + 1 + inner.length + 1 };
}

/** Convert inline markup to HTML, escaping everything else. */
export function inlineHTML(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === '\\' && i + 1 < s.length) {
      i++;
      out += escapeHTML(s[i]);
    } else if (c === '`') {
      const end = s.indexOf('`', i + 1);
      if (end >= 0) { out += '<code>' + escapeHTML(s.slice(i + 1, end)) + '</code>'; i = end; }
      else out += '&#96;';
    } else if (c === '*') {
      const mark = s.startsWith('**', i) ? '**' : '*';
      const tag = mark === '**' ? 'strong' : 'em';
      const end = findClose(s, i + mark.length, mark);
      if (end >= 0) {
        out += `<${tag}>${inlineHTML(s.slice(i + mark.length, end))}</${tag}>`;
        i = end + mark.length - 1;
      } else out += '*';
    } else if (c === '[') {
      const fn = footnoteRef(s, i);
      if (fn) {
        const e = escapeHTML(fn.id);
        out += `<sup class="fnref" id="fnref-${e}"><a href="#fn-${e}">${e}</a></sup>`;
        i = fn.end;
        continue;
      }
      const lk = parseLink(s, i);
      if (lk) { out += `<a href="${escapeHTML(lk.target)}">${inlineHTML(lk.label)}</a>`; i = lk.end; }
      else out += '[';
    } else out += escapeHTML(c);
  }
  return out;
}

/** Strip inline markup, for plain text and analysis. */
export function inlineText(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === '\\' && i + 1 < s.length) { i++; out += s[i]; }
    else if (c === '`') {
      const end = s.indexOf('`', i + 1);
      if (end >= 0) { out += s.slice(i + 1, end); i = end; } else out += c;
    } else if (c === '*') {
      const mark = s.startsWith('**', i) ? '**' : '*';
      const end = findClose(s, i + mark.length, mark);
      if (end >= 0) { out += inlineText(s.slice(i + mark.length, end)); i = end + mark.length - 1; }
      else out += c;
    } else if (c === '[') {
      const fn = footnoteRef(s, i);
      if (fn) { out += `[${fn.id}]`; i = fn.end; continue; }
      const lk = parseLink(s, i);
      if (lk) { out += inlineText(lk.label); i = lk.end; } else out += c;
    } else out += c;
  }
  return out;
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

export const KNOWN = new Map(Object.entries({
  xtxt: false, image: false, video: false, audio: false,
  attachment: false, include: false, embed: false, hr: false,
  code: true, table: true, math: true, mermaid: true,
  metadata: true, comment: true, raw: true, chart: true, footnote: true,
  task: true, decision: true, knowledge: true,
  ai: true, prompt: true, chat: true, note: true,
}));

const REQUIRED_SRC = new Set(['image', 'video', 'audio', 'attachment', 'include']);

/** Semantic checks on top of the parser's syntactic ones. */
export function validate(doc) {
  const issues = [];
  const warn = (line, message) => issues.push({ severity: 'warning', line, message });
  let metadataSeen = false;
  const notes = new Map();

  for (const n of doc.nodes) {
    if (n.kind !== 'directive' && n.kind !== 'block') continue;
    if (!KNOWN.has(n.name)) {
      warn(n.line, `unknown directive @${n.name} (preserved, but this reader cannot render it)`);
      continue;
    }
    const fenced = KNOWN.get(n.name);
    if (fenced && n.kind !== 'block') {
      warn(n.line, `@${n.name} is a block directive and should be closed with @end${n.name}`);
    }
    if (!fenced && n.kind === 'block') {
      warn(n.line, `@${n.name} is not a block directive but was closed with @end${n.name}`);
    }
    if (REQUIRED_SRC.has(n.name) && !n.args.resolve('src')) warn(n.line, `@${n.name} has no src`);

    switch (n.name) {
      case 'metadata':
        if (metadataSeen) warn(n.line, 'duplicate @metadata block');
        metadataSeen = true;
        break;
      case 'table': {
        const t = parseTable(n);
        if (!t.header.length) { warn(n.line, '@table is empty'); break; }
        t.rows.forEach((row, i) => {
          if (row.length !== t.header.length) {
            warn(n.line + 1 + i,
              `table row has ${row.length} cells, header has ${t.header.length}`);
          }
        });
        break;
      }
      case 'code':
        if (!n.args.resolve('language')) {
          warn(n.line, '@code has no language; syntax highlighting will be skipped');
        }
        break;
      case 'chart':
        if (!chartRows(n).length) warn(n.line, '@chart has no readable data rows');
        break;
      case 'task':
        if (!n.fields().get('title')) warn(n.line, '@task has no Title field');
        break;
      case 'footnote': {
        const id = n.args.resolve('id');
        if (!id) warn(n.line, '@footnote has no id; references cannot point at it');
        notes.set(id, n.line);
        break;
      }
    }
  }
  issues.push(...checkFootnoteRefs(doc, notes));
  return issues;
}

function chartRows(n) {
  const rows = [];
  for (const line of n.text.split('\n')) {
    if (isBlank(line)) continue;
    let cells;
    if (line.includes('|')) cells = splitCells(line);
    else {
      const f = line.trim().split(/\s+/);
      cells = f.length >= 2 ? [f.slice(0, -1).join(' '), f[f.length - 1]] : f;
    }
    if (cells.length >= 2 && !isSeparatorRow(cells)) rows.push(cells);
  }
  return rows;
}

function checkFootnoteRefs(doc, notes) {
  const issues = [];
  const cited = new Set();
  const visit = (text, line) => {
    for (let i = 0; i < text.length; i++) {
      if (text[i] === '\\') { i++; continue; }
      if (text[i] !== '[') continue;
      const fn = footnoteRef(text, i);
      if (!fn) continue;
      cited.add(fn.id);
      if (!notes.has(fn.id)) {
        issues.push({
          severity: 'warning', line,
          message: `footnote reference [^${fn.id}] has no matching @footnote(id="${fn.id}")`,
        });
      }
      i = fn.end;
    }
  };
  for (const n of doc.nodes) {
    if (n.kind === 'heading' || n.kind === 'paragraph' || n.kind === 'quote') visit(n.text, n.line);
    else if (n.kind === 'list') for (const it of n.items) visit(it.text, n.line);
  }
  for (const [id, line] of notes) {
    if (id && !cited.has(id)) {
      issues.push({ severity: 'warning', line, message: `@footnote(id="${id}") is never referenced` });
    }
  }
  return issues;
}

export const sortIssues = (issues) => [...issues].sort((a, b) => a.line - b.line);

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

const PRESENTATIONAL = new Set([
  'code', 'table', 'math', 'mermaid', 'metadata', 'comment', 'raw', 'image',
  'video', 'audio', 'attachment', 'hr', 'xtxt', 'include', 'embed', 'footnote',
]);

function linksIn(s, line) {
  const out = [];
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\') { i++; continue; }
    if (s[i] !== '[') continue;
    const lk = parseLink(s, i);
    if (lk) { out.push({ text: inlineText(lk.label), href: lk.target, line }); i = lk.end; }
  }
  return out;
}

/** Everything an agent needs without inferring structure from prose. */
export function extract(doc) {
  const out = {
    version: doc.version, metadata: doc.metadata(),
    outline: [], tasks: [], blocks: [], links: [], media: [], code: [],
    text: '', words: 0,
  };
  const prose = [];

  for (const n of doc.nodes) {
    switch (n.kind) {
      case 'heading': {
        const text = inlineText(n.text);
        out.outline.push({ level: n.level, text, line: n.line });
        prose.push(text);
        out.links.push(...linksIn(n.text, n.line));
        break;
      }
      case 'paragraph':
      case 'quote':
        prose.push(inlineText(n.text));
        out.links.push(...linksIn(n.text, n.line));
        break;
      case 'list':
        for (const it of n.items) {
          prose.push(inlineText(it.text));
          out.links.push(...linksIn(it.text, n.line));
          if (it.checked !== null) {
            out.tasks.push({ title: inlineText(it.text), done: it.checked, line: n.line });
          }
        }
        break;
      case 'directive':
      case 'block':
        absorb(out, n, prose);
        break;
    }
  }
  out.text = prose.join('\n\n');
  out.words = out.text.split(/\s+/).filter(Boolean).length;
  return out;
}

function absorb(out, n, prose) {
  if (n.name === 'comment' || n.name === 'metadata') return;
  if (['image', 'video', 'audio', 'attachment'].includes(n.name)) {
    out.media.push({
      kind: n.name, src: n.args.resolve('src'),
      caption: inlineText(n.args.get('caption')), line: n.line,
    });
    if (n.args.get('caption')) prose.push(inlineText(n.args.get('caption')));
    return;
  }
  if (n.name === 'code') {
    out.code.push({
      language: n.args.resolve('language'),
      lines: n.text.split('\n').length, line: n.line, source: n.text,
    });
    return;
  }
  if (n.name === 'table') {
    const t = parseTable(n);
    for (const row of [t.header, ...t.rows]) prose.push(row.join(' | '));
    return;
  }
  if (PRESENTATIONAL.has(n.name)) {
    if (n.kind === 'block' && n.text) prose.push(n.text);
    return;
  }

  const f = n.fields();
  const block = { type: n.name, line: n.line, text: n.text };
  if (n.args.length) {
    block.args = {};
    n.args.forEach((a, i) => { block.args[a.key || String(i)] = a.value; });
  }
  if (f.length) {
    block.fields = f.toObject();
    block.order = [...f].map((x) => x.key);
  }
  out.blocks.push(block);

  if (n.name === 'task') {
    const m = f.toObject();
    const status = m.status ?? '';
    out.tasks.push({
      title: m.title || m[''] || '', status, owner: m.owner ?? '', due: m.due ?? '',
      done: ['done', 'complete'].includes(status.toLowerCase()), line: n.line,
    });
  }
  if (n.kind === 'block' && n.text) prose.push(n.text);
}

// ---------------------------------------------------------------------------
// Conformance
// ---------------------------------------------------------------------------

/** The normalised shape used by the conformance suite. */
export function canonical(doc) {
  return {
    version: doc.version,
    nodes: doc.nodes.map((n) => ({
      kind: n.kind,
      name: n.name,
      level: n.level,
      text: n.text,
      args: [...n.args].map((a) => ({ key: a.key, value: a.value })),
      items: n.items.map((it) => ({
        text: it.text, ordered: it.ordered, checked: it.checked,
      })),
      line: n.line,
    })),
  };
}

export const canonicalIssues = (issues) =>
  sortIssues(issues).map((i) => ({ severity: i.severity, line: i.line }));

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

/** Render to HTML. `full` wraps the result in a standalone document. */
export function renderHTML(doc, { full = false, title = '' } = {}) {
  const body = [];
  const notes = [];
  for (const n of doc.nodes) {
    switch (n.kind) {
      case 'heading': body.push(`<h${n.level}>${inlineHTML(n.text)}</h${n.level}>`); break;
      case 'paragraph': body.push(`<p>${inlineHTML(n.text)}</p>`); break;
      case 'quote': body.push(`<blockquote><p>${inlineHTML(n.text)}</p></blockquote>`); break;
      case 'list': body.push(listHTML(n)); break;
      case 'directive':
      case 'block':
        if (n.name === 'footnote') notes.push(n);
        else body.push(directiveHTML(n));
        break;
    }
  }
  if (notes.length) {
    const items = notes.map((n, i) => {
      const id = escapeHTML(n.args.resolve('id') || String(i + 1));
      return `<li id="fn-${id}">${inlineHTML(n.text)} <a class="fnback" href="#fnref-${id}">&#8617;</a></li>`;
    }).join('');
    body.push(`<section class="footnotes"><ol>${items}</ol></section>`);
  }

  const out = body.filter(Boolean).join('\n');
  if (!full) return out;
  const t = title || doc.metadata().title
    || doc.nodes.find((n) => n.kind === 'heading')?.text || 'Untitled';
  return '<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
    + '<meta name="viewport" content="width=device-width, initial-scale=1">\n'
    + `<title>${escapeHTML(inlineText(t))}</title>\n</head>\n<body>\n<main class="xtxt">\n`
    + out + '\n</main>\n</body>\n</html>\n';
}

function listHTML(n) {
  if (!n.items.length) return '';
  const tag = n.items[0].ordered ? 'ol' : 'ul';
  const cls = n.items[0].checked !== null ? ' class="checklist"' : '';
  const items = n.items.map((it) => {
    const box = it.checked === null ? ''
      : `<input type="checkbox" disabled${it.checked ? ' checked' : ''}> `;
    return `<li>${box}${inlineHTML(it.text)}</li>`;
  }).join('');
  return `<${tag}${cls}>${items}</${tag}>`;
}

function directiveHTML(n) {
  const e = escapeHTML;
  const cap = n.args.get('caption');
  const figcap = cap ? `<figcaption>${inlineHTML(cap)}</figcaption>` : '';
  switch (n.name) {
    case 'comment': case 'metadata': return '';
    case 'hr': return '<hr>';
    case 'image': {
      const alt = n.args.get('alt') || inlineText(cap);
      const attrs = ['width', 'height']
        .filter((k) => n.args.get(k)).map((k) => ` ${k}="${e(n.args.get(k))}"`).join('');
      return `<figure><img src="${e(n.args.resolve('src'))}" alt="${e(alt)}"${attrs}>${figcap}</figure>`;
    }
    case 'video': case 'audio': {
      const extra = n.name === 'video' ? ' controls playsinline' : ' controls';
      return `<figure><${n.name} src="${e(n.args.resolve('src'))}"${extra}></${n.name}>${figcap}</figure>`;
    }
    case 'attachment': {
      const src = n.args.resolve('src');
      return `<p class="attachment"><a href="${e(src)}" download>${e(n.args.get('name') || src)}</a></p>`;
    }
    case 'code': {
      const lang = n.args.resolve('language');
      return `<pre><code${lang ? ` class="language-${e(lang)}"` : ''}>${e(n.text)}</code></pre>`;
    }
    case 'math': return `<div class="math">${e(n.text)}</div>`;
    case 'mermaid': return `<pre class="mermaid">${e(n.text)}</pre>`;
    case 'raw': return n.args.resolve('format') === 'html' ? n.text : `<pre>${e(n.text)}</pre>`;
    case 'table': return renderTableChart(n, parseTable(n)) + tableHTML(n);
    case 'chart': return renderChart(n);
    default: {
      if (n.kind === 'block') {
        const f = n.fields();
        if (f.length) {
          const rows = [...f]
            .map((x) => `<dt>${e(x.key || '—')}</dt><dd>${inlineHTML(x.value)}</dd>`).join('');
          return `<section class="record" data-type="${e(n.name)}">`
            + `<h4 class="record-type">${e(n.name)}</h4><dl>${rows}</dl></section>`;
        }
      }
      return `<div class="unknown" data-directive="${e(n.name)}">${e(sourceOf(n))}</div>`;
    }
  }
}

function tableHTML(n) {
  const t = parseTable(n);
  if (!t.header.length) return '';
  const style = (i) => {
    const a = t.align[i] ?? 'left';
    return a === 'left' ? '' : ` style="text-align:${a}"`;
  };
  const head = t.header.map((h, i) => `<th${style(i)}>${inlineHTML(h)}</th>`).join('');
  const rows = t.rows.map((r) =>
    '<tr>' + r.map((c, i) => `<td${style(i)}>${inlineHTML(c)}</td>`).join('') + '</tr>').join('');
  return `<table>\n<thead><tr>${head}</tr></thead>\n<tbody>${rows}</tbody>\n</table>`;
}

function sourceOf(n) {
  const parts = [...n.args].map((a) => {
    let v = a.value;
    if (!v || /[ ,)"]/.test(v)) v = '"' + v.replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"';
    return a.key ? `${a.key}=${v}` : v;
  });
  let out = '@' + n.name + (parts.length ? `(${parts.join(', ')})` : '');
  if (n.kind === 'block') out += `\n${n.text}\n@end${n.name}`;
  return out;
}
