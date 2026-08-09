/**
 * XTXT for VS Code — syntax highlighting, live preview and image pasting.
 *
 * The preview renders with the same JavaScript SDK as the browser demo, the
 * CLI's HTML export and the Obsidian plugin, so what you see here and what
 * `xtxt export … html` produces cannot drift apart.
 *
 * VS Code cannot draw images inline in a text editor — no extension API
 * exposes that — so "see the image in the document" means the preview panel
 * beside it, which is how every Markdown extension solves the same problem.
 */

import { Buffer } from 'node:buffer';
import { randomBytes } from 'node:crypto';
import { execFile } from 'node:child_process';
import { tmpdir } from 'node:os';
import { promisify } from 'node:util';
import * as vscode from 'vscode';

import { extract, parse, renderHTML, sortIssues, validate } from 'xtxt-js';

/** Injected by esbuild from assets/chart-runtime.js — see esbuild.config.mjs. */
declare const __CHART_RUNTIME__: string;
const CHART_RUNTIME = __CHART_RUNTIME__;

import { blockRanges, headerCells } from './blocks';
import { isUnder } from './paths';

const run = promisify(execFile);

const SELECTOR: vscode.DocumentSelector = { language: 'xtxt' };

export function activate(context: vscode.ExtensionContext): void {
  const previews = new PreviewManager(context);
  const diagnostics = vscode.languages.createDiagnosticCollection('xtxt');

  context.subscriptions.push(
    diagnostics,
    vscode.commands.registerCommand('xtxt.showPreview', () => previews.show()),
    vscode.commands.registerCommand('xtxt.pasteImage', () => pasteImage()),
    vscode.commands.registerCommand('xtxt.chartTable',
      (uri?: vscode.Uri, line?: number) => chartTable(uri, line)),

    // Re-render as you type, not only on save: a preview that lags behind the
    // buffer is worse than no preview, because you stop trusting it.
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.languageId !== 'xtxt') return;
      previews.update(e.document);
      refreshDiagnostics(e.document, diagnostics);
    }),
    vscode.window.onDidChangeActiveTextEditor((editor) => {
      if (editor?.document.languageId === 'xtxt') previews.update(editor.document);
    }),

    vscode.workspace.onDidOpenTextDocument((doc) => refreshDiagnostics(doc, diagnostics)),
    // Stale squiggles in the Problems panel outlive the editor otherwise.
    vscode.workspace.onDidCloseTextDocument((doc) => diagnostics.delete(doc.uri)),

    vscode.languages.registerDocumentSymbolProvider(SELECTOR, new SymbolProvider()),
    vscode.languages.registerFoldingRangeProvider(SELECTOR, new FoldingProvider()),
    vscode.languages.registerCodeLensProvider(SELECTOR, new ChartLensProvider()),

    // Cmd+V with an image on the clipboard, handled where people expect it.
    vscode.languages.registerDocumentPasteEditProvider(
      { language: 'xtxt' },
      new ImagePasteProvider(),
      { providedPasteEditKinds: [vscode.DocumentDropOrPasteEditKind.Empty.append('xtxt', 'image')],
        pasteMimeTypes: ['image/png', 'image/jpeg', 'image/gif', 'image/webp'] },
    ),
  );

  // Documents restored from the last session are already open by the time we
  // activate, and would otherwise show nothing until their first keystroke.
  for (const doc of vscode.workspace.textDocuments) refreshDiagnostics(doc, diagnostics);
}

