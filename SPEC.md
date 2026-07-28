# XTXT Specification — Version 1.0

XTXT is a plain-text document format. A conforming file is valid UTF-8 and
remains readable in any text editor. All structure is expressed with line-based
constructs so that a parser needs no lookahead beyond the current line, and so
that `git diff` stays meaningful.

Status: draft. File extension: `.xtxt`. MIME type: `text/xtxt`.

---

## 1. Lexical rules

- Encoding is UTF-8. A leading BOM is ignored.
- Line endings may be `LF` or `CRLF`; `CR` is stripped before parsing.
- Lines are significant. Indentation is not significant except inside fenced
  blocks, where it is preserved verbatim.
- A **blank line** is a line containing only whitespace. Blank lines separate
  paragraphs and terminate lists and quotes.
- A line whose first non-whitespace character is `@` introduces a **directive**,
  unless escaped as `\@`, which yields a literal `@` and makes the line ordinary
  text.

## 2. Document

```
document := prologue? block*
```

The optional prologue is a version declaration on the first non-blank line:

```
@xtxt(version=1.0)
```

If absent, readers assume the latest version they implement. A reader
encountering a `version` it does not know MUST still parse the file, treating
unknown constructs per §7.

## 3. Text blocks

### 3.1 Headings

One to six `#` followed by a space:

```
# Title
### Subsection
```

### 3.2 Paragraphs

One or more consecutive non-blank lines that match no other rule. Line breaks
inside a paragraph are soft; renderers join them with a single space.

### 3.3 Quotes

Consecutive lines beginning with `>`:

```
> Quoted text.
> Still the same quote.
```

### 3.4 Lists

Consecutive lines beginning with `- ` or `* ` (unordered) or `N. ` (ordered).
A list ends at the first blank line or non-item line. Lists do not nest in 1.0.

```
- Item one
- Item two

1. First
2. Second
```

### 3.5 Checklists

An unordered item whose content begins with `[ ]` or `[x]`:

```
- [ ] Not done
- [x] Done
```

### 3.6 Inline formatting

Within paragraphs, headings, quotes, list items and table cells:

| Syntax | Meaning |
|---|---|
| `**text**` | strong |
| `*text*` | emphasis |
| `` `text` `` | inline code |
| `[label](target)` | link |

A backslash escapes the next character. Inline code spans are literal: no
other inline rule applies inside them.

## 4. Directives

All non-text content uses a single syntax. There are two forms.

### 4.1 Inline directive

```
@name(arg, key=value, key="value with spaces")
```

The argument list may span multiple lines; it ends at the matching `)`.

```
@image(
    src="cnn.png",
    caption="CNN Architecture",
    width=600
)
```

Argument grammar:

```
args      := (arg (',' arg)*)? ','?
arg       := key '=' value | value
key       := [A-Za-z_][A-Za-z0-9_-]*
value     := quoted | bare
quoted    := '"' ( escaped | any-but-quote )* '"'
bare      := any run of characters excluding ',' ')' and leading/trailing space
```

Positional arguments are assigned to the directive's declared first parameter
(see §5). `@video("demo.mp4")` and `@video(src="demo.mp4")` are equivalent.

### 4.2 Fenced directive

An opening line `@name` (optionally with an argument list) and a closing line
`@endname`. Everything between is the raw payload, preserved byte for byte
except for the trailing newline before the fence.

```
@code(language="python")
print("Hello")
@endcode
```

A blank line immediately after the opening fence and immediately before the
closing fence is not part of the payload, so both spellings below are equal:

```
@code(language="go")
func main(){}
@endcode
```
```
@code(language="go")

func main(){}

@endcode
```

Fences do not nest. A payload line that would close the fence can be escaped
with a leading backslash: `\@endcode`.

### 4.3 Telling the two apart

Both forms may carry an argument list, so the form is decided by the document,
not by a table of known names: **a directive is fenced if and only if a line
consisting exactly of `@end<name>` appears later in the file.** Otherwise it is
inline.

This rule is deliberately local. A reader that has never heard of `@timeline`
still parses `@timeline … @endtimeline` as a block with a payload, and still
parses `@sparkline(data="1,2,3")` as an inline directive — which is what makes
§7 possible without a registry of names every implementation must keep in sync.

## 5. Standard directives

Readers MUST accept all of these. Unknown attributes are ignored, not errors.

| Directive | Form | First positional | Attributes |
|---|---|---|---|
| `xtxt` | inline | `version` | `version` |
| `image` | inline | `src` | `src`, `alt`, `caption`, `width`, `height` |
| `video` | inline | `src` | `src`, `caption`, `width`, `height`, `poster` |
| `audio` | inline | `src` | `src`, `caption` |
| `attachment` | inline | `src` | `src`, `name` |
| `include` | inline | `src` | `src` |
| `hr` | inline | — | — |
| `code` | fenced | `language` | `language`, `caption`, `filename` |
| `table` | fenced | — | `caption`, `align` |
| `math` | fenced | — | `caption` |
| `mermaid` | fenced | — | `caption` |
| `metadata` | fenced | — | — |
| `comment` | fenced | — | — |
| `raw` | fenced | `format` | `format` (e.g. `html`) |

### 5.1 `table` payload

Rows are non-blank lines; cells are separated by `|`. Blank lines between rows
are ignored. If a row consists only of `-`, `:` and `|`, it is a separator and
the rows above it are the header.

```
@table
Name | Age
-----|----
John | 20
@endtable
```

A table without a separator has its first row as the header.

### 5.2 `metadata` payload

`key = value`, one per line. Keys are case-insensitive and trimmed. A document
may contain at most one `metadata` block; readers SHOULD surface it as document
properties rather than rendering it inline.

### 5.3 `comment` payload

Not rendered in any output. Retained by the parser so that tools can round-trip
a document.

## 6. Escaping

| Sequence | Yields |
|---|---|
| `\@` at line start | literal `@` |
| `\@endname` in a payload | literal `@endname` |
| `\*`, `` \` ``, `\[`, `\\` inline | the literal character |

## 7. Extensibility and compatibility

- An unrecognised directive is **not an error**. A reader MUST preserve it in
  the tree as an unknown node with its name, arguments and payload intact, and
  SHOULD render it as a visible placeholder rather than dropping it.
- Adding a directive or an attribute is a minor version change.
- Removing or repurposing one is a major version change.
- A writer MUST NOT reorder or discard unknown nodes when rewriting a file.

This is what makes an XTXT file forward compatible: a 1.0 reader opening a 1.4
document still shows every paragraph, and shows the reader that something newer
is present.

## 8. Errors

Validation reports two levels:

- **error** — the file cannot be interpreted unambiguously: an unclosed fence,
  an unclosed argument list, a `@end` with no matching opening fence.
- **warning** — the file parses but is suspect: an unknown directive, a
  duplicate `metadata` block, a table row with an inconsistent cell count, an
  `@image` with no `src`.

A validator exits non-zero only on errors.

## 9. Parsing cost

Every construct is terminated by a line and nothing nests, so a parser needs a
single pass, no backtracking inside a block, and no grammar generator. The one
place lookahead is required is §4.3, where a directive scans forward for its
closing fence; an implementation that wants to stream may cap that scan and
treat "not found within N lines" as inline.
