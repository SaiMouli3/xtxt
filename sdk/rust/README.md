# xtxt (Rust)

Parser, validator and extractor for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.
No runtime dependencies.

```rust
let res = xtxt::parse(&std::fs::read_to_string("notes.xtxt")?);
for issue in xtxt::validate(&res.doc) {
    println!("{}: {:?}: {}", issue.line, issue.severity, issue.message);
}

let data = xtxt::extract(&res.doc);   // outline, tasks, blocks, links, media, code
let html = xtxt::render_html(&res.doc);
```

This is a port of the Go reference implementation, not a binding. Both are
checked against the same fixtures in `conformance/`.