export function deactivate(): void {
  /* Panels are disposed through context.subscriptions. */
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

const SEVERITY: Record<string, vscode.DiagnosticSeverity> = {
  error: vscode.DiagnosticSeverity.Error,
  warning: vscode.DiagnosticSeverity.Warning,
};

/**
 * Surfaces what `xtxt validate` already reports, in the editor. The parser
 * recovers from everything, so this never throws and always leaves the
 * collection in a defined state — including empty, which clears old squiggles.
 */
function refreshDiagnostics(
  document: vscode.TextDocument,
  collection: vscode.DiagnosticCollection,
): void {
  if (document.languageId !== 'xtxt') return;

  const res = parse(document.getText());
  const issues = sortIssues([...res.issues, ...validate(res.doc)]);

  collection.set(document.uri, issues.map((issue) => {
    const diagnostic = new vscode.Diagnostic(
      lineRange(document, issue.line),
      issue.message,
      SEVERITY[issue.severity] ?? vscode.DiagnosticSeverity.Warning,
    );
    diagnostic.source = 'xtxt';
    return diagnostic;
  }));
}

/**
 * The range covering one 1-based source line, trimmed of indentation so the
 * squiggle sits under the text rather than the whitespace before it.
 */
function lineRange(document: vscode.TextDocument, line: number): vscode.Range {
  // An issue can point one past the end — an unclosed block reported at EOF.
  const index = Math.min(Math.max(line - 1, 0), Math.max(document.lineCount - 1, 0));
  const text = document.lineAt(index);
  return text.isEmptyOrWhitespace
    ? text.range
    : new vscode.Range(index, text.firstNonWhitespaceCharacterIndex, index, text.text.length);
}

// ---------------------------------------------------------------------------
// Outline and folding
// ---------------------------------------------------------------------------

/**
 * Headings nested by level, with record blocks hung under the heading they
 * fall in. Records are the point of the format, so an outline that showed only
 * headings would hide exactly what makes an XTXT document worth navigating.
 */
class SymbolProvider implements vscode.DocumentSymbolProvider {
  provideDocumentSymbols(document: vscode.TextDocument): vscode.DocumentSymbol[] {
    const { doc } = parse(document.getText());
    const data = extract(doc);
    const extent = new Map(blockRanges(lines(document)).map((b) => [b.start, b]));

    const roots: vscode.DocumentSymbol[] = [];
    const stack: Array<{ level: number; symbol: vscode.DocumentSymbol }> = [];

    const push = (symbol: vscode.DocumentSymbol, level: number) => {
      while (stack.length && stack[stack.length - 1].level >= level) stack.pop();
      const parent = stack[stack.length - 1];
      (parent ? parent.symbol.children : roots).push(symbol);
      // A parent's range must contain its children, or the breadcrumb bar
      // silently drops them.
      for (const entry of stack) {
        entry.symbol.range = entry.symbol.range.union(symbol.range);
      }
      return symbol;
    };

    for (const entry of merge(data.outline, data.blocks)) {
      if ('level' in entry) {
        const range = lineRange(document, entry.line);
        const symbol = new vscode.DocumentSymbol(
          entry.text || 'Untitled', '', vscode.SymbolKind.String, range, range);
        stack.push({ level: entry.level, symbol: push(symbol, entry.level) });
      } else {
        const selection = lineRange(document, entry.line);
        const block = extent.get(entry.line - 1);
        const range = block
          ? new vscode.Range(block.start, 0, block.end, document.lineAt(block.end).text.length)
          : selection;
        push(
          new vscode.DocumentSymbol(
            title(entry), entry.type, vscode.SymbolKind.Object, range, selection),
          // Blocks never own children, so they sit below any heading.
          Number.MAX_SAFE_INTEGER,
        );
      }
    }

    return roots;
  }
}

/** Headings and blocks in source order. */
function merge(
  outline: ReturnType<typeof extract>['outline'],
  blocks: ReturnType<typeof extract>['blocks'],
): Array<(typeof outline)[number] | (typeof blocks)[number]> {
  return [...outline, ...blocks].sort((a, b) => a.line - b.line);
}

/** A record's own Title if it carries one, else just its type. */
function title(block: ReturnType<typeof extract>['blocks'][number]): string {
  const fields = block.fields ?? {};
  const named = fields.title ?? fields.Title ?? fields.name ?? fields.Name;
  return named ? `${block.type}: ${named}` : block.type;
}

class FoldingProvider implements vscode.FoldingRangeProvider {
  provideFoldingRanges(document: vscode.TextDocument): vscode.FoldingRange[] {
    const ranges = blockRanges(lines(document))
      .filter((b) => b.end > b.start)
      .map((b) => new vscode.FoldingRange(b.start, b.end));

    // Headings fold to the line before the next heading of the same or higher
    // rank — the behaviour every outline-shaped format has.
    const headings = extract(parse(document.getText()).doc).outline;
    headings.forEach((heading, i) => {
      const next = headings.slice(i + 1).find((h) => h.level <= heading.level);
      const end = (next ? next.line - 1 : document.lineCount) - 1;
      if (end > heading.line - 1) ranges.push(new vscode.FoldingRange(heading.line - 1, end));
    });

    return ranges;
  }
}

/**
 * Read the lines through the document itself rather than splitting its text:
 * the block extents are used to index back into it with `lineAt`, and any
 * disagreement about what ends a line — a lone CR, most plausibly — would
 * throw out of the provider rather than degrade.
 */
function lines(document: vscode.TextDocument): string[] {
  return Array.from({ length: document.lineCount }, (_, i) => document.lineAt(i).text);
}

// ---------------------------------------------------------------------------
// Charting a table
// ---------------------------------------------------------------------------

const CHART_TYPES = ['bar', 'line', 'area', 'stacked', 'pie'];

/** An offer to chart, on the line of every `@table`. */
class ChartLensProvider implements vscode.CodeLensProvider {
  provideCodeLenses(document: vscode.TextDocument): vscode.CodeLens[] {
    return blockRanges(lines(document))
      .filter((b) => b.name === 'table')
      .map((b) => new vscode.CodeLens(new vscode.Range(b.start, 0, b.start, 0), {
        title: document.lineAt(b.start).text.includes('chart=')
          ? '$(graph) Change chart' : '$(graph) Chart this table',
        command: 'xtxt.chartTable',
        arguments: [document.uri, b.start],
      }));
  }
}

/**
 * Writes chart arguments onto a `@table`. Three QuickPicks rather than a form:
 * the whole decision is a type and two column choices, and a webview for that
 * would be more chrome than question.
 */
async function chartTable(uri?: vscode.Uri, startLine?: number): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  const document = uri
    ? await vscode.workspace.openTextDocument(uri)
    : editor?.document;
  if (!document || document.languageId !== 'xtxt') {
    void vscode.window.showInformationMessage('Open an .xtxt file first.');
    return;
  }

  const tables = blockRanges(lines(document)).filter((b) => b.name === 'table');
  // Invoked from the palette rather than the lens: chart the table the cursor
  // is in, which is the only one the author can mean.
  const cursor = editor?.selection.active.line ?? 0;
  const block = startLine !== undefined
    ? tables.find((b) => b.start === startLine)
    : tables.find((b) => cursor >= b.start && cursor <= b.end);
  if (!block) {
    void vscode.window.showInformationMessage('Put the cursor inside an @table block.');
    return;
  }

  const columns = headerCells(lines(document), block);
  if (columns.length === 0) {
    void vscode.window.showWarningMessage('That table has no header row to chart.');
    return;
  }

  const type = await vscode.window.showQuickPick(CHART_TYPES,
    { title: 'Chart type', placeHolder: 'How should the rows be drawn?' });
  if (!type) return;

  const x = await vscode.window.showQuickPick(columns,
    { title: 'Labels', placeHolder: 'Which column names each row?' });
  if (!x) return;

  const y = await vscode.window.showQuickPick(columns.filter((c) => c !== x),
    { title: 'Values', placeHolder: 'Which columns hold the numbers?', canPickMany: true });
  if (!y || y.length === 0) return;

  const edit = new vscode.WorkspaceEdit();
  edit.replace(document.uri, document.lineAt(block.start).range,
    tableLine(document, block, { chart: type, x, y: y.join(', ') }));
  await vscode.workspace.applyEdit(edit);
}

