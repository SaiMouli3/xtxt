#!/usr/bin/env node
/**
 * xtxt-mcp — an MCP server that hands agents the structure inside XTXT
 * documents instead of making them infer it from prose.
 *
 *     xtxt-mcp /path/to/notes
 *
 * The single argument is a root directory. Every path the tools accept is
 * resolved inside it and anything that escapes is refused: exposing a document
 * reader to an agent must not become a way to read the whole filesystem.
 */

import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { z } from 'zod';

import { extract, parse, renderHTML, validate } from 'xtxt-js';

const ROOT = path.resolve(process.argv[2] ?? process.cwd());
const MAX_FILES = 2000;

/**
 * Resolve a caller-supplied path inside ROOT, or throw. Symlinks are resolved
 * before the check so a link cannot be used to step outside.
 */
async function safePath(relative) {
  if (path.isAbsolute(relative)) {
    throw new Error(`absolute paths are not allowed: ${relative}`);
  }
  const joined = path.resolve(ROOT, relative);
  let real = joined;
  try {
    real = await (await import('node:fs/promises')).realpath(joined);
  } catch {
    // The file may not exist; the containment check below still applies to the
    // cleaned path, and the caller gets a plain ENOENT afterwards.
  }
  const rel = path.relative(ROOT, real);
  if (rel.startsWith('..') || path.isAbsolute(rel)) {
    throw new Error(`path escapes the document root: ${relative}`);
  }
  return joined;
}

/** Every .xtxt file under ROOT, relative to it. */
async function listDocuments(dir = ROOT, out = []) {
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const entry of entries) {
    if (out.length >= MAX_FILES) break;
    if (entry.name.startsWith('.') || entry.name === 'node_modules') continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      await listDocuments(full, out);
    } else if (entry.name.endsWith('.xtxt')) {
      out.push(path.relative(ROOT, full));
    }
  }
  return out;
}

async function load(relative) {
  const file = await safePath(relative);
  const source = await readFile(file, 'utf8');
  return { source, ...parse(source) };
}

const server = new McpServer({ name: 'xtxt', version: '0.1.0' });

const json = (value) => ({ content: [{ type: 'text', text: JSON.stringify(value, null, 2) }] });
const text = (value) => ({ content: [{ type: 'text', text: value }] });
const fail = (message) => ({ content: [{ type: 'text', text: message }], isError: true });

