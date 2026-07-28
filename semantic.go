package xtxt

import "strings"

// Field is one `Key: value` entry in a block payload.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Fields is a block payload interpreted as an ordered record. Order matters:
// a @chat block's turns are fields, and their sequence is the conversation.
type Fields []Field

// Get returns the first value whose key matches, case-insensitively.
func (f Fields) Get(key string) string {
	for _, e := range f {
		if strings.EqualFold(e.Key, key) {
			return e.Value
		}
	}
	return ""
}

// Map flattens to a lowercase-keyed map, keeping the first of any duplicate.
func (f Fields) Map() map[string]string {
	m := make(map[string]string, len(f))
	for _, e := range f {
		k := strings.ToLower(e.Key)
		if _, dup := m[k]; !dup {
			m[k] = e.Value
		}
	}
	return m
}

// Field key limits. A key is a label, not a sentence: these caps are what keep
// ordinary prose containing a colon ("one rule matters here: keep it readable")
// from being read as a record field.
const (
	maxFieldKeyLen   = 32
	maxFieldKeyWords = 3
)

// isFieldLine reports whether a line opens a field, returning key and value.
// A key is a short run of letters, digits, spaces, underscores, hyphens and
// dots followed by ':' or '='.
func isFieldLine(line string) (key, val string, ok bool) {
	for i := 0; i < len(line) && i <= maxFieldKeyLen; i++ {
		c := line[i]
		if c == ':' || c == '=' {
			k := strings.TrimSpace(line[:i])
			if k == "" || !isNameStart(k[0]) || len(strings.Fields(k)) > maxFieldKeyWords {
				return "", "", false
			}
			return k, strings.TrimSpace(line[i+1:]), true
		}
		if !(c == ' ' || c == '_' || c == '-' || c == '.' || isNameByte(c)) {
			return "", "", false
		}
	}
	return "", "", false
}

// ParseFields interprets a block payload as an ordered record. Lines before the
// first field become the "" key, so nothing is lost. Lines after a field are
// appended to it, which is what lets a value span paragraphs:
//
//	Summary:
//	This chapter explains neural networks.
//
//	Tags: ml, cnn
func ParseFields(payload string) Fields {
	var out Fields
	var cur *Field
	var preamble []string

	for _, line := range strings.Split(payload, "\n") {
		if k, v, ok := isFieldLine(line); ok {
			out = append(out, Field{Key: k, Value: v})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			preamble = append(preamble, line)
			continue
		}
		if cur.Value == "" {
			cur.Value = strings.TrimSpace(line)
		} else {
			cur.Value += "\n" + line
		}
	}
	for i := range out {
		out[i].Value = strings.TrimSpace(out[i].Value)
	}
	if text := strings.TrimSpace(strings.Join(preamble, "\n")); text != "" {
		out = append(Fields{{Key: "", Value: text}}, out...)
	}
	return out
}

// Fields interprets a block node's payload as a record. It is meaningful for
// record-shaped directives (@task, @knowledge, @ai, @chat, @metadata and any
// plugin block that adopts the convention) and harmless elsewhere.
func (n Node) Fields() Fields {
	if n.Kind != KindBlock {
		return nil
	}
	return ParseFields(n.Text)
}

// Outline is one heading in the document's table of contents.
type Outline struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// Block is a structured directive as an agent sees it: a type, an ordered
// record, and the raw payload for anything the record does not capture.
type Block struct {
	Type   string            `json:"type"`
	Line   int               `json:"line"`
	Args   map[string]string `json:"args,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
	Order  []string          `json:"order,omitempty"` // field keys in source order
	Text   string            `json:"text,omitempty"`
}

// Link is a hyperlink found in prose.
type Link struct {
	Text string `json:"text"`
	Href string `json:"href"`
	Line int    `json:"line"`
}

// Media is an image, video, audio clip or attachment.
type Media struct {
	Kind    string `json:"kind"`
	Src     string `json:"src"`
	Caption string `json:"caption,omitempty"`
	Line    int    `json:"line"`
}

// Code is one code block.
type Code struct {
	Language string `json:"language,omitempty"`
	Lines    int    `json:"lines"`
	Line     int    `json:"line"`
	Source   string `json:"source"`
}

// Task is a unit of work, from either a @task block or a checklist item.
type Task struct {
	Title  string `json:"title"`
	Done   bool   `json:"done"`
	Status string `json:"status,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Due    string `json:"due,omitempty"`
	Line   int    `json:"line"`
}

