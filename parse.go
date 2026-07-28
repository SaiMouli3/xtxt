package xtxt

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Severity of a parse or validation issue.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Issue is a diagnostic tied to a line of source.
type Issue struct {
	Severity Severity `json:"severity"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
}

// Result is the outcome of parsing: a document plus any diagnostics. A document
// is always returned, even when Issues contains errors — recovery is part of
// the format's compatibility guarantee.
type Result struct {
	Doc    *Document `json:"doc"`
	Issues []Issue   `json:"issues,omitempty"`
}

// HasErrors reports whether any issue is fatal.
func (r *Result) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Severity == Error {
			return true
		}
	}
	return false
}

// ParseFile reads and parses the named file.
func ParseFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// ParseString parses s.
func ParseString(s string) *Result {
	r, _ := Parse(strings.NewReader(s))
	return r
}

// Parse reads an XTXT document.
func Parse(r io.Reader) (*Result, error) {
	lines, err := readLines(r)
	if err != nil {
		return nil, err
	}
	p := &parser{lines: lines, doc: &Document{}}
	p.run()
	return &Result{Doc: p.doc, Issues: p.issues}, nil
}

func readLines(r io.Reader) ([]string, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	}
	return lines, sc.Err()
}

type parser struct {
	lines  []string
	i      int
	doc    *Document
	issues []Issue
}

func (p *parser) errf(line int, msg string)  { p.add(Error, line, msg) }
func (p *parser) warnf(line int, msg string) { p.add(Warning, line, msg) }
func (p *parser) add(s Severity, line int, msg string) {
	p.issues = append(p.issues, Issue{Severity: s, Line: line, Message: msg})
}

func (p *parser) more() bool  { return p.i < len(p.lines) }
func (p *parser) cur() string { return p.lines[p.i] }

func (p *parser) run() {
	for p.more() {
		line := p.cur()
		switch {
		case isBlank(line):
			p.i++
		case isDirective(line):
			p.parseDirective()
		case headingLevel(line) > 0:
			lvl := headingLevel(line)
			p.emit(Node{Kind: KindHeading, Level: lvl,
				Text: strings.TrimSpace(strings.TrimLeft(line, " \t")[lvl:]), Line: p.i + 1})
			p.i++
		case strings.HasPrefix(strings.TrimSpace(line), ">"):
			p.parseQuote()
		case itemPrefix(line) != "":
			p.parseList()
		default:
			p.parseParagraph()
		}
	}
}

func (p *parser) emit(n Node) { p.doc.Nodes = append(p.doc.Nodes, n) }

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// headingLevel returns 1-6 for a heading line, else 0.
func headingLevel(s string) int {
	s = strings.TrimLeft(s, " \t")
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(s) || s[n] != ' ' {
		return 0
	}
	// Report the level, but the caller slices the trimmed string, so re-find it.
	return n
}

// isDirective reports whether the line opens a directive (and is not escaped).
func isDirective(s string) bool {
	t := strings.TrimLeft(s, " \t")
	if !strings.HasPrefix(t, "@") || len(t) < 2 {
		return false
	}
	return isNameStart(t[1])
}

func isNameStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isNameByte(b byte) bool {
	return isNameStart(b) || b == '-' || (b >= '0' && b <= '9')
}

// directiveName returns the name and the rest of the line after it.
func directiveName(s string) (name, rest string) {
	t := strings.TrimLeft(s, " \t")
	i := 1
	for i < len(t) && isNameByte(t[i]) {
		i++
	}
	return t[1:i], t[i:]
}

func (p *parser) parseDirective() {
	start := p.i
	name, rest := directiveName(p.cur())

	args, ok := p.parseArgs(rest)
	if !ok {
		p.errf(start+1, "unclosed argument list for @"+name)
		p.i = start + 1
		return
	}

	if strings.HasPrefix(name, "end") {
		p.errf(start+1, "@"+name+" has no matching opening fence")
		p.i++
		return
	}

	if name == "xtxt" && len(p.doc.Nodes) == 0 {
		p.doc.Version = args.Resolve("version")
		p.i++
		return
	}

	// A directive is fenced if a matching @end<name> line follows. This keeps
	// the rule local to the document: no registry of known block names is
	// needed, so a new fenced directive parses correctly in an old reader.
	if end := p.findFence(name, p.i+1); end >= 0 {
		body := p.lines[p.i+1 : end]
		p.i = end + 1
		p.emit(Node{Kind: KindBlock, Name: name, Args: args, Text: trimFenceBody(body), Line: start + 1})
		return
	}

	if p.looksUnclosed(name) {
		p.errf(start+1, "unclosed @"+name+" block: no matching @end"+name)
	}
	p.i++
	p.emit(Node{Kind: KindDirective, Name: name, Args: args, Line: start + 1})
}

// findFence returns the index of the @end<name> line, or -1.
func (p *parser) findFence(name string, from int) int {
	closer := "@end" + name
	for j := from; j < len(p.lines); j++ {
		if strings.TrimRight(p.lines[j], " \t") == closer {
			return j
		}
	}
	return -1
}

// looksUnclosed reports whether a directive that found no closing fence was
// probably meant to be a block: the standard fenced names always are.
func (p *parser) looksUnclosed(name string) bool {
	switch name {
	case "code", "table", "math", "mermaid", "metadata", "comment", "raw":
		return true
	}
	return false
}

// trimFenceBody drops one leading and one trailing blank line and unescapes
// any \@end… lines, then joins with newlines.
func trimFenceBody(body []string) string {
	if len(body) > 0 && isBlank(body[0]) {
		body = body[1:]
	}
	if len(body) > 0 && isBlank(body[len(body)-1]) {
		body = body[:len(body)-1]
	}
	out := make([]string, len(body))
	for i, l := range body {
		if strings.HasPrefix(strings.TrimLeft(l, " \t"), `\@end`) {
			l = strings.Replace(l, `\@end`, "@end", 1)
		}
		out[i] = l
	}
	return strings.Join(out, "\n")
}

// parseArgs reads an argument list starting at rest (the text after the
// directive name), consuming further lines if it spans them. It advances p.i
// past the last consumed line only when it succeeds.
func (p *parser) parseArgs(rest string) (Args, bool) {
	trimmed := strings.TrimSpace(rest)
	if !strings.HasPrefix(trimmed, "(") {
		return nil, true
	}
	buf := trimmed
	line := p.i
	for {
		if inner, ok := balanced(buf); ok {
			p.i = line
			return splitArgs(inner), true
		}
		line++
		if line >= len(p.lines) {
			return nil, false
		}
		buf += "\n" + p.lines[line]
	}
}

// balanced returns the text inside the outermost parens if s closes them.
func balanced(s string) (string, bool) {
	depth, inQuote, esc := 0, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case inQuote:
			if c == '"' {
				inQuote = false
			}
		case c == '"':
			inQuote = true
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return s[1:i], true
			}
		}
	}
	return "", false
}

// splitArgs parses the inside of an argument list.
func splitArgs(s string) Args {
	var args Args
	for _, field := range splitTop(s, ',') {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, val := "", field
		if eq := topIndex(field, '='); eq > 0 && isName(field[:eq]) {
			key = strings.TrimSpace(field[:eq])
			val = strings.TrimSpace(field[eq+1:])
		}
		args = append(args, Arg{Key: key, Value: unquote(val)})
	}
	return args
}

// splitTop splits on sep, ignoring separators inside quotes or nested parens.
func splitTop(s string, sep byte) []string {
	var out []string
	depth, inQuote, esc, start := 0, false, false, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case inQuote:
			if c == '"' {
				inQuote = false
			}
		case c == '"':
			inQuote = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func topIndex(s string, b byte) int {
	parts := splitTop(s, b)
	if len(parts) < 2 {
		return -1
	}
	return len(parts[0])
}

func isName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || !isNameStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isNameByte(s[i]) {
			return false
		}
	}
	return true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return unescape(s[1 : len(s)-1])
	}
	return s
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (p *parser) parseQuote() {
	start := p.i
	var parts []string
	for p.more() {
		t := strings.TrimSpace(p.cur())
		if !strings.HasPrefix(t, ">") {
			break
		}
		parts = append(parts, strings.TrimSpace(t[1:]))
		p.i++
	}
	p.emit(Node{Kind: KindQuote, Text: strings.Join(parts, " "), Line: start + 1})
}

// itemPrefix returns the bullet or number prefix of a list item, or "".
func itemPrefix(s string) string {
	t := strings.TrimLeft(s, " \t")
	if len(t) >= 2 && (t[0] == '-' || t[0] == '*') && t[1] == ' ' {
		return t[:2]
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(t) && t[i] == '.' && t[i+1] == ' ' {
		return t[:i+2]
	}
	return ""
}

func (p *parser) parseList() {
	start := p.i
	var items []Item
	for p.more() {
		pre := itemPrefix(p.cur())
		if pre == "" {
			break
		}
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(p.cur(), " \t"), pre))
		item := Item{Ordered: pre[0] >= '0' && pre[0] <= '9'}
		if len(body) >= 3 && body[0] == '[' && body[2] == ']' {
			switch body[1] {
			case ' ', 'x', 'X':
				checked := body[1] != ' '
				item.Checked = &checked
				body = strings.TrimSpace(body[3:])
			}
		}
		item.Text = body
		items = append(items, item)
		p.i++
	}
	p.emit(Node{Kind: KindList, Items: items, Line: start + 1})
}

func (p *parser) parseParagraph() {
	start := p.i
	var parts []string
	for p.more() {
		line := p.cur()
		if isBlank(line) || headingLevel(line) > 0 || isDirective(line) ||
			itemPrefix(line) != "" || strings.HasPrefix(strings.TrimSpace(line), ">") {
			break
		}
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, `\@`) {
			t = t[1:]
		}
		parts = append(parts, t)
		p.i++
	}
	p.emit(Node{Kind: KindParagraph, Text: strings.Join(parts, " "), Line: start + 1})
}

func parseMetadata(payload string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(payload, "\n") {
		if isBlank(line) {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return m
}

// ParseTable interprets the payload of an @table block.
func ParseTable(n Node) Table {
	var t Table
	var rows [][]string
	sepAt := -1
	for _, line := range strings.Split(n.Text, "\n") {
		if isBlank(line) {
			continue
		}
		cells := splitCells(line)
		if sepAt < 0 && isSeparatorRow(cells) {
			sepAt = len(rows)
			t.Align = alignments(cells)
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return t
	}
	if sepAt <= 0 {
		sepAt = 1
	}
	t.Header = rows[0]
	if sepAt > 1 && sepAt <= len(rows) {
		t.Header = rows[sepAt-1]
	}
	t.Rows = rows[sepAt:]
	return t
}

func splitCells(line string) []string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if c == "" || strings.Trim(c, "-:") != "" {
			return false
		}
	}
	return len(cells) > 0
}

func alignments(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right:
			out[i] = "center"
		case right:
			out[i] = "right"
		default:
			out[i] = "left"
		}
	}
	return out
}
