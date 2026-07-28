package xtxt

import "strings"

// known lists the standard directives from SPEC.md §5 and whether each is a
// fenced block. Anything absent is an unknown directive: a warning, never an
// error, so that a 1.0 reader stays usable on a newer document.
var known = map[string]bool{
	"xtxt": false, "image": false, "video": false, "audio": false,
	"attachment": false, "include": false, "embed": false, "hr": false,
	"code": true, "table": true, "math": true, "mermaid": true,
	"metadata": true, "comment": true, "raw": true, "chart": true,
	"footnote": true,
	// Semantic blocks: structure an agent can read without inferring it from
	// prose. The core validates their shape and renders them; it deliberately
	// does not attach meaning to their field names.
	"task": true, "decision": true, "knowledge": true,
	"ai": true, "prompt": true, "chat": true, "note": true,
}

// requiredSrc lists directives that are meaningless without a source.
var requiredSrc = map[string]bool{
	"image": true, "video": true, "audio": true, "attachment": true, "include": true,
}

// Validate returns semantic issues on top of the parser's syntactic ones.
//
// Names of directives supplied by plugins may be passed in `declared`: a plugin
// manifest is a declaration that the directive exists, so a document using one
// should not be warned about as if the name were a typo.
func Validate(doc *Document, declared ...string) []Issue {
	var issues []Issue
	extra := make(map[string]bool, len(declared))
	for _, name := range declared {
		extra[name] = true
	}
	warn := func(line int, msg string) {
		issues = append(issues, Issue{Severity: Warning, Line: line, Message: msg})
	}

	metadataSeen := false
	seenNotes := map[string]int{}
	for _, n := range doc.Nodes {
		if n.Kind != KindDirective && n.Kind != KindBlock {
			continue
		}
		fenced, ok := known[n.Name]
		if !ok {
			if !extra[n.Name] {
				warn(n.Line, "unknown directive @"+n.Name+" (preserved, but this reader cannot render it)")
			}
			continue
		}
		if fenced && n.Kind != KindBlock {
			warn(n.Line, "@"+n.Name+" is a block directive and should be closed with @end"+n.Name)
		}
		if !fenced && n.Kind == KindBlock {
			warn(n.Line, "@"+n.Name+" is not a block directive but was closed with @end"+n.Name)
		}
		if requiredSrc[n.Name] && n.Args.Resolve("src") == "" {
			warn(n.Line, "@"+n.Name+" has no src")
		}
		switch n.Name {
		case "metadata":
			if metadataSeen {
				warn(n.Line, "duplicate @metadata block")
			}
			metadataSeen = true
		case "table":
			issues = append(issues, checkTable(n)...)
		case "code":
			if n.Args.Resolve("language") == "" {
				warn(n.Line, "@code has no language; syntax highlighting will be skipped")
			}
		case "chart":
			c := ParseChart(n)
			if len(c.Labels) == 0 {
				warn(n.Line, "@chart has no readable data rows")
			}
			for _, w := range c.Warnings {
				warn(n.Line, w)
			}
		case "task":
			if n.Fields().Get("title") == "" {
				warn(n.Line, "@task has no Title field")
			}
		case "footnote":
			if n.Args.Resolve("id") == "" {
				warn(n.Line, "@footnote has no id; references cannot point at it")
			}
			seenNotes[n.Args.Resolve("id")] = n.Line
		}
	}
	issues = append(issues, checkFootnoteRefs(doc, seenNotes)...)
	return issues
}

// checkFootnoteRefs pairs [^id] markers with @footnote blocks in both
// directions: a marker with no note renders as a dead link, and a note nobody
// cites is usually a leftover.
func checkFootnoteRefs(doc *Document, notes map[string]int) []Issue {
	var issues []Issue
	cited := map[string]bool{}
	visit := func(text string, line int) {
		for i := 0; i < len(text); i++ {
			if text[i] == '\\' {
				i++
				continue
			}
			if id, end, ok := footnoteRef(text, i); ok {
				cited[id] = true
				if _, exists := notes[id]; !exists {
					issues = append(issues, Issue{Warning, line,
						"footnote reference [^" + id + "] has no matching @footnote(id=\"" + id + "\")"})
				}
				i = end
			}
		}
	}
	for _, n := range doc.Nodes {
		switch n.Kind {
		case KindHeading, KindParagraph, KindQuote:
			visit(n.Text, n.Line)
		case KindList:
			for _, it := range n.Items {
				visit(it.Text, n.Line)
			}
		}
	}
	for id, line := range notes {
		if id != "" && !cited[id] {
			issues = append(issues, Issue{Warning, line, "@footnote(id=\"" + id + "\") is never referenced"})
		}
	}
	return issues
}

func checkTable(n Node) []Issue {
	t := ParseTable(n)
	if len(t.Header) == 0 {
		return []Issue{{Severity: Warning, Line: n.Line, Message: "@table is empty"}}
	}
	var issues []Issue
	offset := 1
	for i, row := range t.Rows {
		if len(row) != len(t.Header) {
			issues = append(issues, Issue{
				Severity: Warning,
				Line:     n.Line + offset + i,
				Message:  "table row has " + itoa(len(row)) + " cells, header has " + itoa(len(t.Header)),
			})
		}
	}
	return issues
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// SortIssues orders issues by line for stable reporting.
func SortIssues(in []Issue) []Issue {
	out := append([]Issue(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Line < out[j-1].Line; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Lint reports stylistic issues that do not affect meaning.
func Lint(doc *Document) []Issue {
	var issues []Issue
	for _, n := range doc.Nodes {
		switch {
		case n.Kind == KindHeading && strings.HasSuffix(n.Text, ":"):
			issues = append(issues, Issue{Warning, n.Line, "heading ends with a colon"})
		case n.Kind == KindDirective && n.Name == "image" && n.Args.Get("alt") == "" && n.Args.Get("caption") == "":
			issues = append(issues, Issue{Warning, n.Line, "@image has neither alt nor caption"})
		case n.Kind == KindParagraph && strings.Contains(n.Text, "  "):
			issues = append(issues, Issue{Warning, n.Line, "paragraph contains double spaces"})
		}
	}
	return issues
}
