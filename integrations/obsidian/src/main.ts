/**
 * Obsidian plugin for the XTXT document format.
 *
 * Registers `.xtxt` as a first-class file type: a vault can hold documents that
 * keep their images, tables, charts and typed records inside the file, and
 * still open in any text editor.
 *
 * The plugin renders with the same JavaScript SDK the browser demo uses, so
 * what a vault shows and what `xtxt render` produces cannot drift apart.
 */

import { Notice, Plugin, TextFileView, WorkspaceLeaf, normalizePath } from 'obsidian';

import { extract, parse, renderHTML, validate } from 'xtxt-js';

export const VIEW_TYPE_XTXT = 'xtxt-view';

type Mode = 'preview' | 'source';

interface XtxtSettings {
  /** Which mode a document opens in. */
  defaultMode: Mode;
  /** Show parser warnings above the document. */
  showIssues: boolean;
  /**
   * Embed pasted images as data: URIs instead of saving them to the vault.
   * Self-contained, but roughly 33% larger and it makes diffs unreadable.
   */
  embedPastedImages: boolean;
  /**
   * Folder for pasted images, relative to the document. A notes folder fills
   * with screenshots quickly, so media goes into a subfolder by default.
   * Empty writes them beside the document.
   */
  attachmentFolder: string;
}

const DEFAULT_SETTINGS: XtxtSettings = {
  defaultMode: 'preview',
  showIssues: true,
  embedPastedImages: false,
  attachmentFolder: 'assets',
};

const IMAGE_EXTENSIONS: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
  'image/svg+xml': 'svg',
};

/**
 * A view over one .xtxt file. It extends TextFileView so Obsidian handles
 * loading, saving, renaming and dirty tracking; this class only decides what
 * the pane looks like.
 */
export class XtxtView extends TextFileView {
  // `data` is inherited from TextFileView and is the file's text; redeclaring
  // it here would shadow the base class's own bookkeeping.
  private mode: Mode;
  private readonly plugin: XtxtPlugin;

  private container!: HTMLElement;
  private issuesEl!: HTMLElement;
  private bodyEl!: HTMLElement;
  private editorEl!: HTMLTextAreaElement;

  constructor(leaf: WorkspaceLeaf, plugin: XtxtPlugin) {
    super(leaf);
    this.plugin = plugin;
    this.mode = plugin.settings.defaultMode;
  }

  getViewType(): string {
    return VIEW_TYPE_XTXT;
  }

  getDisplayText(): string {
    return this.file?.basename ?? 'XTXT';
  }

  getIcon(): string {
    return 'file-text';
  }

  /** TextFileView contract: the current text of the file. */
  getViewData(): string {
    return this.mode === 'source' ? this.editorEl.value : this.data;
  }

  /** TextFileView contract: replace the text, e.g. after an external change. */
  setViewData(data: string, clear: boolean): void {
    this.data = data;
    if (clear) this.clear();
    this.render();
  }

  clear(): void {
    this.data = '';
  }

  async onOpen(): Promise<void> {
    this.container = this.contentEl.createDiv({ cls: 'xtxt-view' });
    this.issuesEl = this.container.createDiv({ cls: 'xtxt-issues' });
    this.bodyEl = this.container.createDiv({ cls: 'xtxt-body' });
    this.editorEl = this.container.createEl('textarea', { cls: 'xtxt-source' });
    this.editorEl.spellcheck = false;
    this.editorEl.addEventListener('input', () => {
      this.data = this.editorEl.value;
      this.requestSave();
    });
    this.editorEl.addEventListener('paste', (event) => void this.onPaste(event));

    this.addAction(
      'eye',
      'Toggle preview and source',
      () => this.toggleMode(),
    );
    this.render();
  }

  /**
   * Paste an image at the cursor. Obsidian hands us the bytes, so the image is
   * written into the vault and the directive inserted where you were typing —
   * and because the preview renders the same HTML the CLI does, switching back
   * shows the picture in place.
   */
  private async onPaste(event: ClipboardEvent): Promise<void> {
    const items = Array.from(event.clipboardData?.items ?? []);
    const image = items.find((i) => i.kind === 'file' && i.type.startsWith('image/'));
    if (!image) return;             // ordinary text paste: leave it alone
    event.preventDefault();

    const file = image.getAsFile();
    if (!file) return;

    try {
      const directive = await this.plugin.storeImage(
        await file.arrayBuffer(), file.type, this.file?.parent?.path ?? '',
        this.file?.basename ?? 'image');
      this.insertAtCursor(directive);
    } catch (err) {
      new Notice(`Could not paste the image: ${err instanceof Error ? err.message : err}`);
    }
  }

  private insertAtCursor(text: string): void {
    const start = this.editorEl.selectionStart ?? this.editorEl.value.length;
    const end = this.editorEl.selectionEnd ?? start;
    const value = this.editorEl.value;
    this.editorEl.value = value.slice(0, start) + text + value.slice(end);
    this.editorEl.selectionStart = this.editorEl.selectionEnd = start + text.length;
    this.data = this.editorEl.value;
    this.requestSave();
    this.render();
  }

  private toggleMode(): void {
    // Take the edited text before switching, or typing in source mode is lost.
    if (this.mode === 'source') this.data = this.editorEl.value;
    this.mode = this.mode === 'preview' ? 'source' : 'preview';
    this.render();
  }

