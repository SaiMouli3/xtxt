# What XTXT does

A feature list, honest about what is not there. For the normative rules see
[SPEC.md](SPEC.md); this page is for deciding whether the format is worth your
time.

## The one-line version

Plain text that carries structure a machine can read without guessing — and
still opens in any editor and diffs cleanly in git.

---

## Writing

### Text

| | Syntax |
|---|---|
| Headings | `# Title`, `## Section`, up to `######` |
| Paragraphs | blank-line separated |
| Quotes | `> quoted line` |
| Lists | `- item`, `* item`, `1. item` |
| Checklists | `- [ ] todo`, `- [x] done` |
| Emphasis | `*emphasis*`, `**strong**`, `` `code` ``, `~~strike~~` |
| Links | `[text](url)` |
| Footnotes | `@footnote(id="a")` with `[^a]` references |

Lists **do not nest**. An item indented deeper than the first warns rather
than losing your structure quietly.

### Everything else is a directive

One syntax for every non-textual thing. `@name(args)` for a single item,
`@name … @endname` for a block. Adding a name never breaks an older reader.

```
@image(src="diagram.png", caption="How it fits together", width=600)

@code(language="python")
print("hello")
@endcode
```

| Directive | Purpose |
|---|---|
| `@image` `@video` `@audio` `@attachment` | Media, referenced or embedded as `data:` URIs |
| `@code` | Fenced source, with a language for highlighting |
| `@table` | Pipe-separated rows, with alignment |
| `@chart` | Bar, line, area, stacked and proportion charts |
| `@math` | Formulas |
| `@mermaid` | Diagrams |
| `@metadata` | Document properties, one `key = value` per line |
| `@comment` | Text that never renders |
| `@raw(format="html")` | Passthrough for the renderer's own format |
| `@include` `@embed` | Splice another document in; `@embed` demotes its headings |
| `@hr` | Rule |

### Records — the part that is not Markdown-with-extra-steps

A block of `Key: value` lines that a parser returns as data, next to the prose
that explains it.

```
@task
Title: Ship the reference parser
Status: In Progress
Owner: Subbu
Due: 2026-08-15
@endtask
```

`@task`, `@decision`, `@knowledge`, `@note`, `@ai`, `@prompt` and `@chat` are
standard, but the field names mean nothing to the format — it guarantees the
*shape* is preserved and reported. `@experiment` or `@invoice` works with no
parser change.

### Charts from tables

A table can draw itself, so the numbers and the picture cannot disagree. Add a
row and the chart follows.

```
@table(chart="bar", x="Month", y="Signups, Revenue")
Month | Signups | Revenue
------|---------|--------
Jan   | 20      | 1200
Feb   | 35      | 2400
@endtable
```

The table is still rendered underneath, so the figures stay readable as text.

---

## The machine-facing view

`xtxt extract` turns a document into what an agent needs, without inferring
anything from prose:

```sh
xtxt extract notes.xtxt
```

```json
{
  "outline": [{"level": 1, "text": "Project Log", "line": 9}],
  "tasks":   [{"title": "Ship the reference parser", "status": "In Progress",
               "owner": "Subbu", "done": false, "line": 20}],
  "blocks":  [{"type": "decision", "fields": {"title": "…", "why": "…"}}],
  "links": [], "media": [], "code": [], "words": 412
}
```

Everything carries a line number, so anything extracted can be traced back to
the source that produced it.

---

## Command line

```sh
xtxt validate  file.xtxt      # syntax and problems
xtxt lint      file.xtxt      # validate, plus style warnings
xtxt render    file.xtxt      # render in the terminal
xtxt export    file.xtxt html # html, body, md, text, json
xtxt import    notes.md       # Markdown in
xtxt ast       file.xtxt      # parse tree as JSON
xtxt extract   file.xtxt      # machine-facing view as JSON
xtxt paste     file.xtxt      # append the clipboard image
```

Useful flags: `--resolve` expands includes, `--interactive` inlines the chart
runtime, `--mermaid` loads the diagram renderer, `-o` writes to a file.

Diagnostics come at two levels. **Errors** mean the file cannot be interpreted
unambiguously; **warnings** mean it parses but is suspect. A validator exits
non-zero only on errors, and an unknown directive is never an error.

---

## Editors

**VS Code** — syntax highlighting, live preview, problems and squiggles as you
type, an outline covering headings *and* records, folding, image paste with
`Cmd+V`, and a `Chart this table` action.

**Obsidian** — rendering and image paste.

---

## Libraries

| Language | Package | Parses | Renders |
|---|---|---|---|
| Go | `github.com/SaiMouli3/xtxt` | yes | yes |
| JavaScript | `xtxt-js` | yes | yes |
| Python | `xtxt` | yes | — |
| Rust | `xtxt` | yes | — |
| Java | `io.github.saimouli3:xtxt` | yes | — |
| C | `sdk/c` | yes | — |
| C++ | `sdk/cpp` | yes | — |

Every implementation is checked against the same conformance fixtures, which
pin the parse tree and the severity of every diagnostic. Zero runtime
dependencies.

There is also an MCP server (`xtxt-mcp`) so an agent can read and write XTXT
documents directly.

---

## What it deliberately does not do

Being clear about this is the point, not an apology.

- **Nothing nests.** No nested lists, and no blocks inside blocks — so an
  admonition containing a code sample is not expressible. This keeps parsing
  to a single pass with no backtracking, and it is the format's biggest
  limitation.
- **No schemas yet.** A record's field names are preserved and reported, but
  nothing declares that `Status` is an enum or `Due` is a date.
- **No compression, encryption, revision history or binary container.** Those
  belong in a layer above — an encrypted volume, a signature alongside, the
  version control you already have. The bytes on disk are the document.

---

## Reading further

- [SPEC.md](SPEC.md) — the normative definition, implementable in an afternoon
- [Live demo](https://saimouli3.github.io/xtxt/) — edit XTXT, watch it render
- [examples/agent-notes.xtxt](examples/agent-notes.xtxt) — every feature, aimed at agents
- [conformance/](conformance/) — the fixtures that define "an XTXT parser"