/**
 * Rebuilds the `@table(…)` line with new chart arguments, keeping any the
 * picker does not ask about — a title or unit the author wrote by hand should
 * survive being re-charted.
 */
function tableLine(
  document: vscode.TextDocument,
  block: { start: number },
  set: Record<string, string>,
): string {
  const node = parse(document.getText()).doc.nodes
    .find((n) => n.line === block.start + 1 && n.name === 'table');

  const args: Array<[string, string]> = [];
  for (const arg of node?.args ?? []) {
    if (arg.key && !(arg.key in set)) args.push([arg.key, arg.value]);
  }
  for (const [key, value] of Object.entries(set)) args.push([key, value]);

  const indent = document.lineAt(block.start).text.match(/^\s*/)?.[0] ?? '';
  return `${indent}@table(${args.map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', ')})`;
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

class PreviewManager {
  private panel: vscode.WebviewPanel | undefined;

  constructor(private readonly context: vscode.ExtensionContext) {}

  show(): void {
    const editor = vscode.window.activeTextEditor;
    if (!editor || editor.document.languageId !== 'xtxt') {
      void vscode.window.showInformationMessage('Open an .xtxt file first.');
      return;
    }

    if (!this.panel) {
      this.panel = vscode.window.createWebviewPanel(
        'xtxt.preview',
        `Preview ${basename(editor.document.uri)}`,
        { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
        {
          // Scripts are on so charts can be read and filtered, but the CSP
          // below admits exactly one script: ours, carrying a nonce minted per
          // render. A <script> arriving through @raw(format="html") cannot
          // carry it, so enabling this does not hand a document the ability to
          // run code. enableCommandUris stays off, so command: links stay dead.
          enableScripts: true,
          // Images referenced relatively must be loadable, so grant the
          // document's own folder and the workspace — and nothing else.
          localResourceRoots: resourceRoots(editor.document.uri),
        },
      );
      this.panel.onDidDispose(() => {
        this.panel = undefined;
      }, null, this.context.subscriptions);
    }

    this.update(editor.document);
    this.panel.reveal(vscode.ViewColumn.Beside, true);
  }

  update(document: vscode.TextDocument): void {
    if (!this.panel) return;
    // The preview follows whichever .xtxt document is active.
    this.panel.title = `Preview ${basename(document.uri)}`;

    const source = document.getText();
    const res = parse(source);
    const issues = [...res.issues, ...validate(res.doc)].sort((a, b) => a.line - b.line);
    const data = extract(res.doc);

    this.panel.webview.html = previewHTML(
      this.panel.webview,
      document.uri,
      renderHTML(res.doc),
      issues,
      `${data.outline.length} headings · ${data.tasks.length} tasks · ` +
        `${data.blocks.length} records · ${data.words} words`,
    );
  }
}

function basename(uri: vscode.Uri): string {
  return uri.path.split('/').pop() ?? 'document';
}

function resourceRoots(uri: vscode.Uri): vscode.Uri[] {
  const roots = [vscode.Uri.joinPath(uri, '..')];
  const folder = vscode.workspace.getWorkspaceFolder(uri);
  if (folder) roots.push(folder.uri);
  return roots;
}

/** The first of `roots` that `target` is, or sits underneath. */
function containingRoot(roots: vscode.Uri[], target: vscode.Uri): vscode.Uri | undefined {
  return roots.find((root) =>
    root.scheme === target.scheme &&
    root.authority === target.authority &&
    isUnder(root.path, target.path));
}

/**
 * Whether writing to `target` stays inside one of `roots` — in fact, not just
 * on paper.
 *
 * Comparing paths is not enough by itself. A repository can commit a symlink,
 * so `img` can satisfy every lexical check and still land in `~/.ssh`; the
 * filesystem follows it on write. Walking the segments and refusing any link
 * is what makes the check mean what it says.
 */
async function writable(roots: vscode.Uri[], target: vscode.Uri): Promise<boolean> {
  const root = containingRoot(roots, target);
  if (!root) return false;

  let at = root;
  for (const segment of target.path.slice(root.path.length).split('/').filter(Boolean)) {
    at = vscode.Uri.joinPath(at, segment);
    let stat: vscode.FileStat;
    try {
      stat = await vscode.workspace.fs.stat(at);
    } catch {
      return true; // Nothing here yet, so there is no link to follow.
    }
    if (stat.type & vscode.FileType.SymbolicLink) return false;
  }
  return true;
}

/** Rewrite relative media sources so the webview can actually load them. */
function resolveMedia(html: string, webview: vscode.Webview, document: vscode.Uri): string {
  return html.replace(/(<(?:img|video|audio|source)\b[^>]*?\bsrc=")([^"]+)(")/g, (all, head, src, tail) => {
    if (/^(https?:|data:|blob:|vscode-)/.test(src)) return all;
    const target = vscode.Uri.joinPath(document, '..', src);
    return head + webview.asWebviewUri(target).toString() + tail;
  });
}

function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&#34;').replace(/'/g, '&#39;');
}

