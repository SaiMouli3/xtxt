# XTXT for VS Code

Syntax highlighting, live preview and image pasting for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.

## Features

- **Live preview** — `Cmd+K V` (`Ctrl+K V`) opens a panel beside the editor
  that re-renders as you type, with records as cards, charts as inline SVG and
  images resolved relative to the document
- **Paste an image** — `Cmd+V` with an image on the clipboard saves it next to
  the document and inserts the `@image` directive. `Cmd+Alt+V` forces it when
  the platform does not surface the image to VS Code
- **Syntax highlighting**, with `@code(language="…")` blocks highlighted in
  their own language
- **Problems and squiggles** — everything `xtxt validate` reports, in the
  editor and the Problems panel as you type. Unknown directives are warnings,
  never errors
- **Outline** — headings *and* records, so `Cmd+Shift+O`, the breadcrumb bar
  and the Outline view jump straight to a `@task` or `@decision`
- **Chart a table** — a `Chart this table` action on every `@table` picks the
  type, the label column and the value columns, and writes them onto the block.
  The preview draws the chart and lets you read values off it and hide a series
- **Folding** on `@block` / `@endblock` and on headings, and `@comment`
  toggling

VS Code cannot draw images inline in a text editor — no extension API exposes
that — so the preview panel is where you see them, the same way Markdown works.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `xtxt.paste.embed` | `false` | Embed pasted images as `data:` URIs instead of saving them beside the document. Self-contained, ~33% larger, and it makes git diffs unreadable. |
| `xtxt.paste.folder` | `""` | Folder for pasted images, relative to the document. Empty means beside it. |

## Install from source

```sh
git clone https://github.com/SaiMouli3/xtxt
cd xtxt/editors/vscode
npm install && npm run build
npx @vscode/vsce package --no-dependencies
code --install-extension xtxt-*.vsix
```

The preview renders with the same JavaScript SDK as the CLI's HTML export and
the browser demo, so what you see here and what `xtxt export … html` produces
cannot drift apart.

[Specification](https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md) ·
[Live demo](https://saimouli3.github.io/xtxt/)
