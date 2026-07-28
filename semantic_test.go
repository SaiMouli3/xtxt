package xtxt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFields(t *testing.T) {
	f := ParseFields("Title: Build parser\nStatus: In Progress\nOwner: Subbu")
	if got := f.Get("title"); got != "Build parser" {
		t.Errorf("case-insensitive Get: %q", got)
	}
	if len(f) != 3 || f[1].Key != "Status" {
		t.Errorf("field order not preserved: %+v", f)
	}
}

func TestFieldsMultilineValue(t *testing.T) {
	f := ParseFields("Summary:\nThis chapter explains\nneural networks.\n\nTags: ml, cnn")
	if got := f.Get("Summary"); got != "This chapter explains\nneural networks." {
		t.Errorf("continuation lines not folded into the value: %q", got)
	}
	if got := f.Get("Tags"); got != "ml, cnn" {
		t.Errorf("field after a multi-line value: %q", got)
	}
}

func TestFieldsPreambleKept(t *testing.T) {
	f := ParseFields("Parser will use a single pass.\n\nRationale: simplicity")
	if got := f.Get(""); !strings.HasPrefix(got, "Parser will use") {
		t.Errorf("prose before the first field was dropped: %+v", f)
	}
}

func TestProseIsNotMistakenForFields(t *testing.T) {
	// A colon deep in a sentence must not turn the line into a record field.
	f := ParseFields("There is one rule that matters here: keep it readable.")
	if len(f) != 1 || f[0].Key != "" {
		t.Errorf("prose parsed as a field: %+v", f)
	}
}

func TestExtract(t *testing.T) {
	doc := parse(t, `# Title

Prose with a [link](https://example.com).

- [x] shipped
- [ ] pending

@task
Title: Build parser
Status: In Progress
Owner: Subbu
@endtask

@image(src="a.png", caption="A caption")

@code(language="go")
func main(){}
@endcode

@metadata
author = Subbu
@endmetadata
`)
	e := Extract(doc)

	if len(e.Outline) != 1 || e.Outline[0].Text != "Title" {
		t.Errorf("outline: %+v", e.Outline)
	}
	if e.Metadata["author"] != "Subbu" {
		t.Errorf("metadata: %+v", e.Metadata)
	}
	if len(e.Tasks) != 3 {
		t.Fatalf("want 2 checklist tasks + 1 @task, got %+v", e.Tasks)
	}
	if !e.Tasks[0].Done || e.Tasks[1].Done {
		t.Errorf("checklist done state: %+v", e.Tasks[:2])
	}
	if e.Tasks[2].Owner != "Subbu" || e.Tasks[2].Status != "In Progress" {
		t.Errorf("@task fields: %+v", e.Tasks[2])
	}
	if len(e.Links) != 1 || e.Links[0].Href != "https://example.com" {
		t.Errorf("links: %+v", e.Links)
	}
	if len(e.Media) != 1 || e.Media[0].Kind != "image" {
		t.Errorf("media: %+v", e.Media)
	}
	if len(e.Code) != 1 || e.Code[0].Language != "go" {
		t.Errorf("code: %+v", e.Code)
	}
	if len(e.Blocks) != 1 || e.Blocks[0].Type != "task" {
		t.Errorf("blocks: %+v", e.Blocks)
	}
	if strings.Contains(e.Text, "author = Subbu") {
		t.Error("metadata leaked into the prose text")
	}
	if e.Words == 0 {
		t.Error("word count not computed")
	}
}

func TestExtractKeepsUnknownBlocks(t *testing.T) {
	// The whole point of §7: a block this build has never heard of still
	// reaches an agent with its structure intact.
	doc := parse(t, "@experiment\nHypothesis: it works\nConfidence: 0.7\n@endexperiment\n")
	e := Extract(doc)
	if len(e.Blocks) != 1 || e.Blocks[0].Type != "experiment" {
		t.Fatalf("blocks: %+v", e.Blocks)
	}
	if e.Blocks[0].Fields["hypothesis"] != "it works" {
		t.Errorf("fields: %+v", e.Blocks[0].Fields)
	}
}