// Extraction is the machine-facing view of a document: everything an agent
// needs without having to infer structure from prose.
type Extraction struct {
	Version  string            `json:"version,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Outline  []Outline         `json:"outline,omitempty"`
	Tasks    []Task            `json:"tasks,omitempty"`
	Blocks   []Block           `json:"blocks,omitempty"`
	Links    []Link            `json:"links,omitempty"`
	Media    []Media           `json:"media,omitempty"`
	Code     []Code            `json:"code,omitempty"`
	Text     string            `json:"text"`
	Words    int               `json:"words"`
}

// presentational names carry no meaning beyond how they look, so they are not
// reported as semantic blocks. Everything else is — including directives this
// build has never heard of, which is the point.
var presentational = map[string]bool{
	"code": true, "table": true, "math": true, "mermaid": true,
	"metadata": true, "comment": true, "raw": true, "image": true,
	"video": true, "audio": true, "attachment": true, "hr": true,
	"xtxt": true, "include": true, "embed": true, "footnote": true,
}

// Extract builds the machine-facing view of a document.
func Extract(doc *Document) *Extraction {
	e := &Extraction{Version: doc.Version, Metadata: doc.Metadata()}
	var prose []string

	for _, n := range doc.Nodes {
		switch n.Kind {
		case KindHeading:
			text := InlineText(n.Text)
			e.Outline = append(e.Outline, Outline{Level: n.Level, Text: text, Line: n.Line})
			prose = append(prose, text)
			e.Links = append(e.Links, linksIn(n.Text, n.Line)...)
		case KindParagraph, KindQuote:
			prose = append(prose, InlineText(n.Text))
			e.Links = append(e.Links, linksIn(n.Text, n.Line)...)
		case KindList:
			for _, it := range n.Items {
				prose = append(prose, InlineText(it.Text))
				e.Links = append(e.Links, linksIn(it.Text, n.Line)...)
				if it.Checked != nil {
					e.Tasks = append(e.Tasks, Task{
						Title: InlineText(it.Text), Done: *it.Checked, Line: n.Line,
					})
				}
			}
		case KindDirective, KindBlock:
			e.absorb(n, &prose)
		}
	}

	e.Text = strings.Join(prose, "\n\n")
	e.Words = len(strings.Fields(e.Text))
	return e
}

func (e *Extraction) absorb(n Node, prose *[]string) {
	switch n.Name {
	case "comment", "metadata":
		return
	case "image", "video", "audio", "attachment":
		e.Media = append(e.Media, Media{
			Kind: n.Name, Src: n.Args.Resolve("src"),
			Caption: InlineText(n.Args.Get("caption")), Line: n.Line,
		})
		if c := n.Args.Get("caption"); c != "" {
			*prose = append(*prose, InlineText(c))
		}
		return
	case "code":
		e.Code = append(e.Code, Code{
			Language: n.Args.Resolve("language"),
			Lines:    strings.Count(n.Text, "\n") + 1,
			Line:     n.Line, Source: n.Text,
		})
		return
	case "table":
		t := ParseTable(n)
		for _, row := range append([][]string{t.Header}, t.Rows...) {
			*prose = append(*prose, strings.Join(row, " | "))
		}
		return
	}

	if presentational[n.Name] {
		if n.Kind == KindBlock && n.Text != "" {
			*prose = append(*prose, n.Text)
		}
		return
	}

	f := n.Fields()
	b := Block{Type: n.Name, Line: n.Line, Text: n.Text}
	if len(n.Args) > 0 {
		b.Args = map[string]string{}
		for i, a := range n.Args {
			key := a.Key
			if key == "" {
				key = itoa(i)
			}
			b.Args[key] = a.Value
		}
	}
	if len(f) > 0 {
		b.Fields = f.Map()
		for _, x := range f {
			b.Order = append(b.Order, x.Key)
		}
	}
	e.Blocks = append(e.Blocks, b)

	if n.Name == "task" {
		m := f.Map()
		title := m["title"]
		if title == "" {
			title = m[""]
		}
		status := m["status"]
		e.Tasks = append(e.Tasks, Task{
			Title: title, Status: status, Owner: m["owner"], Due: m["due"],
			Done: strings.EqualFold(status, "done") || strings.EqualFold(status, "complete"),
			Line: n.Line,
		})
	}
	if n.Kind == KindBlock && n.Text != "" {
		*prose = append(*prose, n.Text)
	}
}

// linksIn finds inline links in a run of text.
func linksIn(s string, line int) []Link {
	var out []Link
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] != '[' {
			continue
		}
		if label, target, end, ok := link(s, i); ok {
			out = append(out, Link{Text: InlineText(label), Href: target, Line: line})
			i = end
		}
	}
	return out
}