server.registerTool(
  'xtxt_extract',
  {
    title: 'Extract structure from an XTXT document',
    description:
      'Return the machine-facing view of an XTXT document: outline, tasks, records ' +
      '(@task, @decision, @knowledge and any other block shaped like one), links, ' +
      'media, code blocks and the plain text. Prefer this over reading the file: ' +
      'the structure is parsed, not inferred.',
    inputSchema: {
      path: z.string().describe('Path to a .xtxt file, relative to the document root'),
    },
  },
  async ({ path: relative }) => {
    try {
      const { doc } = await load(relative);
      return json({ path: relative, ...extract(doc) });
    } catch (err) {
      return fail(`xtxt_extract failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_list',
  {
    title: 'List XTXT documents',
    description:
      'List every .xtxt document under the root, with its title, heading outline and ' +
      'block counts. Use this first to find the document you want.',
    inputSchema: {},
  },
  async () => {
    try {
      const files = await listDocuments();
      const out = [];
      for (const file of files) {
        try {
          const { doc } = await load(file);
          const data = extract(doc);
          out.push({
            path: file,
            title: data.metadata.title ?? data.outline[0]?.text ?? file,
            headings: data.outline.length,
            tasks: data.tasks.length,
            records: data.blocks.length,
            words: data.words,
          });
        } catch {
          out.push({ path: file, error: 'could not be parsed' });
        }
      }
      return json({ root: ROOT, count: out.length, documents: out });
    } catch (err) {
      return fail(`xtxt_list failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_tasks',
  {
    title: 'Collect tasks across all documents',
    description:
      'Every task in every document under the root, from both checklist items and ' +
      '@task blocks, with status, owner and due date where present. This is a parse, ' +
      'not a search: nothing is missed and nothing is guessed.',
    inputSchema: {
      open_only: z.boolean().optional().describe('Return only tasks that are not done'),
      owner: z.string().optional().describe('Only tasks with this owner (case-insensitive)'),
    },
  },
  async ({ open_only = false, owner }) => {
    try {
      const files = await listDocuments();
      const tasks = [];
      for (const file of files) {
        try {
          const { doc } = await load(file);
          for (const task of extract(doc).tasks) {
            if (open_only && task.done) continue;
            if (owner && (task.owner ?? '').toLowerCase() !== owner.toLowerCase()) continue;
            tasks.push({ ...task, path: file });
          }
        } catch {
          // A document that will not parse is reported by xtxt_validate, not here.
        }
      }
      return json({ count: tasks.length, tasks });
    } catch (err) {
      return fail(`xtxt_tasks failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_records',
  {
    title: 'Find records by type',
    description:
      'Every record block of a given type across all documents — @decision, @knowledge, ' +
      '@chat, or a type this build has never heard of. Field names are returned exactly ' +
      'as written; the format assigns them no meaning.',
    inputSchema: {
      type: z
        .string()
        .optional()
        .describe('Record type such as "decision" or "knowledge". Omit for every type.'),
    },
  },
  async ({ type }) => {
    try {
      const files = await listDocuments();
      const blocks = [];
      for (const file of files) {
        try {
          const { doc } = await load(file);
          for (const block of extract(doc).blocks) {
            if (type && block.type !== type) continue;
            blocks.push({ ...block, path: file });
          }
        } catch {
          /* see xtxt_validate */
        }
      }
      const types = [...new Set(blocks.map((b) => b.type))].sort();
      return json({ count: blocks.length, types, blocks });
    } catch (err) {
      return fail(`xtxt_records failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_search',
  {
    title: 'Search XTXT documents',
    description:
      'Case-insensitive search across document prose and record field values. Returns ' +
      'the matching documents with the headings and records that matched.',
    inputSchema: {
      query: z.string().describe('Text to look for'),
    },
  },
  async ({ query }) => {
    try {
      const needle = query.toLowerCase();
      const files = await listDocuments();
      const hits = [];
      for (const file of files) {
        try {
          const { doc } = await load(file);
          const data = extract(doc);
          const inText = data.text.toLowerCase().includes(needle);
          const headings = data.outline.filter((h) => h.text.toLowerCase().includes(needle));
          const records = data.blocks.filter((b) =>
            Object.values(b.fields ?? {}).some((v) => String(v).toLowerCase().includes(needle)));
          if (inText || headings.length || records.length) {
            hits.push({
              path: file,
              title: data.metadata.title ?? data.outline[0]?.text ?? file,
              headings,
              records,
            });
          }
        } catch {
          /* see xtxt_validate */
        }
      }
      return json({ query, count: hits.length, matches: hits });
    } catch (err) {
      return fail(`xtxt_search failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_validate',
  {
    title: 'Validate an XTXT document',
    description:
      'Report syntax errors and semantic warnings for a document. Unknown directives are ' +
      'warnings, never errors: a reader from today stays usable on a document written ' +
      'against a later version of the spec.',
    inputSchema: {
      path: z.string().describe('Path to a .xtxt file, relative to the document root'),
    },
  },
  async ({ path: relative }) => {
    try {
      const { doc, issues } = await load(relative);
      const all = [...issues, ...validate(doc)].sort((a, b) => a.line - b.line);
      return json({
        path: relative,
        ok: !all.some((i) => i.severity === 'error'),
        blocks: doc.nodes.length,
        issues: all,
      });
    } catch (err) {
      return fail(`xtxt_validate failed: ${err.message}`);
    }
  },
);

server.registerTool(
  'xtxt_render',
  {
    title: 'Render an XTXT document',
    description: 'Render a document to an HTML fragment or to its plain text.',
    inputSchema: {
      path: z.string().describe('Path to a .xtxt file, relative to the document root'),
      format: z.enum(['html', 'text']).default('text').describe('Output format'),
    },
  },
  async ({ path: relative, format }) => {
    try {
      const { doc } = await load(relative);
      return text(format === 'html' ? renderHTML(doc) : extract(doc).text);
    } catch (err) {
      return fail(`xtxt_render failed: ${err.message}`);
    }
  },
);

/* Documents are also exposed as resources, for clients that browse rather than call. */
server.registerResource(
  'document',
  'xtxt://document',
  { title: 'XTXT documents', description: 'Every .xtxt document under the root' },
  async () => {
    const files = await listDocuments();
    return {
      contents: files.map((file) => ({
        uri: `xtxt://document/${file}`,
        mimeType: 'text/xtxt',
        text: file,
      })),
    };
  },
);

async function main() {
  try {
    const info = await stat(ROOT);
    if (!info.isDirectory()) throw new Error('not a directory');
  } catch (err) {
    console.error(`xtxt-mcp: cannot use ${ROOT} as a document root: ${err.message}`);
    process.exit(2);
  }
  await server.connect(new StdioServerTransport());
  console.error(`xtxt-mcp: serving ${ROOT}`);
}

main().catch((err) => {
  console.error(`xtxt-mcp: ${err.stack ?? err.message}`);
  process.exit(1);
});