func TestFootnotes(t *testing.T) {
	doc := parse(t, "Claim[^1].\n\n@footnote(id=\"1\")\nThe evidence.\n@endfootnote\n")
	out := RenderHTML(doc, HTMLOptions{})
	for _, want := range []string{`href="#fn-1"`, `id="fn-1"`, "The evidence."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Index(out, "<section class=\"footnotes\"") < strings.Index(out, "Claim") {
		t.Error("footnotes should be rendered after the body")
	}
}

func TestFootnoteWarnings(t *testing.T) {
	res := ParseString("Text[^missing].\n\n@footnote(id=\"unused\")\nbody\n@endfootnote\n")
	var msgs string
	for _, i := range Validate(res.Doc) {
		msgs += i.Message + "\n"
	}
	for _, want := range []string{"[^missing] has no matching", `"unused") is never referenced`} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing warning %q in:\n%s", want, msgs)
		}
	}
}

func TestRecordRendering(t *testing.T) {
	doc := parse(t, "@decision\nTitle: Use a single pass\nWhy: simplicity\n@enddecision\n")
	out := RenderHTML(doc, HTMLOptions{})
	for _, want := range []string{`data-type="decision"`, "<dt>Title</dt>", "<dd>Use a single pass</dd>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestChartParsing(t *testing.T) {
	cases := map[string]string{
		"space": "@chart\nJan 20\nFeb 35\n@endchart\n",
		"pipe":  "@chart\nJan | 20\nFeb | 35\n@endchart\n",
		"colon": "@chart\nJan: 20\nFeb: 35\n@endchart\n",
	}
	for name, src := range cases {
		c := ParseChart(parse(t, src).Nodes[0])
		if len(c.Labels) != 2 || c.Labels[0] != "Jan" {
			t.Errorf("%s: labels %+v", name, c.Labels)
		}
		if len(c.Series) != 1 || c.Series[0].Values[1] != 35 {
			t.Errorf("%s: series %+v", name, c.Series)
		}
	}
}

func TestChartMultiLabelWithSpaces(t *testing.T) {
	c := ParseChart(parse(t, "@chart\nNew York 20\nSan Francisco 35\n@endchart\n").Nodes[0])
	if c.Labels[0] != "New York" || c.Labels[1] != "San Francisco" {
		t.Errorf("multi-word labels lost: %+v", c.Labels)
	}
}

func TestChartMultiSeriesAndHeader(t *testing.T) {
	c := ParseChart(parse(t, "@chart\nQ | North | South\n1 | 10 | 12\n2 | 14 | 9\n@endchart\n").Nodes[0])
	if len(c.Series) != 2 || c.Series[0].Name != "North" || c.Series[1].Values[1] != 9 {
		t.Fatalf("series: %+v", c.Series)
	}
	if len(c.Labels) != 2 || c.Labels[0] != "1" {
		t.Errorf("labels: %+v", c.Labels)
	}
}

func TestChartFoldsExcessSeries(t *testing.T) {
	c := ParseChart(parse(t,
		"@chart\nX | A | B | C | D\n1 | 1 | 2 | 3 | 4\n@endchart\n").Nodes[0])
	if len(c.Series) != maxSeries+1 || c.Series[maxSeries].Name != "Other" {
		t.Fatalf("series past the validated palette should fold into Other: %+v", c.Series)
	}
	if c.Series[maxSeries].Values[0] != 4 {
		t.Errorf("Other should sum the folded series: %+v", c.Series[maxSeries])
	}
	if len(c.Warnings) == 0 {
		t.Error("folding must be reported, not silent")
	}
}

func TestChartRendersWithTableView(t *testing.T) {
	doc := parse(t, "@chart(type=\"bar\", title=\"Monthly\")\nJan 20\nFeb 35\n@endchart\n")
	out := RenderHTML(doc, HTMLOptions{})
	for _, want := range []string{"<svg", "var(--chart-1)", "<title>Jan: 20</title>", "chart-data", "<td>Feb</td>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in chart output", want)
		}
	}
	if strings.Contains(out, "<script") {
		t.Error("chart output must stay script-free")
	}
}

