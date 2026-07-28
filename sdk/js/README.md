# xtxt (JavaScript)

Parser, validator, extractor and HTML renderer for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.
Dependency-free ESM, runs in Node and the browser.

```js
import { parse, validate, extract, renderHTML } from 'xtxt';

const { doc, issues } = parse(source);
for (const i of validate(doc)) console.log(i.line, i.severity, i.message);

const data = extract(doc);          // outline, tasks, blocks, links, media, code
const html = renderHTML(doc, { full: true });
```

This is a port of the Go reference implementation, not a binding. Both are
checked against the same fixtures in `conformance/`.
