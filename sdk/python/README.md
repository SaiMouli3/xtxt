# xtxt (Python)

Parser, validator, extractor and HTML renderer for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.
Pure Python, no dependencies.

```python
import xtxt

res = xtxt.parse_file("notes.xtxt")
for issue in xtxt.validate(res.doc):
    print(issue.line, issue.severity, issue.message)

data = xtxt.extract(res.doc)      # outline, tasks, blocks, links, media, code
html = xtxt.render_html(res.doc, full=True)
```

```sh
python -m xtxt extract notes.xtxt
```

This is a port of the Go reference implementation, not a binding. Both are
checked against the same fixtures in `conformance/`.