func TestChartTextOutput(t *testing.T) {
	out := RenderText(parse(t, "@chart\nJan 20\nFeb 35\n@endchart\n"), TextOptions{Width: 60})
	if !strings.Contains(out, "█") || !strings.Contains(out, "35") {
		t.Errorf("terminal chart: %q", out)
	}
}

func TestPluginRendering(t *testing.T) {
	plugins, err := CompilePlugins([]Plugin{{
		Name: "youtube",
		HTML: `<iframe src="https://www.youtube.com/embed/{{.Args.id}}" title="{{.Args.title}}"></iframe>`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	doc := parse(t, `@youtube(id="abc123", title="A talk")`)
	out := RenderHTML(doc, HTMLOptions{Plugins: plugins})
	if !strings.Contains(out, "youtube.com/embed/abc123") {
		t.Errorf("plugin did not render: %s", out)
	}
	if strings.Contains(out, "class=\"unknown\"") {
		t.Error("plugin output should replace the unknown-directive placeholder")
	}
}

func TestPluginEscapesArguments(t *testing.T) {
	plugins, err := CompilePlugins([]Plugin{{Name: "card", HTML: `<div>{{.Args.text}}</div>`}})
	if err != nil {
		t.Fatal(err)
	}
	doc := parse(t, `@card(text="<script>alert(1)</script>")`)
	out := RenderHTML(doc, HTMLOptions{Plugins: plugins})
	if strings.Contains(out, "<script>") {
		t.Errorf("plugin arguments must be escaped: %s", out)
	}
}

func TestInclude(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("intro.xtxt", "# Introduction\n\nIntro text.\n")
	write("main.xtxt", "# Main\n\n@include(src=\"intro.xtxt\")\n\nAfter.\n")

	res, err := ParseFile(filepath.Join(dir, "main.xtxt"))
	if err != nil {
		t.Fatal(err)
	}
	doc, issues := Resolve(res.Doc, dir)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	var texts []string
	for _, n := range doc.Nodes {
		texts = append(texts, n.Text)
	}
	joined := strings.Join(texts, "|")
	if !strings.Contains(joined, "Introduction") || !strings.Contains(joined, "After.") {
		t.Errorf("include did not splice: %q", joined)
	}
}

func TestEmbedDemotesHeadings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.xtxt"), []byte("# Sub\n\ntext\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.xtxt"), []byte("# Main\n\n@embed(src=\"sub.xtxt\")\n"), 0o644)

	res, _ := ParseFile(filepath.Join(dir, "main.xtxt"))
	doc, issues := Resolve(res.Doc, dir)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	for _, n := range doc.Nodes {
		if n.Kind == KindHeading && n.Text == "Sub" && n.Level != 2 {
			t.Errorf("embedded heading should be demoted to level 2, got %d", n.Level)
		}
	}
}

func TestIncludeCycleIsAnError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.xtxt"), []byte("@include(src=\"b.xtxt\")\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.xtxt"), []byte("@include(src=\"a.xtxt\")\n"), 0o644)

	res, _ := ParseFile(filepath.Join(dir, "a.xtxt"))
	_, issues := Resolve(res.Doc, dir)
	found := false
	for _, i := range issues {
		if i.Severity == Error && strings.Contains(i.Message, "cycle") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a cycle error, got %+v", issues)
	}
}

func TestIncludeRefusesEscapingPaths(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.xtxt"), []byte(
		"@include(src=\"../secret.xtxt\")\n\n@include(src=\"/etc/passwd\")\n"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.xtxt"), []byte("secrets\n"), 0o644)

	res, _ := ParseFile(filepath.Join(dir, "a.xtxt"))
	doc, issues := Resolve(res.Doc, dir)
	if len(issues) != 2 {
		t.Fatalf("both traversals must be refused, got %+v", issues)
	}
	for _, n := range doc.Nodes {
		if strings.Contains(n.Text, "secrets") {
			t.Fatal("path traversal was not blocked")
		}
	}
}
