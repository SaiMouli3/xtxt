package xtxt

// Canonical converts a document to the normalised shape used by the
// conformance suite. Every field is present with an explicit default, so an
// implementation cannot pass by omitting something the reference emits — and
// two implementations cannot appear to differ merely because one of them
// leaves empty fields out of its JSON.
func Canonical(doc *Document) map[string]any {
	nodes := make([]any, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		args := make([]any, 0, len(n.Args))
		for _, a := range n.Args {
			args = append(args, map[string]any{"key": a.Key, "value": a.Value})
		}
		items := make([]any, 0, len(n.Items))
		for _, it := range n.Items {
			var checked any
			if it.Checked != nil {
				checked = *it.Checked
			}
			items = append(items, map[string]any{
				"text": it.Text, "ordered": it.Ordered, "checked": checked,
			})
		}
		nodes = append(nodes, map[string]any{
			"kind":  string(n.Kind),
			"name":  n.Name,
			"level": n.Level,
			"text":  n.Text,
			"args":  args,
			"items": items,
			"line":  n.Line,
		})
	}
	return map[string]any{"version": doc.Version, "nodes": nodes}
}

// CanonicalIssues normalises diagnostics for the conformance suite. Messages
// are implementation wording and are deliberately excluded: what every
// implementation must agree on is *that* a line is an error, not how it phrases
// the complaint.
func CanonicalIssues(issues []Issue) []any {
	out := make([]any, 0, len(issues))
	for _, i := range SortIssues(issues) {
		out = append(out, map[string]any{
			"severity": string(i.Severity),
			"line":     i.Line,
		})
	}
	return out
}