function previewHTML(
  webview: vscode.Webview,
  document: vscode.Uri,
  body: string,
  issues: Array<{ severity: string; line: number; message: string }>,
  summary: string,
): string {
  const problems = issues.length
    ? `<ul class="issues">${issues
        .map((i) => `<li class="${i.severity}">${i.line}: ${escapeHTML(i.message)}</li>`)
        .join('')}</ul>`
    : '';

  // One nonce per render. Only the chart runtime carries it, so it is the only
  // script that can run — script-src does not fall back to default-src here,
  // it is stated explicitly so nothing else is ever admitted.
  const nonce = randomBytes(16).toString('base64');
  const csp =
    `default-src 'none'; img-src ${webview.cspSource} https: data:; ` +
    `media-src ${webview.cspSource} https: data:; style-src 'unsafe-inline'; ` +
    `script-src 'nonce-${nonce}';`;

  // Only ship the runtime when there is something for it to upgrade.
  const runtime = body.includes('data-xtxt-chart')
    ? `<script nonce="${nonce}">${CHART_RUNTIME}</script>`
    : '';

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<style>${PREVIEW_CSS}</style>
</head>
<body>
<div class="summary">${escapeHTML(summary)}</div>
${problems}
<main class="xtxt">
${resolveMedia(body, webview, document)}
</main>
${runtime}
</body>
</html>`;
}

/** Styled with VS Code's own theme variables, so it matches the editor. */
const PREVIEW_CSS = `
body { font-family: var(--vscode-font-family); color: var(--vscode-foreground);
       background: var(--vscode-editor-background); padding: 0 1.5rem 4rem; line-height: 1.6; }
.summary { position: sticky; top: 0; padding: .5rem 0; margin-bottom: .5rem;
           background: var(--vscode-editor-background); color: var(--vscode-descriptionForeground);
           font-size: .8rem; border-bottom: 1px solid var(--vscode-panel-border); }
.issues { list-style: none; padding: .5rem .8rem; margin: 0 0 1rem;
          background: var(--vscode-textBlockQuote-background); font-size: .8rem;
          font-family: var(--vscode-editor-font-family); border-radius: 3px; }
.issues .warning { color: var(--vscode-editorWarning-foreground); }
.issues .error { color: var(--vscode-editorError-foreground); }
.xtxt { max-width: 46rem; }
h1, h2, h3, h4 { line-height: 1.25; margin: 1.8em 0 .5em; }
h1 { font-size: 1.9rem; margin-top: .4em; }
a { color: var(--vscode-textLink-foreground); }
blockquote { border-left: 3px solid var(--vscode-panel-border); margin-left: 0;
             padding-left: 1rem; color: var(--vscode-descriptionForeground); }
pre { background: var(--vscode-textCodeBlock-background); padding: .8rem;
      border-radius: 4px; overflow-x: auto; }
pre, code { font-family: var(--vscode-editor-font-family); font-size: .875rem; }
table { border-collapse: collapse; width: 100%; margin: 1.5em 0; font-size: .92rem; }
th, td { border-bottom: 1px solid var(--vscode-panel-border); padding: .45em .7em; text-align: left; }
figure { margin: 1.8em 0; text-align: center; }
figure img, figure video { max-width: 100%; border-radius: 4px; }
figcaption { color: var(--vscode-descriptionForeground); font-size: .85rem; margin-top: .5em; }
ul.checklist { list-style: none; padding-left: .3em; }
.record { border: 1px solid var(--vscode-panel-border);
          border-left: 3px solid var(--vscode-textLink-foreground); border-radius: 4px;
          padding: .8rem 1rem; margin: 1.5em 0; }
.record-type { margin: 0 0 .5em; font-size: .7rem; letter-spacing: .09em;
               text-transform: uppercase; color: var(--vscode-descriptionForeground); }
.record dl { display: grid; grid-template-columns: minmax(4.5rem, max-content) 1fr;
             gap: .3em .9em; margin: 0; font-size: .94rem; }
.record dt { color: var(--vscode-descriptionForeground); font-size: .82rem; }
.record dd { margin: 0; white-space: pre-wrap; }
.unknown { border: 1px dashed var(--vscode-panel-border); border-radius: 4px;
           padding: .6em .9em; margin: 1.4em 0; white-space: pre-wrap;
           color: var(--vscode-descriptionForeground);
           font-family: var(--vscode-editor-font-family); font-size: .8rem; }
.xtxt-chart { display: block; max-width: 100%; }
.xtxt-chart .c-label { fill: var(--vscode-foreground); font-size: 12px; }
.xtxt-chart .c-value, .xtxt-chart .c-axis { fill: var(--vscode-descriptionForeground); }
.xtxt-chart .c-grid { stroke: var(--vscode-panel-border); }
.xtxt-chart .c-inbar { fill: #fff; font-size: 11px; font-weight: 600; }
.chart-data summary { color: var(--vscode-descriptionForeground); cursor: pointer; font-size: .85rem; }
main { --chart-1: #2a78d6; --chart-2: #eb6834; --chart-3: #1baf7a;
       --chart-surface: var(--vscode-editor-background); }
@media (prefers-color-scheme: dark) {
  main { --chart-1: #3987e5; --chart-2: #d95926; --chart-3: #199e70; }
}
.footnotes { margin-top: 3em; padding-top: 1em; border-top: 1px solid var(--vscode-panel-border);
             color: var(--vscode-descriptionForeground); font-size: .88rem; }
.chart-warning { color: var(--vscode-editorWarning-foreground); font-size: .82rem;
                 margin: .3em 0 1.2em; }
.xtxt-chart-readout { min-height: 1.3em; margin: .2em 0 0; font-size: .85rem;
                      color: var(--vscode-descriptionForeground);
                      font-variant-numeric: tabular-nums; }
.xtxt-chart-legend { display: flex; flex-wrap: wrap; gap: .4em; margin-top: .4em; }
.xtxt-chart-toggle { font: inherit; font-size: .78rem; cursor: pointer;
                     padding: .15em .6em; border-radius: 999px;
                     color: var(--vscode-foreground);
                     background: var(--vscode-editor-background);
                     border: 1px solid var(--vscode-panel-border); }
.xtxt-chart-toggle[aria-pressed="false"] { opacity: .45; text-decoration: line-through; }
.tok-com { color: var(--vscode-editorLineNumber-foreground); font-style: italic; }
.tok-str { color: var(--vscode-debugTokenExpression-string, #0a7d55); }
.tok-num { color: var(--vscode-debugTokenExpression-number, #9a5b00); }
.tok-kw  { color: var(--vscode-symbolIcon-keywordForeground, #8250c8); font-weight: 600; }
`;

// ---------------------------------------------------------------------------
// Image pasting
// ---------------------------------------------------------------------------

/**
 * Handles Cmd+V when the clipboard holds an image. VS Code hands us the bytes
 * directly, so no platform-specific clipboard reading is needed here — that is
 * only required for the standalone command below.
 */
class ImagePasteProvider implements vscode.DocumentPasteEditProvider {
  async provideDocumentPasteEdits(
    document: vscode.TextDocument,
    _ranges: readonly vscode.Range[],
    dataTransfer: vscode.DataTransfer,
    _context: vscode.DocumentPasteEditContext,
    token: vscode.CancellationToken,
  ): Promise<vscode.DocumentPasteEdit[] | undefined> {
    for (const mime of ['image/png', 'image/jpeg', 'image/gif', 'image/webp']) {
      const item = dataTransfer.get(mime);
      if (!item) continue;
      const file = item.asFile();
      if (!file) continue;

      const bytes = await file.data();
      if (token.isCancellationRequested) return undefined;

      const directive = await storeImage(document, Buffer.from(bytes), mime);
      const edit = new vscode.DocumentPasteEdit(
        directive,
        'Insert as XTXT image',
        vscode.DocumentDropOrPasteEditKind.Empty.append('xtxt', 'image'),
      );
      return [edit];
    }
    return undefined;
  }
}

const EXTENSION_FOR_MIME: Record<string, string> = {
  'image/png': '.png',
  'image/jpeg': '.jpg',
  'image/gif': '.gif',
  'image/webp': '.webp',
  'image/svg+xml': '.svg',
};

/**
 * Write the image where the settings say, and return the directive text.
 * Saving beside the document keeps the file small and the diff readable;
 * embedding makes it self-contained at about 33% size overhead.
 */
async function storeImage(
  document: vscode.TextDocument,
  bytes: Buffer,
  mime: string,
): Promise<string> {
  const config = vscode.workspace.getConfiguration('xtxt');

  if (config.get<boolean>('paste.embed', false)) {
    return imageDirective(`data:${mime};base64,${bytes.toString('base64')}`);
  }

  const ext = EXTENSION_FOR_MIME[mime] ?? '.png';
  const stem = (basename(document.uri).replace(/\.xtxt$/, '') || 'image');
  const here = vscode.Uri.joinPath(document.uri, '..');

  let folder = config.get<string>('paste.folder', '').trim();
  let dir = folder ? vscode.Uri.joinPath(document.uri, '..', folder) : here;

  // The setting is window-scoped, so a repository can set it in its own
  // .vscode/settings.json. Opening someone else's project must not let it
  // choose where this extension creates directories and writes files.
  if (folder && !await writable(resourceRoots(document.uri), dir)) {
    void vscode.window.showWarningMessage(
      `Ignoring xtxt.paste.folder "${folder}": it points outside the workspace.`);
    folder = '';
    dir = here;
  }

  if (folder) {
    await vscode.workspace.fs.createDirectory(dir);
  }

  const name = await uniqueName(dir, stem, ext);
  await vscode.workspace.fs.writeFile(vscode.Uri.joinPath(dir, name), bytes);
  return imageDirective(folder ? `${folder}/${name}` : name);
}

async function uniqueName(dir: vscode.Uri, stem: string, ext: string): Promise<string> {
  let existing: string[] = [];
  try {
    existing = (await vscode.workspace.fs.readDirectory(dir)).map(([n]) => n);
  } catch {
    // A folder that does not exist yet simply has nothing in it.
  }
  for (let i = 1; i < 10000; i++) {
    const name = `${stem}-${i}${ext}`;
    if (!existing.includes(name)) return name;
  }
  throw new Error(`could not find an unused filename in ${dir.fsPath}`);
}

function imageDirective(src: string): string {
  return `@image(src=${JSON.stringify(src)})\n`;
}

/**
 * The explicit command, for when the clipboard holds an image that VS Code's
 * paste pipeline does not surface — a screenshot on Linux, most often.
 */
async function pasteImage(): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  if (!editor || editor.document.languageId !== 'xtxt') {
    void vscode.window.showInformationMessage('Open an .xtxt file first.');
    return;
  }

  let bytes: Buffer;
  try {
    bytes = await readClipboardImage();
  } catch (err) {
    void vscode.window.showWarningMessage(
      `No image on the clipboard: ${err instanceof Error ? err.message : String(err)}`);
    return;
  }

  const directive = await storeImage(editor.document, bytes, 'image/png');
  await editor.edit((builder) => builder.insert(editor.selection.active, directive));
}

/** Reads a PNG from the OS clipboard, using whatever the platform ships. */
async function readClipboardImage(): Promise<Buffer> {
  const tmp = vscode.Uri.joinPath(
    vscode.Uri.file(tmpdir()), `xtxt-paste-${Date.now()}.png`);

  if (process.platform === 'darwin') {
    const script = `
      set outFile to POSIX file ${JSON.stringify(tmp.fsPath)}
      try
        set imageData to the clipboard as «class PNGf»
      on error
        return "no-image"
      end try
      set fh to open for access outFile with write permission
      set eof fh to 0
      write imageData to fh
      close access fh
      return "ok"`;
    const { stdout } = await run('osascript', ['-e', script]);
    if (stdout.trim() !== 'ok') throw new Error('the clipboard holds no image');
  } else if (process.platform === 'win32') {
    // A single-quoted PowerShell string is literal; a double-quoted one would
    // expand `$(…)` out of the temp path.
    const literal = `'${tmp.fsPath.replace(/'/g, "''")}'`;
    await run('powershell', ['-NoProfile', '-STA', '-Command', `
      Add-Type -AssemblyName System.Windows.Forms
      $img = [Windows.Forms.Clipboard]::GetImage()
      if ($img -eq $null) { exit 1 }
      $img.Save(${literal}, [System.Drawing.Imaging.ImageFormat]::Png)`]);
  } else {
    // The path goes in as an argument and is read back as "$0", so the shell
    // never parses it as script however it is spelled.
    const { stdout } = await run('sh', ['-c',
      'wl-paste --type image/png > "$0" 2>/dev/null || ' +
      'xclip -selection clipboard -t image/png -o > "$0" 2>/dev/null || ' +
      'echo missing',
      tmp.fsPath]);
    if (stdout.trim() === 'missing') {
      throw new Error('install wl-clipboard or xclip to paste images');
    }
  }

  const data = await vscode.workspace.fs.readFile(tmp);
  await vscode.workspace.fs.delete(tmp).then(undefined, () => undefined);
  if (data.byteLength === 0) throw new Error('the clipboard holds no image');
  return Buffer.from(data);
}
