package xtxt

import (
	_ "embed"
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

// defaultCSS is the stylesheet emitted with a standalone HTML document. It
// lives in a real .css file so the browser demo and the CLI cannot drift apart:
// there is one copy, and this embeds it.
//
//go:embed assets/xtxt.css
var defaultCSS string
