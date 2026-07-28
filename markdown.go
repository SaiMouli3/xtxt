package xtxt

import (
	"fmt"
	"regexp"
	"strings"
)

// RenderMarkdown converts a document to CommonMark. Constructs Markdown has no
// equivalent for (math, mermaid, unknown directives) become fenced code blocks
// tagged with the directive name, which is what most Markdown ecosystems already
// expect.
func RenderMarkdown(doc *Document) string {
	var b strings.Builder
	if m := doc.Metadata(); len(m) > 0 {
		b.WriteString("---\n")
		for _, k := range sortedKeys(m) {
			fmt.Fprintf(&b, "%s: %s\n", k, m[k])
		}
		b.WriteString("---\n\n")
	}
	for _, n := range doc.Nodes {
		switch n.Kind {
		case KindHeading:
			fmt.Fprintf(&b, "%s %s\n\n", strings.Repeat("#", n.Level), n.Text)
		case KindParagraph:
			b.WriteString(n.Text + "\n\n")
		case KindQuote:
			b.WriteString("> " + n.Text + "\n\n")
		case KindList:
			for i, it := range n.Items {
				marker := "-"
				if it.Ordered {
					marker = fmt.Sprintf("%d.", i+1)
				}
				box := ""
				if it.Checked != nil {
					box = "[ ] "
					if *it.Checked {
						box = "[x] "
					}
				}
				fmt.Fprintf(&b, "%s %s%s\n", marker, box, it.Text)
			}
			b.WriteString("\n")
		case KindDirective, KindBlock:
			b.WriteString(directiveMarkdown(n))
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func directiveMarkdown(n Node) string {
	switch n.Name {
	case "comment":
		return "<!-- " + strings.ReplaceAll(n.Text, "--", "- -") + " -->\n\n"
	case "metadata":
		return "" // emitted as front matter
	case "hr":
		return "---\n\n"
	case "image":
		alt := altText(n)
		out := fmt.Sprintf("![%s](%s)\n", alt, n.Args.Resolve("src"))
		if c := n.Args.Get("caption"); c != "" && c != alt {
			out += "\n*" + c + "*\n"
		}
		return out + "\n"
	case "video", "audio", "attachment":
		label := n.Args.Get("caption")
		if label == "" {
			label = n.Args.Resolve("src")
		}
		return fmt.Sprintf("[%s](%s)\n\n", label, n.Args.Resolve("src"))
	case "table":
		return tableMarkdown(n)
	case "raw":
		if n.Args.Resolve("format") == "html" {
			return n.Text + "\n\n"
		}
		return fence("", n.Text)
	case "code":
		return fence(n.Args.Resolve("language"), n.Text)
	case "math":
		return "$$\n" + n.Text + "\n$$\n\n"
	default:
		return fence(n.Name, n.Text)
	}
}

func fence(lang, body string) string {
	ticks := "```"
	for strings.Contains(body, ticks) {
		ticks += "`"
	}
	return ticks + lang + "\n" + body + "\n" + ticks + "\n\n"
}

func tableMarkdown(n Node) string {
	t := ParseTable(n)
	if len(t.Header) == 0 {
		return ""
	}
	row := func(cells []string) string {
		out := make([]string, len(t.Header))
		for i := range out {
			if i < len(cells) {
				out[i] = strings.ReplaceAll(cells[i], "|", `\|`)
			}
		}
		return "| " + strings.Join(out, " | ") + " |\n"
	}
	var b strings.Builder
	b.WriteString(row(t.Header))
	seps := make([]string, len(t.Header))
	for i := range seps {
		seps[i] = "---"
		if i < len(t.Align) {
			switch t.Align[i] {
			case "right":
				seps[i] = "--:"
			case "center":
				seps[i] = ":-:"
			}
		}
	}
	b.WriteString("| " + strings.Join(seps, " | ") + " |\n")
	for _, r := range t.Rows {
		b.WriteString(row(r))
	}
	return b.String() + "\n"
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

var (
	reFence     = regexp.MustCompile("^\\s*(`{3,}|~{3,})\\s*([A-Za-z0-9_+-]*)\\s*$")
	reMdImage   = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)\s*$`)
	reFrontRule = regexp.MustCompile(`^---+\s*$`)
	reMdTable   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
)

// FromMarkdown converts CommonMark to XTXT source. Text constructs pass through
// unchanged — the formats agree on those — so only fences, tables, images and
// front matter are rewritten.
func FromMarkdown(src string) string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var b strings.Builder
	i := 0

	if len(lines) > 0 && reFrontRule.MatchString(lines[0]) {
		if end := indexMatch(lines, 1, reFrontRule); end > 0 {
			b.WriteString("@metadata\n")
			for _, l := range lines[1:end] {
				if k, v, ok := strings.Cut(l, ":"); ok {
					fmt.Fprintf(&b, "%s = %s\n", strings.TrimSpace(k), strings.TrimSpace(v))
				}
			}
			b.WriteString("@endmetadata\n\n")
			i = end + 1
		}
	}

	for ; i < len(lines); i++ {
		line := lines[i]
		if m := reFence.FindStringSubmatch(line); m != nil {
			lang, close := m[2], m[1]
			end := i + 1
			for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), close) {
				end++
			}
			body := strings.Join(lines[i+1:min(end, len(lines))], "\n")
			switch lang {
			case "mermaid", "math":
				fmt.Fprintf(&b, "@%s\n%s\n@end%s\n", lang, body, lang)
			case "":
				fmt.Fprintf(&b, "@code\n%s\n@endcode\n", body)
			default:
				fmt.Fprintf(&b, "@code(language=%q)\n%s\n@endcode\n", lang, body)
			}
			i = end
			continue
		}
		if m := reMdImage.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			fmt.Fprintf(&b, "@image(src=%q, alt=%q)\n", m[2], m[1])
			continue
		}
		if reMdTable.MatchString(line) {
			end := i
			for end < len(lines) && reMdTable.MatchString(lines[end]) {
				end++
			}
			b.WriteString("@table\n")
			for _, r := range lines[i:end] {
				cells := splitCells(r)
				if isSeparatorRow(cells) {
					b.WriteString(strings.Join(cells, " | ") + "\n")
					continue
				}
				b.WriteString(strings.Join(cells, " | ") + "\n")
			}
			b.WriteString("@endtable\n")
			i = end - 1
			continue
		}
		if strings.HasPrefix(line, "@") {
			b.WriteString("\\" + line + "\n")
			continue
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func indexMatch(lines []string, from int, re *regexp.Regexp) int {
	for i := from; i < len(lines); i++ {
		if re.MatchString(lines[i]) {
			return i
		}
	}
	return -1
}
