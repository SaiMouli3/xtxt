package xtxt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func parse(t *testing.T, src string) *Document {
	t.Helper()
	res := ParseString(src)
	if res.HasErrors() {
		t.Fatalf("unexpected errors: %+v", res.Issues)
	}
	return res.Doc
}

func TestTextBlocks(t *testing.T) {
	doc := parse(t, `# Title

First line
second line.

> quoted
> more

- one
- [x] done
- [ ] todo

1. first
2. second
`)
	want := []Kind{KindHeading, KindParagraph, KindQuote, KindList, KindList}
	if len(doc.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d: %+v", len(doc.Nodes), len(want), doc.Nodes)
	}
	for i, k := range want {
		if doc.Nodes[i].Kind != k {
			t.Errorf("node %d: got %s want %s", i, doc.Nodes[i].Kind, k)
		}
	}
	if got := doc.Nodes[1].Text; got != "First line second line." {
		t.Errorf("paragraph soft-wrap: %q", got)
	}
	if got := doc.Nodes[2].Text; got != "quoted more" {
		t.Errorf("quote: %q", got)
	}
	list := doc.Nodes[3].Items
	if len(list) != 3 || list[0].Checked != nil || list[1].Checked == nil || !*list[1].Checked || *list[2].Checked {
		t.Errorf("checklist parsing: %+v", list)
	}
	if !doc.Nodes[4].Items[0].Ordered {
		t.Error("ordered list not detected")
	}
}

func TestHeadingLevels(t *testing.T) {
	doc := parse(t, "###### deep\n\n####### too deep\n")
	if doc.Nodes[0].Level != 6 || doc.Nodes[0].Text != "deep" {
		t.Errorf("h6: %+v", doc.Nodes[0])
	}
	if doc.Nodes[1].Kind != KindParagraph {
		t.Errorf("7 hashes should not be a heading: %+v", doc.Nodes[1])
	}
}

func TestInlineDirectiveMultiline(t *testing.T) {
	doc := parse(t, `@image(
    src="cnn.png",
    caption="CNN, Architecture",
    width=600
)

after
`)
	n := doc.Nodes[0]
	if n.Kind != KindDirective || n.Name != "image" {
		t.Fatalf("got %+v", n)
	}
	if n.Args.Resolve("src") != "cnn.png" {
		t.Errorf("src: %q", n.Args.Resolve("src"))
	}
	if n.Args.Get("caption") != "CNN, Architecture" {
		t.Errorf("comma inside quotes split the argument: %q", n.Args.Get("caption"))
	}
	if n.Args.Get("width") != "600" {
		t.Errorf("width: %q", n.Args.Get("width"))
	}
	if doc.Nodes[1].Text != "after" {
		t.Errorf("parser lost its place after a multi-line directive: %+v", doc.Nodes[1])
	}
}

func TestPositionalArgument(t *testing.T) {
	doc := parse(t, `@video("demo.mp4")`)
	if got := doc.Nodes[0].Args.Resolve("src"); got != "demo.mp4" {
		t.Errorf("positional resolve: %q", got)
	}
}

func TestFencedBlocks(t *testing.T) {
	doc := parse(t, "@code(language=\"go\")\n\nfunc main(){}\n\n@endcode\n")
	n := doc.Nodes[0]
	if n.Kind != KindBlock || n.Name != "code" {
		t.Fatalf("got %+v", n)
	}
	if n.Text != "func main(){}" {
		t.Errorf("payload should drop one blank line at each end: %q", n.Text)
	}
	if n.Args.Resolve("language") != "go" {
		t.Errorf("language: %q", n.Args.Resolve("language"))
	}
}

func TestFencePreservesIndentation(t *testing.T) {
	doc := parse(t, "@code\nif x:\n    y = 1\n@endcode\n")
	if doc.Nodes[0].Text != "if x:\n    y = 1" {
		t.Errorf("indentation lost: %q", doc.Nodes[0].Text)
	}
}

func TestUnclosedFenceIsAnError(t *testing.T) {
	res := ParseString("@code\nprint()\n")
	if !res.HasErrors() {
		t.Fatal("unclosed @code should be an error")
	}
}

func TestStrayEndIsAnError(t *testing.T) {
	res := ParseString("text\n\n@endcode\n")
	if !res.HasErrors() {
		t.Fatal("@endcode with no opener should be an error")
	}
}