  private render(): void {
    if (!this.container) return;

    const source = this.data;
    const res = parse(source);
    const issues = [...res.issues, ...validate(res.doc)].sort((a, b) => a.line - b.line);

    this.issuesEl.empty();
    const showable = this.plugin.settings.showIssues ? issues : issues.filter(
      (i) => i.severity === 'error');
    if (showable.length > 0) {
      const list = this.issuesEl.createEl('ul');
      for (const issue of showable) {
        const item = list.createEl('li', { cls: `xtxt-issue xtxt-${issue.severity}` });
        item.setText(`${issue.line}: ${issue.severity}: ${issue.message}`);
      }
    }

    this.editorEl.toggleClass('xtxt-hidden', this.mode !== 'source');
    this.bodyEl.toggleClass('xtxt-hidden', this.mode === 'source');

    if (this.mode === 'source') {
      if (this.editorEl.value !== source) this.editorEl.value = source;
      return;
    }

    this.bodyEl.empty();
    // The SDK's output is built from the document's own text, all of which is
    // escaped on the way through; there is no path for unescaped input here.
    this.bodyEl.innerHTML = renderHTML(res.doc);
    this.resolveInternalImages();
  }

  /**
   * Point relative image sources at the vault, so `@image(src="cat.png")` next
   * to the document resolves the way a reader would expect.
   */
  private resolveInternalImages(): void {
    const dir = this.file?.parent?.path ?? '';
    this.bodyEl.querySelectorAll('img, video, audio').forEach((el) => {
      const media = el as HTMLImageElement;
      const src = media.getAttribute('src');
      if (!src || /^(https?:|data:|blob:|\/)/.test(src)) return;
      const target = this.app.vault.getFileByPath(normalizePath(dir ? `${dir}/${src}` : src));
      if (target) media.setAttribute('src', this.app.vault.getResourcePath(target));
    });
  }
}

export default class XtxtPlugin extends Plugin {
  settings: XtxtSettings = DEFAULT_SETTINGS;

  async onload(): Promise<void> {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());

    this.registerView(VIEW_TYPE_XTXT, (leaf) => new XtxtView(leaf, this));
    this.registerExtensions(['xtxt'], VIEW_TYPE_XTXT);

    this.addCommand({
      id: 'create-xtxt-note',
      name: 'Create new XTXT note',
      callback: () => void this.createNote(),
    });

    this.addCommand({
      id: 'summarise-xtxt-structure',
      name: 'Show structure of the current document',
      checkCallback: (checking) => {
        const view = this.app.workspace.getActiveViewOfType(XtxtView);
        if (!view) return false;
        if (!checking) this.showStructure(view);
        return true;
      },
    });
  }

  /**
   * Write a pasted image and return the directive that references it.
   * Saving into the vault keeps the document small and the diff readable;
   * embedding makes it self-contained at about 33% size overhead.
   */
  async storeImage(bytes: ArrayBuffer, mime: string, docDir: string,
                   stem: string): Promise<string> {
    if (this.settings.embedPastedImages) {
      const base64 = btoa(String.fromCharCode(...new Uint8Array(bytes)));
      return `@image(src="data:${mime};base64,${base64}")\n`;
    }

    const ext = IMAGE_EXTENSIONS[mime] ?? 'png';
    // Relative to the document, matching the CLI and the VS Code extension.
    // A vault-absolute folder would put one project's screenshots beside
    // another's, and the same setting would mean different things in each tool.
    const folder = this.settings.attachmentFolder.trim().replace(/^\/+|\/+$/g, '');
    const dir = folder ? (docDir ? `${docDir}/${folder}` : folder) : docDir;
    if (folder && !this.app.vault.getAbstractFileByPath(normalizePath(dir))) {
      await this.app.vault.createFolder(normalizePath(dir));
    }

    let name = '';
    for (let i = 1; i < 10000; i++) {
      const candidate = normalizePath(dir ? `${dir}/${stem}-${i}.${ext}` : `${stem}-${i}.${ext}`);
      if (!this.app.vault.getAbstractFileByPath(candidate)) {
        name = candidate;
        break;
      }
    }
    if (!name) throw new Error('could not find an unused filename');

    await this.app.vault.createBinary(name, bytes);
    // Always reference it relative to the document, so the file resolves when
    // the folder is opened outside the vault by any other XTXT reader.
    const relative = docDir && name.startsWith(`${docDir}/`)
      ? name.slice(docDir.length + 1)
      : name;
    return `@image(src="${relative}")\n`;
  }

  /** A short report of what an agent would receive from `xtxt extract`. */
  private showStructure(view: XtxtView): void {
    const data = extract(parse(view.getViewData()).doc);
    const types = [...new Set(data.blocks.map((b) => b.type))];
    new Notice(
      [
        `${data.outline.length} headings`,
        `${data.tasks.length} tasks (${data.tasks.filter((t) => !t.done).length} open)`,
        `${data.blocks.length} records${types.length ? ` — ${types.join(', ')}` : ''}`,
        `${data.words} words`,
      ].join('\n'),
      8000,
    );
  }

  private async createNote(): Promise<void> {
    const folder = this.app.workspace.getActiveFile()?.parent?.path ?? '';
    const base = folder ? `${folder}/Untitled` : 'Untitled';
    let path = normalizePath(`${base}.xtxt`);
    for (let i = 2; this.app.vault.getAbstractFileByPath(path); i++) {
      path = normalizePath(`${base} ${i}.xtxt`);
    }

    const template = [
      '@metadata',
      'title = Untitled',
      `created = ${new Date().toISOString().slice(0, 10)}`,
      '@endmetadata',
      '',
      '# Untitled',
      '',
      'Write here. Structure a machine can read goes in a record:',
      '',
      '@task',
      'Title: Something to do',
      'Status: Open',
      '@endtask',
      '',
    ].join('\n');

    const file = await this.app.vault.create(path, template);
    await this.app.workspace.getLeaf(false).openFile(file);
  }

  onunload(): void {
    // Obsidian detaches views registered with registerView automatically.
  }

  async saveSettings(): Promise<void> {
    await this.saveData(this.settings);
  }
}
