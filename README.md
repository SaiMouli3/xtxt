# XTXT

A plain-text document format that keeps images, tables, code, diagrams, math and
metadata **in the file**, in a form you can still read in `cat`, edit in `vi`,
and diff in `git`.

```text
# Machine Learning

Neural networks are inspired by the brain.

@image(src="cnn.png", caption="CNN Architecture", width=600)

@table
Layer | Parameters
------|-----------
Conv1 | 256
Conv2 | 512
@endtable
```

Everything non-textual uses one syntax — `@name(args)` for a single item,
`@name … @endname` for a block. That is the whole format. The rest is which
names exist, and adding a name never breaks an old reader.

- **[SPEC.md](SPEC.md)** — the format definition. Implementable in an afternoon.
- **[examples/notes.xtxt](examples/notes.xtxt)** — a document using every feature.

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

`-` reads stdin. `xtxt export … html` produces a standalone, dependency-free
page with light and dark styling; add `--mermaid` to have diagrams drawn.
For PDF, print that HTML — a bundled PDF engine is not worth the dependency.

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

## Editor support

`editors/vscode/` is a VS Code extension providing syntax highlighting, code
blocks highlighted in their own language, folding on `@block`/`@endblock`, and
comment toggling. To try it, symlink it into `~/.vscode/extensions/` and reload.

## What is here, and what is not

Built and tested:

- the specification
- a parser producing a documented AST, with line numbers on every node
- a validator (errors) and linter (style)
- renderers: HTML, terminal, CommonMark, JSON
- a Markdown importer
- the CLI
- a VS Code syntax extension

Not built, and deliberately: a desktop editor, SDKs in four more languages, a
PDF engine, and a language server. Each is a project of its own, and each is
much easier to write against a spec and a reference implementation that already
exist — which is the point of doing these first. The Go implementation is small
enough (about 1,800 lines) to port rather than bind to.

## Tests

```sh
go test ./...
```

## License

MIT.
