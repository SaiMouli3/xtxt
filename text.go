package xtxt

import (
	"fmt"
	"strings"
)

// TextOptions controls RenderText.
type TextOptions struct {
	Width int  // wrap column; 0 means 80
	Color bool // emit ANSI styling
}

// RenderText renders a document for a terminal.
func RenderText(doc *Document, opt TextOptions) string {
	if opt.Width <= 0 {
		opt.Width = 80
	}
	c := ansi{on: opt.Color}
	var b strings.Builder
	for _, n := range doc.Nodes {
		switch n.Kind {
		case KindHeading:
			b.WriteString("\n" + c.bold(strings.ToUpper(InlineText(n.Text))) + "\n")
			b.WriteString(c.dim(strings.Repeat("=", min(len(InlineText(n.Text)), opt.Width))) + "\n\n")
		case KindParagraph:
			b.WriteString(wrap(InlineText(n.Text), opt.Width) + "\n\n")
		case KindQuote:
			for _, l := range strings.Split(wrap(InlineText(n.Text), opt.Width-2), "\n") {
				b.WriteString(c.dim("│ ") + l + "\n")
			}
			b.WriteString("\n")
		case KindList:
			for i, it := range n.Items {
				bullet := "•"
				if it.Ordered {
					bullet = fmt.Sprintf("%d.", i+1)
				}
				if it.Checked != nil {
					bullet = "[ ]"
					if *it.Checked {
						bullet = "[x]"
					}
				}
				b.WriteString("  " + bullet + " " + indentRest(wrap(InlineText(it.Text), opt.Width-6), 6) + "\n")
			}
			b.WriteString("\n")
		case KindDirective, KindBlock:
			b.WriteString(renderDirectiveText(n, opt, c))
		}
	}
	return strings.TrimLeft(b.String(), "\n")
}

func renderDirectiveText(n Node, opt TextOptions, c ansi) string {
	switch n.Name {
	case "comment", "metadata":
		return ""
	case "hr":
		return c.dim(strings.Repeat("─", opt.Width)) + "\n\n"
	case "image", "video", "audio", "attachment":
		icon := map[string]string{"image": "🖼", "video": "▶", "audio": "♪", "attachment": "📎"}[n.Name]
		label := n.Args.Get("caption")
		if label == "" {
			label = n.Args.Resolve("src")
		}
		return c.dim(fmt.Sprintf("  %s %s  (%s)", icon, InlineText(label), n.Args.Resolve("src"))) + "\n\n"
	case "code":
		var b strings.Builder
		if lang := n.Args.Resolve("language"); lang != "" {
			b.WriteString(c.dim("  ┌─ "+lang) + "\n")
		}
		for _, l := range strings.Split(n.Text, "\n") {
			b.WriteString(c.dim("  │ ") + l + "\n")
		}
		return b.String() + "\n"
	case "math":
		return indentAll(n.Text, 4) + "\n\n"
	case "mermaid":
		return c.dim("  [diagram]") + "\n" + indentAll(n.Text, 4) + "\n\n"
	case "table":
		return renderTableText(n, c)
	case "raw":
		return n.Text + "\n\n"
	case "chart":
		return renderChartText(ParseChart(n), opt, c)
	case "footnote":
		id := n.Args.Resolve("id")
		return c.dim("  ["+id+"] ") + InlineText(n.Text) + "\n\n"
	default:
		if f := n.Fields(); len(f) > 0 {
			var b strings.Builder
			b.WriteString(c.dim("  "+strings.ToUpper(n.Name)) + "\n")
			for _, e := range f {
				key := e.Key
				if key == "" {
					b.WriteString("    " + indentRest(wrap(InlineText(e.Value), opt.Width-4), 4) + "\n")
					continue
				}
				b.WriteString("    " + c.dim(key+": ") +
					indentRest(wrap(InlineText(e.Value), opt.Width-6), 6) + "\n")
			}
			return b.String() + "\n"
		}
		return c.dim("  ["+n.Name+"]") + "\n\n"
	}
}

func renderTableText(n Node, c ansi) string {
	t := ParseTable(n)
	if len(t.Header) == 0 {
		return ""
	}
	cols := len(t.Header)
	w := make([]int, cols)
	cell := func(row []string, i int) string {
		if i < len(row) {
			return InlineText(row[i])
		}
		return ""
	}
	for i, h := range t.Header {
		w[i] = len([]rune(InlineText(h)))
	}
	for _, r := range t.Rows {
		for i := 0; i < cols; i++ {
			if n := len([]rune(cell(r, i))); n > w[i] {
				w[i] = n
			}
		}
	}
	pad := func(s string, i int) string {
		return s + strings.Repeat(" ", max(0, w[i]-len([]rune(s))))
	}
	var b strings.Builder
	var head, rule []string
	for i, h := range t.Header {
		head = append(head, pad(InlineText(h), i))
		rule = append(rule, strings.Repeat("─", w[i]))
	}
	b.WriteString("  " + c.bold(strings.TrimRight(strings.Join(head, "  "), " ")) + "\n")
	b.WriteString("  " + c.dim(strings.Join(rule, "──")) + "\n")
	for _, r := range t.Rows {
		var out []string
		for i := 0; i < cols; i++ {
			out = append(out, pad(cell(r, i), i))
		}
		b.WriteString("  " + strings.TrimRight(strings.Join(out, "  "), " ") + "\n")
	}
	return b.String() + "\n"
}

// renderChartText draws a chart with block characters. A terminal has no
// colour worth relying on, so every bar is direct-labelled with its value.
func renderChartText(ch Chart, opt TextOptions, c ansi) string {
	if len(ch.Labels) == 0 || len(ch.Series) == 0 {
		return ""
	}
	labelW := 0
	for _, l := range ch.Labels {
		labelW = max(labelW, len([]rune(l)))
	}
	max_ := 0.0
	for _, s := range ch.Series {
		for _, v := range s.Values {
			max_ = maxf(max_, v)
		}
	}
	if max_ <= 0 {
		max_ = 1
	}
	valueW := 0
	for _, s := range ch.Series {
		for _, v := range s.Values {
			valueW = max(valueW, len(formatNumber(v, ch.Unit)))
		}
	}
	barW := max(10, opt.Width-labelW-valueW-8)

	var b strings.Builder
	if ch.Title != "" {
		b.WriteString("  " + c.bold(ch.Title) + "\n")
	}
	for i, label := range ch.Labels {
		for si, s := range ch.Series {
			name := label
			if len(ch.Series) > 1 {
				name = label + seriesSuffix(s.Name)
			}
			if si > 0 {
				name = strings.Repeat(" ", len([]rune(label))) + seriesSuffix(s.Name)
			}
			filled := int(float64(barW) * s.Values[i] / max_)
			b.WriteString("  " + name + strings.Repeat(" ", max(0, labelW-len([]rune(name)))) + " " +
				strings.Repeat("█", filled) + " " + formatNumber(s.Values[i], ch.Unit) + "\n")
		}
	}
	return b.String() + "\n"
}

type ansi struct{ on bool }

func (a ansi) bold(s string) string { return a.wrap("\x1b[1m", s) }
func (a ansi) dim(s string) string  { return a.wrap("\x1b[2m", s) }
func (a ansi) wrap(code, s string) string {
	if !a.on {
		return s
	}
	return code + s + "\x1b[0m"
}

func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case len([]rune(line))+1+len([]rune(word)) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func indentAll(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func indentRest(s string, n int) string {
	return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(" ", n))
}