func TestUnknownDirectivePreserved(t *testing.T) {
	res := ParseString("@futurething(a=1)\n\n@newblock\nbody\n@endnewblock\n")
	if res.HasErrors() {
		t.Fatalf("unknown directives must not be errors: %+v", res.Issues)
	}
	if len(res.Doc.Nodes) != 2 {
		t.Fatalf("both unknown nodes should survive: %+v", res.Doc.Nodes)
	}
	if res.Doc.Nodes[1].Text != "body" {
		t.Errorf("unknown fenced payload lost: %q", res.Doc.Nodes[1].Text)
	}
	warnings := Validate(res.Doc)
	if len(warnings) != 2 {
		t.Errorf("expected a warning per unknown directive, got %+v", warnings)
	}
	html := RenderHTML(res.Doc, HTMLOptions{})
	if !strings.Contains(html, "futurething") || !strings.Contains(html, "body") {
		t.Errorf("unknown directives must render as placeholders, got %q", html)
	}
}

func TestEscapedAtSign(t *testing.T) {
	doc := parse(t, "\\@image is written like this\n")
	if doc.Nodes[0].Kind != KindParagraph || doc.Nodes[0].Text != "@image is written like this" {
		t.Errorf("escape failed: %+v", doc.Nodes[0])
	}
}

func TestMetadataAndVersion(t *testing.T) {
	doc := parse(t, "@xtxt(version=1.0)\n\n@metadata\nAuthor = Subbu\nversion = 1.0\n@endmetadata\n")
	if doc.Version != "1.0" {
		t.Errorf("version: %q", doc.Version)
	}
	m := doc.Metadata()
	if m["author"] != "Subbu" || m["version"] != "1.0" {
		t.Errorf("metadata: %+v", m)
	}
}

func TestTable(t *testing.T) {
	doc := parse(t, "@table\nName | Age\n-----|----:\nJohn | 20\n\nAlice | 22\n@endtable\n")
	tbl := ParseTable(doc.Nodes[0])
	if len(tbl.Header) != 2 || tbl.Header[0] != "Name" {
		t.Fatalf("header: %+v", tbl.Header)
	}
	if len(tbl.Rows) != 2 || tbl.Rows[1][1] != "22" {
		t.Fatalf("rows (blank lines between rows must be ignored): %+v", tbl.Rows)
	}
	if tbl.Align[1] != "right" {
		t.Errorf("alignment: %+v", tbl.Align)
	}
}

func TestTableWithoutSeparator(t *testing.T) {
	tbl := ParseTable(parse(t, "@table\nA | B\n1 | 2\n@endtable\n").Nodes[0])
	if len(tbl.Header) != 2 || len(tbl.Rows) != 1 {
		t.Errorf("first row should be the header: %+v %+v", tbl.Header, tbl.Rows)
	}
}

