package xtxt

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// HTMLOptions controls RenderHTML.
type HTMLOptions struct {
	// Full wraps the output in a complete HTML document with default styling.
	Full bool
	// Title overrides the document title; defaults to metadata title or the
	// first heading.
	Title string
	// Mermaid includes the mermaid.js loader from a CDN when the document has
	// diagrams. Off by default so output stays self-contained and offline.
	Mermaid bool
	// Plugins render directives the core does not know about. Anything still
	// unknown after this falls back to a visible placeholder.
	Plugins Plugins
}

// RenderHTML renders a document to HTML.
func RenderHTML(doc *Document, opt HTMLOptions) string {
	var b strings.Builder
	body := renderBody(doc, opt)
	if !opt.Full {
		return body
	}
	title := opt.Title
	if title == "" {
		title = documentTitle(doc)
	}
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s</style>\n", html.EscapeString(title), defaultCSS)
	if opt.Mermaid && hasDirective(doc, "mermaid") {
		b.WriteString(`<script type="module">import m from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";m.initialize({startOnLoad:true});</script>` + "\n")
	}
	b.WriteString("</head>\n<body>\n<main class=\"xtxt\">\n")
	b.WriteString(body)
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.String()
}

func documentTitle(doc *Document) string {
	if m := doc.Metadata(); m["title"] != "" {
		return m["title"]
	}
	for _, n := range doc.Nodes {
		if n.Kind == KindHeading {
			return InlineText(n.Text)
		}
	}
	return "Untitled"
}

func hasDirective(doc *Document, name string) bool {
	for _, n := range doc.Nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

func renderBody(doc *Document, opt HTMLOptions) string {
	var b strings.Builder
	var notes []Node
	for _, n := range doc.Nodes {
		switch n.Kind {
		case KindHeading:
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", n.Level, InlineHTML(n.Text), n.Level)
		case KindParagraph:
			fmt.Fprintf(&b, "<p>%s</p>\n", InlineHTML(n.Text))
		case KindQuote:
			fmt.Fprintf(&b, "<blockquote><p>%s</p></blockquote>\n", InlineHTML(n.Text))
		case KindList:
			b.WriteString(renderList(n))
		case KindDirective, KindBlock:
			if n.Name == "footnote" {
				notes = append(notes, n)
				continue
			}
			if out, ok := opt.Plugins.render(n); ok {
				b.WriteString(out + "\n")
				continue
			}
			b.WriteString(renderDirectiveHTML(n))
		}
	}
	b.WriteString(renderFootnotes(notes))
	return b.String()
}

func renderFootnotes(notes []Node) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<section class=\"footnotes\">\n<ol>\n")
	for i, n := range notes {
		id := n.Args.Resolve("id")
		if id == "" {
			id = itoa(i + 1)
		}
		e := html.EscapeString(id)
		value := ""
		if _, err := strconv.Atoi(id); err == nil {
			value = ` value="` + e + `"`
		}
		fmt.Fprintf(&b, "<li id=\"fn-%s\"%s>%s <a class=\"fnback\" href=\"#fnref-%s\">&#8617;</a></li>\n",
			e, value, InlineHTML(n.Text), e)
	}
	b.WriteString("</ol>\n</section>\n")
	return b.String()
}

