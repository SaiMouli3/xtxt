# xtxt (C)

Parser and validator for the [XTXT](https://github.com/SaiMouli3/xtxt)
plain-text document format. C99, no dependencies.

```c
#include "xtxt.h"

xtxt_result *r = xtxt_parse(source, 0);
for (size_t i = 0; i < r->doc.node_count; i++) {
    printf("%s at line %zu\n", xtxt_kind_name(r->doc.nodes[i].kind), r->doc.nodes[i].line);
}

xtxt_issues issues = xtxt_validate(&r->doc, NULL, 0);
xtxt_issues_free(issues);
xtxt_result_free(r);
```

Ownership is deliberately boring: every function returning a pointer returns
memory you own, and each has exactly one matching free. Strings inside a
document belong to the document and die with it.

## Build

```sh
make          # build/libxtxt.a
make test     # unit checks + the shared conformance suite
make asan     # both again under AddressSanitizer and UndefinedBehaviorSanitizer
```

The C library is also the FFI substrate: anything that can call C — Ruby, PHP,
Lua, Zig, Swift, Python via ctypes — can read XTXT without a new parser.

## Scope

Parsing, validation, arguments, records, tables and inline formatting. The
extraction view and HTML rendering live in the [C++ wrapper](../cpp), which has
`std::string` and containers to do them without inventing an ownership
convention for every return value.

This is a port of the Go reference implementation, not a binding. Both are
checked against the same fixtures in `conformance/`.
