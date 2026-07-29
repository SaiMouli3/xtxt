# xtxt (C++)

C++17 interface to the [XTXT](https://github.com/SaiMouli3/xtxt) plain-text
document format.

```cpp
#include "xtxt.hpp"

auto res = xtxt::parse(source);
for (const auto& n : res.doc.nodes) {
    std::cout << xtxt::kind_name(n.kind) << " at line " << n.line << "\n";
}

auto data = xtxt::extract(res.doc);      // outline, tasks, blocks, links, media, code
auto html = xtxt::render_html(res.doc);
for (const auto& issue : xtxt::validate(source)) { /* ... */ }
```

**This is a wrapper over the [C library](../c), not a sixth parser.** One
parser means C and C++ cannot disagree about the format — only about
ergonomics, which is where they should differ. Parsing copies out of the C
structures immediately, so nothing dangles and there is nothing to free.

`validate` takes the source rather than a `Document`, because validation runs
in the C library and re-parsing is cheaper and far less error-prone than
marshalling a C++ tree back across the boundary.

## Build

```sh
cmake -S . -B build && cmake --build build
ctest --test-dir build --output-on-failure
```

Header-only apart from the C translation unit: add `sdk/c/xtxt.c` to your build
and put both directories on the include path.