func renderList(n Node) string {
	if len(n.Items) == 0 {
		return ""
	}
	tag, class := "ul", ""
	if n.Items[0].Ordered {
		tag = "ol"
	}
	if n.Items[0].Checked != nil {
		class = ` class="checklist"`
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s%s>\n", tag, class)
	for _, it := range n.Items {
		b.WriteString("<li>")
		if it.Checked != nil {
			checked := ""
			if *it.Checked {
				checked = " checked"
			}
			fmt.Fprintf(&b, `<input type="checkbox" disabled%s> `, checked)
		}
		b.WriteString(InlineHTML(it.Text) + "</li>\n")
	}
	fmt.Fprintf(&b, "</%s>\n", tag)
	return b.String()
}

func renderDirectiveHTML(n Node) string {
	esc := html.EscapeString
	switch n.Name {
	case "comment":
		return ""
	case "metadata":
		return "" // surfaced as document properties, not inline content
	case "hr":
		return "<hr>\n"
	case "image":
		var b strings.Builder
		b.WriteString("<figure>")
		fmt.Fprintf(&b, `<img src="%s" alt="%s"%s%s>`,
			esc(n.Args.Resolve("src")), esc(altText(n)),
			attr("width", n.Args.Get("width")), attr("height", n.Args.Get("height")))
		if c := n.Args.Get("caption"); c != "" {
			fmt.Fprintf(&b, "<figcaption>%s</figcaption>", InlineHTML(c))
		}
		b.WriteString("</figure>\n")
		return b.String()
	case "video":
		return media("video", n, ` controls playsinline`)
	case "audio":
		return media("audio", n, ` controls`)
	case "attachment":
		src := n.Args.Resolve("src")
		name := n.Args.Get("name")
		if name == "" {
			name = src
		}
		return fmt.Sprintf("<p class=\"attachment\"><a href=\"%s\" download>%s</a></p>\n", esc(src), esc(name))
	case "code":
		lang := n.Args.Resolve("language")
		cls := ""
		if lang != "" {
			cls = fmt.Sprintf(` class="language-%s"`, esc(lang))
		}
		return fmt.Sprintf("<pre><code%s>%s</code></pre>\n", cls, esc(n.Text))
	case "math":
		return fmt.Sprintf("<div class=\"math\">%s</div>\n", esc(n.Text))
	case "mermaid":
		return fmt.Sprintf("<pre class=\"mermaid\">%s</pre>\n", esc(n.Text))
	case "raw":
		if n.Args.Resolve("format") == "html" {
			return n.Text + "\n"
		}
		return fmt.Sprintf("<pre>%s</pre>\n", esc(n.Text))
	case "table":
		return renderTableHTML(n)
	case "chart":
		c := ParseChart(n)
		svg := RenderChartSVG(c)
		if svg == "" {
			return ""
		}
		out := "<figure class=\"chart\">" + svg
		if c.Title != "" {
			out += "<figcaption>" + InlineHTML(c.Title) + "</figcaption>"
		}
		// The table view is mandatory, not a nicety: part of the palette sits
		// below 3:1 against the light surface, and print or forced-colors modes
		// may drop the fills entirely.
		return out + chartTableHTML(c) + "</figure>\n"
	default:
		if n.Kind == KindBlock {
			if f := n.Fields(); len(f) > 0 {
				return renderRecordHTML(n, f)
			}
		}
		// Unknown directive: show it rather than dropping it (SPEC §7).
		return fmt.Sprintf("<div class=\"unknown\" data-directive=\"%s\">%s</div>\n",
			esc(n.Name), esc(sourceOf(n)))
	}
}

// renderRecordHTML renders any record-shaped block — @task, @decision,
// @knowledge, @ai, @prompt, @chat and whatever a later version adds — as a
// labelled card. The core does not need to know the directive's meaning to show
// its structure faithfully, which is what keeps §7 honest for semantic blocks.
func renderRecordHTML(n Node, f Fields) string {
	esc := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, "<section class=\"record\" data-type=\"%s\">\n", esc(n.Name))
	fmt.Fprintf(&b, "<h4 class=\"record-type\">%s</h4>\n", esc(n.Name))
	b.WriteString("<dl>\n")
	for _, e := range f {
		key := e.Key
		if key == "" {
			key = "—"
		}
		fmt.Fprintf(&b, "<dt>%s</dt><dd>%s</dd>\n", esc(key), InlineHTML(e.Value))
	}
	b.WriteString("</dl>\n</section>\n")
	return b.String()
}

func altText(n Node) string {
	if a := n.Args.Get("alt"); a != "" {
		return a
	}
	return InlineText(n.Args.Get("caption"))
}

