# XTXT

[![ci](https://github.com/SaiMouli3/xtxt/actions/workflows/ci.yml/badge.svg)](https://github.com/SaiMouli3/xtxt/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/SaiMouli3/xtxt.svg)](https://pkg.go.dev/github.com/SaiMouli3/xtxt)
[![PyPI](https://img.shields.io/pypi/v/xtxt?label=pypi)](https://pypi.org/project/xtxt/)
[![npm](https://img.shields.io/npm/v/xtxt-js?label=npm)](https://www.npmjs.com/package/xtxt-js)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Plain text that machines can read — without guessing.**

Markdown makes an agent infer structure from prose. XTXT lets the document
carry it: images, tables, charts and typed records, in a file that still opens
in any text editor and diffs cleanly in git.

### → [Try it in your browser](https://saimouli3.github.io/xtxt/)

```text
# Machine Learning

Neural networks are inspired by the brain.

@image(src="cnn.png", caption="CNN Architecture", width=600)

@task
Title: Ship the reference parser
Status: In Progress
Owner: Subbu
@endtask
```

That `@task` is not a comment or a convention — it is a block the parser
returns and `xtxt extract` hands to an agent as JSON. Everything non-textual
uses one syntax: `@name(args)` for a single item, `@name … @endname` for a
block. That is the whole format. Adding a name never breaks an old reader.

- **[SPEC.md](SPEC.md)** — the format definition. Implementable in an afternoon.
- **[Live demo](https://saimouli3.github.io/xtxt/)** — edit XTXT, watch it render.
- **[examples/agent-notes.xtxt](examples/agent-notes.xtxt)** — every feature, aimed at agents.
- **[conformance/](conformance/)** — the fixtures that define "an XTXT parser".

## Structure a machine can read

The part that is not Markdown-with-extra-steps: a document can carry records,
next to the prose they describe.

```text
@decision
Title: Unknown directives are warnings, never errors
Why: A reader from today must stay useful on a document written tomorrow.
@enddecision
```

`xtxt extract` turns a document into exactly what an agent needs — outline,
tasks, decisions, links, media, code, and every record block — without
inferring any of it from prose:

```sh
xtxt extract notes.xtxt
```

```json
{
  "outline": [{"level": 1, "text": "Project Log", "line": 9}],
  "tasks": [{"title": "Ship the reference parser", "status": "In Progress",
             "owner": "Subbu", "due": "2026-08-15", "done": false, "line": 20}],
  "blocks": [{"type": "decision", "fields": {"title": "…", "why": "…"}}]
}
```

Field names mean nothing to the format. XTXT guarantees the *shape* is
preserved and reported; what `Priority: High` means is your application's
business. That is the same bargain HTML made with class names, and it is why
`@experiment` or `@invoice` works without anyone updating a parser.

## Install

```sh
go install github.com/SaiMouli3/xtxt/cmd/xtxt@latest
```

Or from this directory:

```sh
go build -o xtxt ./cmd/xtxt
```

## Use

```sh
xtxt validate notes.xtxt          # syntax + semantic check, non-zero on errors
xtxt lint notes.xtxt              # the above, plus style warnings
xtxt render notes.xtxt            # read it in the terminal
xtxt export notes.xtxt html -o notes.html
xtxt export notes.xtxt md         # convert to CommonMark
xtxt import notes.md              # convert CommonMark to XTXT
xtxt ast notes.xtxt               # the parse tree, as JSON
```

`-` reads stdin. `--resolve` expands `@include`/`@embed` first.

`xtxt export … html` produces a standalone, dependency-free page with light and
dark styling; add `--mermaid` to have diagrams drawn. For PDF, print that HTML —
a bundled PDF engine is not worth the dependency.

## Charts

```text
@chart(type="bar", title="Monthly signups")
Jan | 20
Feb | 35
@endchart
```

Renders as inline SVG with no script and no dependency, themed by CSS custom
properties. Colours come from a palette validated for colour-vision deficiency
and for contrast against both light and dark surfaces, capped at the three
slots that pass on every pair — extra series fold into "Other" rather than
inventing hues, and say so. Every chart ships a table view, because colour is
not an accessible encoding on its own.

`type="pie"` is accepted and drawn as a proportion bar: lengths can be
compared, angles cannot.

## Plugins

Drop an `xtxt.plugins.json` beside a document and new directives render:

```json
[{ "name": "youtube",
   "html": "<iframe src=\"https://www.youtube.com/embed/{{.Args.id}}\"></iframe>" }]
```

```text
@youtube(id="abc123")
```

A plugin is a declaration, not code — a name and a template. Values are escaped
on the way in, so a document can never inject markup, and opening a document is
never consent to run anything.

## Use as a library

```go
res, err := xtxt.ParseFile("notes.xtxt")
for _, n := range res.Doc.Nodes {
    fmt.Println(n.Kind, n.Name, n.Line)
}
fmt.Println(res.Doc.Metadata()["author"])
html := xtxt.RenderHTML(res.Doc, xtxt.HTMLOptions{Full: true})
```

Parsing never fails on unrecognised content: `res.Doc` is always usable, and
`res.Issues` tells you what was wrong. That is the compatibility guarantee —
a reader from today opens a document written against a later version of the
spec, shows every paragraph, and shows a placeholder where the new thing was.

## SDKs

| Language | Location | Install |
|---|---|---|
| Go | this module | `go get github.com/SaiMouli3/xtxt` |
| Python | `sdk/python` | `pip install xtxt` |
| JavaScript | `sdk/js` | `npm install xtxt-js` |
| Rust | `sdk/rust` | `cargo add xtxt` |
| Java | `sdk/java` | `io.github.saimouli3:xtxt:0.1.0` |
| C | `sdk/c` | `make` — C99, no dependencies |
| C++ | `sdk/cpp` | `cmake` — C++17, wraps the C library |

```python
import xtxt
res = xtxt.parse_file("notes.xtxt")
for t in xtxt.extract(res.doc)["tasks"]:
    print(t["title"], t["status"])
```

```js
import { parse, extract } from 'xtxt-js';
const { doc } = parse(await readFile('notes.xtxt', 'utf8'));
console.log(extract(doc).outline);
```

These are ports, not bindings — no subprocess, no shared library. They agree
because all three run the same fixtures in `conformance/`:

```sh
go test ./...                      # regenerate with -update
cd sdk/python && python -m pytest
cd sdk/js     && node --test
cd sdk/rust   && cargo test
cd sdk/java   && mvn test
cd sdk/c      && make test        # and `make asan`
cd sdk/cpp    && cmake -S . -B build && cmake --build build && ctest --test-dir build
```

A fourth implementation joins the standard by passing that directory. Each case
is a `.xtxt` file and the normalised AST plus diagnostics it must produce.

Every SDK covers parsing and validation. Extraction and HTML are everywhere
except C, which stops at the parser and hands the rest to the C++ wrapper —
C has no string type worth inventing an ownership convention for. Charts are
in Go and JavaScript; terminal and Markdown rendering, `@include` resolution
and plugins live in Go only.

C++ is a wrapper over C rather than a seventh parser: one parser means the two
cannot disagree about the format. The C library is also the FFI substrate —
anything that can call C can read XTXT without a new implementation, which is
the cheapest way to reach Ruby, PHP, Lua, Zig or Swift.

## Editor support

`editors/vscode/` is a VS Code extension providing syntax highlighting, code
blocks highlighted in their own language, folding on `@block`/`@endblock`, and
comment toggling. To try it, symlink it into `~/.vscode/extensions/` and reload.

## What is here, and what is not

Built and tested:

- the specification
- a parser producing a documented AST, with line numbers on every node
- a validator (errors) and linter (style), including footnote reference checking
- renderers: HTML, terminal, CommonMark, JSON, and the `extract` view for agents
- records, charts, footnotes, `@include`/`@embed`, and a declarative plugin system
- a Markdown importer
- the CLI
- SDKs for Python, JavaScript, Rust, Java, C and C++, all held to one conformance suite
- a VS Code syntax extension

Not built: a desktop editor, a language server, a PDF engine, DOCX/EPUB/PPTX
writers, mobile apps, and cloud sync. Each is a project of its own, and each is
far easier against a spec and three agreeing implementations — which is the
point of doing these first.

Not built **on purpose**, which is different: in-file compression, encryption,
and embedded revision history. Each would trade away the property the format
exists for — that the bytes on disk are the document, readable in any editor
and diffable by any tool. Put the `.xtxt` in an encrypted volume, sign it
alongside, and let git keep the history. See SPEC §7.2.

## Tests

```sh
go test ./...
```

## License

MIT.
