# XTXT for Obsidian

Open, read and edit `.xtxt` documents in your vault: images, tables, charts and
typed records in one plain-text file that still opens in any editor.

- `.xtxt` files open in a proper view, not as unrecognised text
- Preview and source, toggled from the pane header
- Records — `@task`, `@decision`, `@knowledge`, or a type nobody has invented
  yet — render as labelled cards
- Charts render as inline SVG, themed by your vault
- Relative `@image(src="cat.png")` resolves against the document's folder
- Parser warnings appear above the document; unknown directives are warnings,
  never errors

## Commands

| Command | What it does |
|---|---|
| Create new XTXT note | A new `.xtxt` with metadata and a starter record |
| Show structure of the current document | Headings, tasks, records and word count — what `xtxt extract` would hand an agent |

## Install from source

```sh
git clone https://github.com/SaiMouli3/xtxt
cd xtxt/integrations/obsidian
npm install && npm run build
mkdir -p /path/to/vault/.obsidian/plugins/xtxt
cp main.js manifest.json styles.css /path/to/vault/.obsidian/plugins/xtxt/
```

Enable **XTXT** under Settings → Community plugins, then open any `.xtxt` file.

## Why this and not Markdown

Front matter is one record, at the top of the file. XTXT lets a record sit
anywhere — next to the thing it describes — and names it. A document with
twelve decisions in it cannot express that as front matter.

Rendering uses the same JavaScript SDK as the browser demo and the CLI, so what
your vault shows and what `xtxt render` produces cannot drift apart.

[Format specification](https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md) ·
[Live demo](https://saimouli3.github.io/xtxt/)