func media(tag string, n Node, extra string) string {
	var b strings.Builder
	b.WriteString("<figure>")
	fmt.Fprintf(&b, `<%s src="%s"%s%s%s></%s>`, tag, html.EscapeString(n.Args.Resolve("src")),
		extra, attr("width", n.Args.Get("width")), attr("height", n.Args.Get("height")), tag)
	if c := n.Args.Get("caption"); c != "" {
		fmt.Fprintf(&b, "<figcaption>%s</figcaption>", InlineHTML(c))
	}
	b.WriteString("</figure>\n")
	return b.String()
}

func attr(name, val string) string {
	if val == "" {
		return ""
	}
	return fmt.Sprintf(` %s="%s"`, name, html.EscapeString(val))
}

func renderTableHTML(n Node) string {
	t := ParseTable(n)
	if len(t.Header) == 0 {
		return ""
	}
	align := func(i int) string {
		if i < len(t.Align) && t.Align[i] != "left" {
			return fmt.Sprintf(` style="text-align:%s"`, t.Align[i])
		}
		return ""
	}
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for i, h := range t.Header {
		fmt.Fprintf(&b, "<th%s>%s</th>", align(i), InlineHTML(h))
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range t.Rows {
		b.WriteString("<tr>")
		for i, c := range row {
			fmt.Fprintf(&b, "<td%s>%s</td>", align(i), InlineHTML(c))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
	if c := n.Args.Get("caption"); c != "" {
		return strings.Replace(b.String(), "<table>\n",
			fmt.Sprintf("<table>\n<caption>%s</caption>\n", InlineHTML(c)), 1)
	}
	return b.String()
}

// sourceOf reconstructs the source text of a directive, used for round-tripping
// and for showing unknown directives.
func sourceOf(n Node) string {
	var b strings.Builder
	b.WriteString("@" + n.Name)
	if len(n.Args) > 0 {
		var parts []string
		for _, a := range n.Args {
			v := a.Value
			if strings.ContainsAny(v, ` ,)"`) || v == "" {
				v = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
			}
			if a.Key != "" {
				v = a.Key + "=" + v
			}
			parts = append(parts, v)
		}
		b.WriteString("(" + strings.Join(parts, ", ") + ")")
	}
	if n.Kind == KindBlock {
		b.WriteString("\n" + n.Text + "\n@end" + n.Name)
	}
	return b.String()
}

const defaultCSS = `
:root { color-scheme: light dark; --fg:#1a1a1a; --bg:#fff; --muted:#666; --rule:#e3e3e3; --code-bg:#f6f6f6; --accent:#3b5bdb; }
@media (prefers-color-scheme: dark) { :root { --fg:#e6e6e6; --bg:#151515; --muted:#9a9a9a; --rule:#333; --code-bg:#1e1e1e; --accent:#8aa2ff; } }
* { box-sizing: border-box; }
body { margin:0; background:var(--bg); color:var(--fg); font:16px/1.65 ui-serif, Georgia, "Times New Roman", serif; }
.xtxt { max-width: 44rem; margin: 0 auto; padding: 3rem 1.25rem 6rem; }
h1,h2,h3,h4,h5,h6 { font-family: ui-sans-serif, system-ui, sans-serif; line-height:1.25; margin:2.2em 0 .6em; }
h1 { font-size:2.1rem; margin-top:0; } h2 { font-size:1.5rem; } h3 { font-size:1.2rem; }
p { margin: 0 0 1.1em; }
a { color: var(--accent); }
blockquote { margin:1.5em 0; padding:.2em 0 .2em 1.2em; border-left:3px solid var(--rule); color:var(--muted); }
ul,ol { padding-left:1.4em; margin:0 0 1.1em; }
ul.checklist { list-style:none; padding-left:.2em; }
ul.checklist input { margin-right:.4em; }
figure { margin:2em 0; text-align:center; }
figure img, figure video { max-width:100%; height:auto; border-radius:4px; }
figcaption { font-size:.85rem; color:var(--muted); margin-top:.6em; font-family:ui-sans-serif,system-ui,sans-serif; }
pre { background:var(--code-bg); padding:1rem; border-radius:6px; overflow-x:auto; }
pre, code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size:.875rem; }
p code, li code, td code { background:var(--code-bg); padding:.12em .35em; border-radius:3px; }
table { border-collapse:collapse; width:100%; margin:2em 0; font-family:ui-sans-serif,system-ui,sans-serif; font-size:.92rem; display:block; overflow-x:auto; }
th,td { border-bottom:1px solid var(--rule); padding:.55em .7em; text-align:left; }
th { font-weight:600; border-bottom-width:2px; }
caption { caption-side:bottom; color:var(--muted); font-size:.85rem; padding-top:.6em; }
hr { border:0; border-top:1px solid var(--rule); margin:3em 0; }
.math { margin:1.6em 0; text-align:center; font-family:ui-serif,Georgia,serif; font-size:1.15rem; font-style:italic; }
.unknown { border:1px dashed var(--rule); border-radius:6px; padding:.8em 1em; margin:1.5em 0; color:var(--muted); white-space:pre-wrap; font-family:ui-monospace,monospace; font-size:.8rem; }
.attachment a::before { content:"\1F4CE\00a0"; }

/* Charts. Categorical slots 1-3 of a palette validated for CVD and
   normal-vision separation on both surfaces; dark steps are selected for the
   dark surface, not flipped from the light ones. */
:root { --chart-1:#2a78d6; --chart-2:#eb6834; --chart-3:#1baf7a; --chart-surface:#fff; --chart-grid:#e3e3e3; }
@media (prefers-color-scheme: dark) { :root { --chart-1:#3987e5; --chart-2:#d95926; --chart-3:#199e70; --chart-surface:#151515; --chart-grid:#333; } }
figure.chart { margin:2em 0; text-align:left; }
.xtxt-chart { display:block; max-width:100%; overflow:visible; font-family:ui-sans-serif,system-ui,sans-serif; }
.xtxt-chart .c-label { font-size:12px; fill:var(--fg); }
.xtxt-chart .c-value { font-size:11px; fill:var(--muted); }
.xtxt-chart .c-inbar { font-size:11px; fill:#fff; font-weight:600; }
.xtxt-chart .c-axis  { font-size:10px; fill:var(--muted); }
.xtxt-chart .c-grid  { stroke:var(--chart-grid); stroke-width:1; }
.chart-data { margin-top:.5em; font-size:.85rem; }
.chart-data summary { cursor:pointer; color:var(--muted); font-family:ui-sans-serif,system-ui,sans-serif; }
.chart-data table { margin:.6em 0 0; }
@media (forced-colors: active) { .xtxt-chart path, .xtxt-chart rect, .xtxt-chart polyline { forced-color-adjust:none; } }

/* Record blocks: @task, @decision, @knowledge, @ai, @chat and friends. */
.record { border:1px solid var(--rule); border-left:3px solid var(--accent); border-radius:6px; padding:.9em 1.1em; margin:1.8em 0; }
.record-type { font-family:ui-sans-serif,system-ui,sans-serif; font-size:.7rem; letter-spacing:.09em; text-transform:uppercase; color:var(--muted); margin:0 0 .6em; font-weight:600; }
.record dl { display:grid; grid-template-columns:minmax(4.5rem,max-content) 1fr; gap:.35em .9em; margin:0; font-size:.94rem; }
.record dt { font-family:ui-sans-serif,system-ui,sans-serif; font-size:.8rem; color:var(--muted); padding-top:.15em; }
.record dd { margin:0; white-space:pre-wrap; }

/* Footnotes. */
.fnref a { text-decoration:none; padding:0 .1em; }
.footnotes { margin-top:3em; padding-top:1.2em; border-top:1px solid var(--rule); font-size:.88rem; color:var(--muted); }
.footnotes li { margin-bottom:.4em; }
.fnback { text-decoration:none; }
`