func TestInlineHTML(t *testing.T) {
	cases := map[string]string{
		"plain":                 "plain",
		"**bold**":              "<strong>bold</strong>",
		"*em*":                  "<em>em</em>",
		"`a < b`":               "<code>a &lt; b</code>",
		"`**not bold**`":        "<code>**not bold**</code>",
		"[go](https://go.dev)":  `<a href="https://go.dev">go</a>`,
		"a < b & c":             "a &lt; b &amp; c",
		`\*literal\*`:           "*literal*",
		"unclosed *star":        "unclosed *star",
		"<script>":              "&lt;script&gt;",
		"**bold with *em* in**": "<strong>bold with <em>em</em> in</strong>",
	}
	for in, want := range cases {
		if got := InlineHTML(in); got != want {
			t.Errorf("InlineHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInlineText(t *testing.T) {
	if got := InlineText("**bold** and `code` and [x](y)"); got != "bold and code and x" {
		t.Errorf("got %q", got)
	}
}

func TestHTMLEscapesUntrustedContent(t *testing.T) {
	doc := parse(t, "@image(src=\"x\" onerror=\"alert(1)\")\n")
	html := RenderHTML(doc, HTMLOptions{})
	if strings.Contains(html, `" onerror=`) {
		t.Errorf("attribute injection escaped out of the src attribute: %s", html)
	}
}

func TestRawHTMLPassesThrough(t *testing.T) {
	doc := parse(t, "@raw(format=\"html\")\n<b>hi</b>\n@endraw\n")
	if !strings.Contains(RenderHTML(doc, HTMLOptions{}), "<b>hi</b>") {
		t.Error("raw html should not be escaped")
	}
}

func TestCommentsAreNotRendered(t *testing.T) {
	doc := parse(t, "@comment\nsecret\n@endcomment\n\nvisible\n")
	for name, out := range map[string]string{
		"html": RenderHTML(doc, HTMLOptions{}),
		"text": RenderText(doc, TextOptions{}),
	} {
		if strings.Contains(out, "secret") {
			t.Errorf("%s output leaked a comment: %q", name, out)
		}
		if !strings.Contains(out, "visible") {
			t.Errorf("%s output dropped real content", name)
		}
	}
}

func TestValidateWarnings(t *testing.T) {
	res := ParseString("@image(caption=\"no source\")\n\n@metadata\na=1\n@endmetadata\n\n@metadata\nb=2\n@endmetadata\n")
	msgs := ""
	for _, i := range Validate(res.Doc) {
		if i.Severity != Warning {
			t.Errorf("expected warnings only, got %+v", i)
		}
		msgs += i.Message + "\n"
	}
	for _, want := range []string{"no src", "duplicate @metadata"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing warning %q in:\n%s", want, msgs)
		}
	}
}

func TestMarkdownRoundTrip(t *testing.T) {
	src := "# Title\n\nA paragraph.\n\n@code(language=\"go\")\nfunc main(){}\n@endcode\n\n@table\nA | B\n1 | 2\n@endtable\n"
	md := RenderMarkdown(parse(t, src))
	back := parse(t, FromMarkdown(md))

	if back.Nodes[0].Kind != KindHeading || back.Nodes[0].Text != "Title" {
		t.Errorf("heading did not survive: %+v", back.Nodes[0])
	}
	var code, table *Node
	for i := range back.Nodes {
		switch back.Nodes[i].Name {
		case "code":
			code = &back.Nodes[i]
		case "table":
			table = &back.Nodes[i]
		}
	}
	if code == nil || code.Text != "func main(){}" || code.Args.Resolve("language") != "go" {
		t.Errorf("code block did not survive the round trip: %+v", code)
	}
	if table == nil {
		t.Fatal("table did not survive the round trip")
	}
	if tbl := ParseTable(*table); len(tbl.Header) != 2 || len(tbl.Rows) != 1 {
		t.Errorf("table shape changed: %+v", tbl)
	}
}

func TestRenderTextTableAligns(t *testing.T) {
	out := RenderText(parse(t, "@table\nName | Age\nJohnathan | 20\n@endtable\n"), TextOptions{})
	if !strings.Contains(out, "Johnathan") || !strings.Contains(out, "Name") {
		t.Errorf("table text output: %q", out)
	}
}

func TestFullHTMLDocument(t *testing.T) {
	out := RenderHTML(parse(t, "# Hello\n\nworld\n"), HTMLOptions{Full: true})
	for _, want := range []string{"<!doctype html>", "<title>Hello</title>", "<h1>Hello</h1>", "<p>world</p>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestLineNumbers(t *testing.T) {
	doc := parse(t, "# a\n\nb\n\n@code\nx\n@endcode\n")
	for i, want := range []int{1, 3, 5} {
		if doc.Nodes[i].Line != want {
			t.Errorf("node %d line = %d, want %d", i, doc.Nodes[i].Line, want)
		}
	}
}

func TestInlineHTMLPreservesUTF8(t *testing.T) {
	// The scan is byte-oriented; converting a byte to a rune would corrupt
	// every multi-byte character in the document.
	cases := []string{
		"an em dash — here",
		"日本語のテキスト",
		"café naïve",
		"**émphase** and `código`",
		"emoji 🎉 survives",
	}
	for _, in := range cases {
		got := InlineHTML(in)
		if !utf8.ValidString(got) {
			t.Errorf("InlineHTML(%q) produced invalid UTF-8: %q", in, got)
		}
		for _, r := range in {
			if r > 127 && !strings.ContainsRune(got, r) {
				t.Errorf("InlineHTML(%q) lost %q: got %q", in, r, got)
			}
		}
	}
	if got := InlineHTML("a — b & c"); got != "a — b &amp; c" {
		t.Errorf("got %q", got)
	}
}
