# Conformance suite

Each case is a `.xtxt` input and the `.json` output every implementation must
produce: the normalised AST (`ast`) and the diagnostics it must report
(`issues`).

Diagnostic **messages** are excluded on purpose. What implementations must
agree on is that a given line is an error or a warning, not how they word the
complaint.

Run it:

```sh
go test ./...                        # regenerate: go test -run TestConformance -update
cd sdk/python && python -m pytest
cd sdk/js     && node --test
```

To add an implementation, read every `*.xtxt` here, produce the same two
structures, and compare. That is the entire bar for calling something an XTXT
parser.
