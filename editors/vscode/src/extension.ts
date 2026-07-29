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
import { execFile } from 'node:child_process';
import { tmpdir } from 'node:os';
import { promisify } from 'node:util';
import * as vscode from 'vscode';

import { extract, parse, renderHTML, validate } from 'xtxt-js';

const run = promisify(execFile);

export function activate(context: vscode.ExtensionContext): void {
  const previews = new PreviewManager(context);

  context.subscriptions.push(
    vscode.commands.registerCommand('xtxt.showPreview', () => previews.show()),
    vscode.commands.registerCommand('xtxt.pasteImage', () => pasteImage()),

    // Re-render as you type, not only on save: a preview that lags behind the
    // buffer is worse than no preview, because you stop trusting it.
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.languageId === 'xtxt') previews.update(e.document);
    }),
    vscode.window.onDidChangeActiveTextEditor((editor) => {
      if (editor?.document.languageId === 'xtxt') previews.update(editor.document);
    }),

    // Cmd+V with an image on the clipboard, handled where people expect it.
    vscode.languages.registerDocumentPasteEditProvider(
      { language: 'xtxt' },
      new ImagePasteProvider(),
      { providedPasteEditKinds: [vscode.DocumentDropOrPasteEditKind.Empty.append('xtxt', 'image')],
        pasteMimeTypes: ['image/png', 'image/jpeg', 'image/gif', 'image/webp'] },
    ),
  );
}

export function deactivate(): void {
  /* Panels are disposed through context.subscriptions. */
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
          enableScripts: false,
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

  // No scripts in the webview at all, so the CSP can be maximally strict.
  const csp =
    `default-src 'none'; img-src ${webview.cspSource} https: data:; ` +
    `media-src ${webview.cspSource} https: data:; style-src 'unsafe-inline';`;

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
  const folder = config.get<string>('paste.folder', '').trim();
  const dir = folder
    ? vscode.Uri.joinPath(document.uri, '..', folder)
    : vscode.Uri.joinPath(document.uri, '..');

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
    await run('powershell', ['-NoProfile', '-STA', '-Command', `
      Add-Type -AssemblyName System.Windows.Forms
      $img = [Windows.Forms.Clipboard]::GetImage()
      if ($img -eq $null) { exit 1 }
      $img.Save(${JSON.stringify(tmp.fsPath)}, [System.Drawing.Imaging.ImageFormat]::Png)`]);
  } else {
    const { stdout } = await run('sh', ['-c',
      `wl-paste --type image/png > ${JSON.stringify(tmp.fsPath)} 2>/dev/null || ` +
      `xclip -selection clipboard -t image/png -o > ${JSON.stringify(tmp.fsPath)} 2>/dev/null || ` +
      `echo missing`]);
    if (stdout.trim() === 'missing') {
      throw new Error('install wl-clipboard or xclip to paste images');
    }
  }

  const data = await vscode.workspace.fs.readFile(tmp);
  await vscode.workspace.fs.delete(tmp).then(undefined, () => undefined);
  if (data.byteLength === 0) throw new Error('the clipboard holds no image');
  return Buffer.from(data);
}
