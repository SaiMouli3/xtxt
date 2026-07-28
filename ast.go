// Package xtxt parses and renders the XTXT plain-text document format.
// See SPEC.md for the format definition.
package xtxt

// Kind identifies the type of a Node.
type Kind string

const (
	KindHeading   Kind = "heading"
	KindParagraph Kind = "paragraph"
	KindQuote     Kind = "quote"
	KindList      Kind = "list"
	KindDirective Kind = "directive" // inline form: @name(...)
	KindBlock     Kind = "block"     // fenced form: @name ... @endname
)

// Node is a single block in the document tree. XTXT 1.0 has no nested blocks,
// so a document is a flat slice of these; Items and Rows carry the sub-structure
// that lists and tables need.
type Node struct {
	Kind  Kind   `json:"kind"`
	Name  string `json:"name,omitempty"`  // directive name, for KindDirective/KindBlock
	Level int    `json:"level,omitempty"` // heading depth 1-6
	Text  string `json:"text,omitempty"`  // heading/paragraph/quote text, or fenced payload
	Args  Args   `json:"args,omitempty"`
	Items []Item `json:"items,omitempty"` // list items
	Line  int    `json:"line"`            // 1-based line where the node starts
}

// Item is one entry in a list. Checked is nil unless the item is a checklist entry.
type Item struct {
	Text    string `json:"text"`
	Ordered bool   `json:"ordered,omitempty"`
	Checked *bool  `json:"checked,omitempty"`
}

// Args holds a directive's arguments in source order. Duplicate keys keep the
// first occurrence when looked up, but all are preserved for round-tripping.
type Args []Arg

// Arg is one argument. Key is empty for a positional argument.
type Arg struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value"`
}

// Get returns the value for key, or "" if absent.
func (a Args) Get(key string) string {
	for _, arg := range a {
		if arg.Key == key {
			return arg.Value
		}
	}
	return ""
}

// Has reports whether key was supplied.
func (a Args) Has(key string) bool {
	for _, arg := range a {
		if arg.Key == key {
			return true
		}
	}
	return false
}

// Positional returns the i-th argument that had no key.
func (a Args) Positional(i int) string {
	n := 0
	for _, arg := range a {
		if arg.Key == "" {
			if n == i {
				return arg.Value
			}
			n++
		}
	}
	return ""
}

// Resolve returns Get(key), falling back to the first positional argument.
// This is what makes @video("x.mp4") and @video(src="x.mp4") equivalent.
func (a Args) Resolve(key string) string {
	if v := a.Get(key); v != "" {
		return v
	}
	return a.Positional(0)
}

// Document is a parsed XTXT file.
type Document struct {
	Version string `json:"version,omitempty"`
	Nodes   []Node `json:"nodes"`
}

// Metadata returns the key/value pairs of the document's @metadata block,
// with keys lowercased. Returns nil if there is no metadata block.
func (d *Document) Metadata() map[string]string {
	for _, n := range d.Nodes {
		if n.Kind == KindBlock && n.Name == "metadata" {
			return parseMetadata(n.Text)
		}
	}
	return nil
}

// Table is the interpreted payload of an @table block.
type Table struct {
	Header []string
	Rows   [][]string
	Align  []string // "left", "right" or "center" per column; empty means left
}
