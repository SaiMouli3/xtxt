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

Because they do not nest, indentation before a marker carries no meaning and
the item joins the same flat list. An item indented deeper than the list's
first item is almost always an attempt to nest, so a reader MUST report a
warning there — once per list is enough. Flattening the structure silently
would let a formatting mistake destroy the author's meaning with nothing to
show for it; a uniformly indented list is a style choice and MUST stay quiet.

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
| `embed` | inline | `src` | `src` |
| `footnote` | fenced | `id` | `id` |
| `chart` | fenced | `type` | `type`, `title`, `unit` |
| `task` | fenced | — | record (§5.4) |
| `decision` | fenced | — | record (§5.4) |
| `knowledge` | fenced | — | record (§5.4) |
| `note` | fenced | — | record (§5.4) |
| `ai` | fenced | — | record (§5.4) |
| `prompt` | fenced | — | record (§5.4) |
| `chat` | fenced | — | record (§5.4) |

A renderer that highlights `@code` SHOULD do so while rendering rather than by
loading a script into the output, so that a rendered document stays
self-contained. Highlighting is presentational: it MUST NOT change the text a
reader copies, and a language the renderer does not know MUST be rendered as
plain escaped text rather than guessed at.

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

A `@table` may also ask to be drawn as a chart over its own rows:

```
@table(chart="bar", x="Month", y="Signups, Revenue", title="Growth")
Month | Signups | Revenue
------|---------|--------
Jan   | 20      | 1200
Feb   | 35      | 2400
@endtable
```

`chart` names the type and is what enables the chart; the values are those of
`@chart` (§5.5). `x` names the label column and defaults to the first; `y` is a
comma-separated list of value columns and defaults to every other column that
holds a number. `title` and `unit` carry through to the chart.

A renderer that draws the chart MUST still render the table, which is what
keeps the numbers reachable as text. A reader that does not recognise these
arguments ignores them and renders the table alone, so the document is
readable either way.

Values that do not parse as numbers, and names in `x` or `y` that match no
column, are reported however the implementation reports chart problems; they
never make the document invalid.

### 5.2 `metadata` payload

`key = value`, one per line. Keys are case-insensitive and trimmed. A document
may contain at most one `metadata` block; readers SHOULD surface it as document
properties rather than rendering it inline.

### 5.3 `comment` payload

Not rendered in any output. Retained by the parser so that tools can round-trip
a document.

### 5.4 Record payloads

A **record** is a block payload read as an ordered list of `Key: value` (or
`Key = value`) entries. It is how a document carries structure a machine can
use without inferring it from prose:

```
@task
Title: Build parser
Status: In Progress
Owner: Subbu
Priority: High
@endtask
```

Rules:

- A line opens a field when it begins with a key followed by `:` or `=`. A key
  starts with a letter or `_`, contains only letters, digits, spaces, `_`, `-`
  and `.`, is at most **32 characters**, and is at most **3 words**. These caps
  are deliberate: without them an ordinary sentence containing a colon would be
  read as a field.
- Lines following a field are appended to that field's value, so a value may
  span paragraphs.
- Lines before the first field are kept under the empty key, so nothing is lost.
- **Order is significant.** A `@chat` block's turns are fields, and their
  sequence is the conversation.
- Field names carry no meaning at the format level. A reader renders and
  extracts them faithfully; what `Priority: High` *means* is the application's
  business, not the format's.

Any block may be read as a record, including one the reader has never seen.
That is what lets a new semantic block work in an old reader:

```
@experiment
Hypothesis: caching helps
Confidence: 0.7
@endexperiment
```

### 5.5 `chart` payload

Rows of `Label | value`, `Label: value` or `Label value`. A first row whose
trailing cells are all non-numeric names the series.

```
@chart(type="bar", title="Monthly signups")
Jan | 20
Feb | 35
@endchart
```

`type` is `bar` (default), `line`, `area`, or `stacked`. `pie` is accepted and
renders as a proportion bar: lengths can be compared, angles cannot.

A renderer MUST make the underlying numbers reachable as text — a table, a
caption or direct labels — because colour alone is not an accessible encoding
and print, forced-colours and colour-vision-deficient readers may not receive
it at all.

### 5.6 `include` and `embed`

```
@include(src="introduction.xtxt")
@embed(src="api-reference.xtxt")
```

`@include` splices the referenced document's blocks in place. `@embed` does the
same but demotes the referenced document's headings by one level, so it nests
under the including section rather than competing with it.

A resolver MUST:

- refuse absolute paths and any path that escapes the including document's
  directory,
- refuse remote sources unless the host application opts in explicitly,
- detect cycles and bound nesting depth,
- bound the **total** number of nodes produced by expansion.

Rendering a document must never become a way to read arbitrary files.

The last requirement is not implied by the one before it. Cycle detection
tracks the path currently being expanded, so including the same file twice
from different places is legal and should stay legal — that is a diamond, not
a cycle. But it means depth alone does not bound the work: a file that includes
four others, fourteen levels deep, is 268 million nodes from under 1.5 KB on
disk. Bounding depth without bounding total expansion reads like a defence
without being one.

### 5.7 Footnotes

A marker `[^id]` in prose refers to an `@footnote` block anywhere in the file:

```
Neural networks are universal approximators[^cybenko].

@footnote(id="cybenko")
Cybenko, G. (1989).
@endfootnote
```

Markers and notes are matched by `id`. A marker with no note, or a note nobody
cites, is a warning — never an error.

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

### 7.1 Extensions

Nothing in §4 or §5.4 is reserved to this specification. A new directive needs
no registration to work:

```
@youtube(id="abc123")
@spotify(track="…")
@timeline
2024-01: started
2025-06: shipped
@endtimeline
```

A conforming reader parses all three today: the first two as inline directives,
the third as a fenced block whose payload is a record. What a reader does *not*
know is how to draw them, and the answer is a **renderer**, not a parser change.

An implementation MAY accept a declarative plugin manifest mapping a directive
name to a rendering template. It MUST NOT let a document itself supply
executable code: opening a document is not consent to run it.

### 7.2 What XTXT deliberately does not do

A format is defined as much by what it refuses. XTXT does not specify
compression, encryption, embedded revision history, or a binary container.
Each would trade away the property the format exists for — that the bytes on
disk are the document, readable in any editor and diffable by any tool.

These belong in a layer above: a `.xtxt` inside an encrypted volume, a
signature alongside the file, history in the version control system that
already does it well. The format's job is to be worth putting there.

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
closing fence.

That scan MUST be unbounded: it continues to the end of the document or until
the fence is found. An earlier version of this section allowed an implementation
to cap the scan and treat "not found within N lines" as inline, with `N` left to
the implementation. That was a defect. Two conformant parsers choosing different
caps would produce different trees for identical bytes, which is precisely the
dialect problem this format exists to avoid — and no implementation ever used
the licence.

A parser that wants to stream must therefore buffer from the opening line until
the fence resolves. In the worst case that is the rest of the document, which is
the price of the guarantee that the same bytes parse the same way everywhere.
