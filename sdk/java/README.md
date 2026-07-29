# xtxt (Java)

Parser, validator and extractor for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.
No runtime dependencies, Java 17+.

```xml
<dependency>
  <groupId>io.github.saimouli3</groupId>
  <artifactId>xtxt</artifactId>
  <version>0.1.2</version>
</dependency>
```

```java
import io.github.saimouli3.xtxt.Xtxt;

var res = Xtxt.parse(Files.readString(Path.of("notes.xtxt")));
for (var issue : Xtxt.validate(res.doc)) {
    System.out.println(issue.line + ": " + issue.severity + ": " + issue.message);
}

var data = Xtxt.extract(res.doc);   // outline, tasks, blocks, links, media, code
var html = Xtxt.renderHtml(res.doc);
```

This is a port of the Go reference implementation, not a binding. Both are
checked against the same fixtures in `conformance/`.
