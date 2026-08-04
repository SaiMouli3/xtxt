# Charts from tables, and an interactive view of them

Status: approved, not yet implemented
Date: 2026-08-04

## The ask

Tables should be able to render as charts, updating as rows are edited; the
author should be able to pick the axes, the values and the chart type; and the
resulting chart should respond to a reader.

## Shape of the work

Three pieces, in dependency order.

1. **`@table(chart=…)`** — the format change. Delivers "add a row, the chart
   updates".
2. **A picker in VS Code** — delivers "choose the axes, values and type".
3. **A shared interactive runtime** — delivers hover, series toggling and
   type switching for a reader.

## What the survey changed

Two findings moved the design before a line was written.

**Only two of seven implementations render charts.** Go (`chart.go`,
`html.go`) and the JavaScript SDK (`chart.js`) do. C, C++, Java, Python and
Rust parse only. Directive arguments are parsed generically by all of them, so
`@table(chart="bar")` reaches every SDK's AST with no code change in five of
them.

**Conformance pins parse shape and issues, not rendering.** So a new fixture
costs nothing across the five parse-only SDKs *provided it raises no new
issue* — but any new `validate()` warning would have to be implemented seven
times, because those SDKs do implement semantic validation (Python already
emits `@chart has no readable data rows`).

That produces the central scoping decision below.

## Decisions

**No new validation issues in v1.** Chart problems — an unknown column name, a
non-numeric cell, an unrecognised chart type — surface as renderer warnings in
Go and JS, not as `validate()` issues. Conformance therefore stays green across
all seven SDKs with changes to only two.

The cost is honest and worth naming: a typo in `x="Moth"` will not produce a
squiggle in the editor, only a warning in the preview. If that proves annoying
in use, promoting these to real diagnostics is a separate change across seven
implementations, and should be its own spec.

**The runtime is an asset, not an implementation.** One checked-in JavaScript
file. Go embeds it with `//go:embed`; the JS SDK imports it as a string. Same
bytes on both sides, so the two renderers cannot drift, and its behaviour is
tested once, in JavaScript. Go never writes JavaScript.

**Export stays script-free by default.** `xtxt export html --interactive` opts
in. The property that an exported file is safe to embed or open from an
untrusted source is worth keeping as the default.

## 1. Format

```
@table(chart="bar", x="Month", y="Signups, Revenue", title="Growth", unit="users")
Month | Signups | Revenue
------|---------|--------
Jan   | 20      | 1200
Feb   | 35      | 2400
@endtable
```

| Arg | Meaning | Default |
|---|---|---|
| `chart` | `bar`, `line`, `area`, `stacked` or `pie` | absent — table only |
| `x` | header of the label column | first column |
| `y` | comma-separated headers of value columns | every non-`x` column parsing as numeric |
| `title` | chart title | none |
| `unit` | value unit | none |

`chart` both enables the chart and names its type. Without it, behaviour is
exactly what it is today.

**Semantics.** A non-numeric cell is a gap, not a zero — zero is a claim the
document did not make. An unknown column in `x` or `y`, an unrecognised chart
type, or a table with no numeric column each fall back (to the default column,
to `bar`, to table-only) and record a renderer warning.

**Compatibility.** A reader that does not know these arguments ignores them and
renders the table, which is the format's standing guarantee that adding a name
never breaks an old reader.

**Rendering.** Chart first, then the full table, mirroring the existing
`renderChart` shape. Because the table stays visible, SPEC §5.5's requirement
that the numbers be reachable as text is met structurally.

**Surface.** A `Table → Chart` adapter of roughly thirty lines in Go and in JS;
the existing `renderChartSVG` does all drawing. One conformance fixture pinning
the AST with an empty issue list.

## 2. Picker

A CodeLens reading `Chart this table` above every `@table`, plus a command
`XTXT: Chart this table`. Both open a QuickPick chain: chart type, then the
label column, then the value columns (multi-select). The result is written as
arguments on the `@table(…)` line, creating or replacing them.

Finding the enclosing table reuses `blockRanges` from the folding and outline
work. A QuickPick chain avoids building a webview form for what is three
choices.

## 3. Interactive runtime

Progressive enhancement over what already renders. The static SVG and the
visible table are the document; the runtime upgrades them in place. With
JavaScript unavailable, nothing is lost and §5.5 still holds.

The renderer emits the chart's numbers as JSON in a `data-chart` attribute on
the figure. The runtime finds each figure and adds:

- a hover readout of the value under the pointer,
- legend entries that toggle a series,
- a control that switches between the chart types valid for the data.

Hosts:

- **VS Code preview** — `enableScripts: true` with a per-render CSP nonce, so
  the runtime executes and a `<script>` injected through `@raw(format="html")`
  does not, since it cannot carry the nonce.
- **`xtxt export html --interactive`** — runtime inlined; without the flag,
  output is unchanged and script-free.
- **Browser demo** — always on.

**Testing.** The runtime's logic is written as pure functions — which series
are visible, what the readout says, which types are offered — tested with
`node --test`. The DOM glue stays thin and deliberately untested rather than
introducing a browser harness for it.

## Error handling

Every failure degrades to the table, which is always rendered. A malformed
`chart` argument, an unresolvable column, a row whose cells do not parse: the
table is still correct and complete, and a warning explains the missing chart.
No input produces an error, consistent with the format's recovery guarantee.

## Out of scope

Editing data by dragging a chart. Chart-type-specific options such as axis
bounds or colour overrides. Promoting renderer warnings to `validate()`
issues. Any change to the five parse-only SDKs.
