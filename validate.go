package xtxt

import "strings"

// known lists the standard directives from SPEC.md §5 and whether each is a
// fenced block. Anything absent is an unknown directive: a warning, never an
// error, so that a 1.0 reader stays usable on a newer document.
var known = map[string]bool{
	"xtxt": false, "image": false, "video": false, "audio": false,
	"attachment": false, "include": false, "hr": false,
	"code": true, "table": true, "math": true, "mermaid": true,
	"metadata": true, "comment": true, "raw": true,
}

// requiredSrc lists directives that are meaningless without a source.
var requiredSrc = map[string]bool{
	"image": true, "video": true, "audio": true, "attachment": true, "include": true,
}

// Validate returns semantic issues on top of the parser's syntactic ones.
func Validate(doc *Document) []Issue {
	var issues []Issue
	warn := func(line int, msg string) {
		issues = append(issues, Issue{Severity: Warning, Line: line, Message: msg})
	}

	metadataSeen := false
	for _, n := range doc.Nodes {
		if n.Kind != KindDirective && n.Kind != KindBlock {
			continue
		}
		fenced, ok := known[n.Name]
		if !ok {
			warn(n.Line, "unknown directive @"+n.Name+" (preserved, but this reader cannot render it)")
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
