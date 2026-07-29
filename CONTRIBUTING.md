# Contributing

The most valuable contribution is **an implementation in a language that does
not have one yet**. A format with one parser is a tool; a format with several
is a standard.

## Writing an implementation

1. Read [SPEC.md](SPEC.md). It is 404 lines and complete — there is no second
   document and no folklore.
2. Read every `*.xtxt` in [conformance/cases](conformance/cases) and produce
   the two structures in the matching `.json`: the normalised AST and the
   diagnostics. That is the entire bar.
3. Open a PR adding it under `sdk/<language>/` with a test that runs those
   fixtures, and a CI job that runs the test.

A passing parser is roughly 600–900 lines in most languages. You do not need
extraction, HTML rendering or charts — those are conveniences the Go and
JavaScript implementations happen to have.

If the spec is ambiguous enough that two people could implement it differently,
**that is a spec bug and the most useful issue you can file.** Please open one
rather than guessing.

## Changing the format

Adding a directive or an attribute is a minor version. Removing or repurposing
one is major (SPEC §7). Anything that would make an existing document parse
differently needs a very good reason.

Two things the format deliberately does not do, and PRs adding them will be
declined with thanks: **in-file compression and encryption**. Both trade away
the property the format exists for — that the bytes on disk are the document,
readable in any editor and diffable by any tool. See SPEC §7.2.

## Running everything

```sh
go test ./...                                    # reference implementation
cd sdk/python && python -m pytest
cd sdk/js     && node --test
cd sdk/rust   && cargo test
cd sdk/java   && mvn -B test
cd sdk/c      && make test && make asan
cd sdk/cpp    && cmake -S . -B build && cmake --build build && ctest --test-dir build
```

`npm install` at the repo root links the JavaScript SDK to the integrations
that depend on it, so the MCP server, the Obsidian plugin and the VS Code
extension all build without anything being published first.

To regenerate the conformance expectations after an intentional spec change:

```sh
go test -run TestConformance -update
```

Then re-run every other implementation. If they disagree, the fixtures are
doing their job.

## Reporting bugs

A parser bug is much easier to act on as a document. Please include the
smallest `.xtxt` that reproduces it, what you expected, and what you got —
ideally `xtxt ast yourfile.xtxt` output.
