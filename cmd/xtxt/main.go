// Command xtxt validates, renders and converts XTXT documents.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SaiMouli3/xtxt"
)

const usage = `xtxt — tools for the XTXT plain-text document format

usage:
  xtxt validate <file>...          check syntax and report problems
  xtxt lint <file>...              validate, plus style warnings
  xtxt render <file>               render to the terminal
  xtxt export <file> <format>      convert to another format
  xtxt import <file.md>            convert Markdown to XTXT
  xtxt ast <file>                  print the parse tree as JSON
  xtxt extract <file>              print the machine-facing view as JSON

formats:
  html   standalone HTML document
  body   HTML fragment, no <head> or styling
  md     CommonMark
  text   plain text
  json   parse tree

options:
  --resolve    expand @include and @embed before rendering
  --plugins <path>  load a plugin manifest (default: nearest xtxt.plugins.json)
  -o <path>    write to a file instead of stdout
  -w <n>       wrap column for text output (default 80)
  --mermaid    in html output, load mermaid.js from a CDN to draw diagrams
  --no-color   disable ANSI colour in terminal output

Use - as <file> to read from stdin.
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var out, pluginPath string
	var width = 80
	var mermaid, noColor, resolve bool
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i < len(args) {
				out = args[i]
			}
		case "-w":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &width)
			}
		case "--mermaid":
			mermaid = true
		case "--resolve":
			resolve = true
		case "--plugins":
			i++
			if i < len(args) {
				pluginPath = args[i]
			}
		case "--no-color":
			noColor = true
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	opts := runOptions{width: width, mermaid: mermaid, resolve: resolve, pluginPath: pluginPath}
	cmd, files := rest[0], rest[1:]
	switch cmd {
	case "validate", "lint":
		os.Exit(check(files, cmd == "lint", opts))
	case "render":
		opts.color = !noColor && out == ""
		emit(out, render(one(files), "text", opts))
	case "export":
		if len(files) < 2 {
			die("export needs a file and a format")
		}
		emit(out, render(files[0], files[1], opts))
	case "import":
		src, err := read(one(files))
		if err != nil {
			die(err.Error())
		}
		emit(out, xtxt.FromMarkdown(src))
	case "ast":
		emit(out, render(one(files), "json", opts))
	case "extract":
		emit(out, render(one(files), "extract", opts))
	default:
		die("unknown command " + cmd)
	}
}

func one(files []string) string {
	if len(files) != 1 {
		die("expected exactly one file")
	}
	return files[0]
}

func read(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

type runOptions struct {
	width      int
	mermaid    bool
	resolve    bool
	color      bool
	pluginPath string
}

// load parses a document and, when asked, expands its @include/@embed
// references relative to its own directory.
func load(path string, opts runOptions) *xtxt.Result {
	src, err := read(path)
	if err != nil {
		die(err.Error())
	}
	res := xtxt.ParseString(src)
	if opts.resolve {
		doc, issues := xtxt.Resolve(res.Doc, docDir(path))
		res.Doc = doc
		res.Issues = append(res.Issues, issues...)
	}
	return res
}

func docDir(path string) string {
	if path == "-" {
		return mustCwd()
	}
	return filepath.Dir(path)
}

// loadPlugins finds a manifest explicitly named or sitting beside the document.
// A missing default manifest is not an error; a named one that fails to load is.
func loadPlugins(path string, opts runOptions) xtxt.Plugins {
	manifest := opts.pluginPath
	explicit := manifest != ""
	if !explicit {
		manifest = xtxt.FindPluginManifest(docDir(path))
	}
	if manifest == "" {
		return nil
	}
	p, err := xtxt.LoadPlugins(manifest)
	if err != nil {
		if explicit {
			die(manifest + ": " + err.Error())
		}
		fmt.Fprintf(os.Stderr, "xtxt: %s: %s (ignored)\n", manifest, err)
		return nil
	}
	return p
}

func render(path, format string, opts runOptions) string {
	res := load(path, opts)
	reportTo(os.Stderr, path, res.Issues)
	switch format {
	case "html":
		return xtxt.RenderHTML(res.Doc, xtxt.HTMLOptions{
			Full: true, Mermaid: opts.mermaid, Plugins: loadPlugins(path, opts)})
	case "body":
		return xtxt.RenderHTML(res.Doc, xtxt.HTMLOptions{Plugins: loadPlugins(path, opts)})
	case "md", "markdown":
		return xtxt.RenderMarkdown(res.Doc)
	case "text", "txt":
		return xtxt.RenderText(res.Doc, xtxt.TextOptions{Width: opts.width, Color: opts.color})
	case "json", "ast":
		b, _ := json.MarshalIndent(res.Doc, "", "  ")
		return string(b) + "\n"
	case "extract":
		b, _ := json.MarshalIndent(xtxt.Extract(res.Doc), "", "  ")
		return string(b) + "\n"
	default:
		die("unknown format " + format + " (want html, body, md, text, json or extract)")
		return ""
	}
}

func check(files []string, style bool, opts runOptions) int {
	if len(files) == 0 {
		die("nothing to check")
	}
	failed := 0
	for _, f := range files {
		res := load(f, opts)
		declared := loadPlugins(f, opts).Names()
		issues := append(append([]xtxt.Issue{}, res.Issues...), xtxt.Validate(res.Doc, declared...)...)
		if style {
			issues = append(issues, xtxt.Lint(res.Doc)...)
		}
		issues = xtxt.SortIssues(issues)
		reportTo(os.Stdout, f, issues)
		if hasErrors(issues) {
			failed++
			continue
		}
		if len(issues) == 0 {
			fmt.Printf("%s: ok (%d blocks)\n", display(f), len(res.Doc.Nodes))
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func hasErrors(issues []xtxt.Issue) bool {
	for _, i := range issues {
		if i.Severity == xtxt.Error {
			return true
		}
	}
	return false
}

func reportTo(w io.Writer, path string, issues []xtxt.Issue) {
	for _, i := range issues {
		fmt.Fprintf(w, "%s:%d: %s: %s\n", display(path), i.Line, i.Severity, i.Message)
	}
}

func display(path string) string {
	if path == "-" {
		return "<stdin>"
	}
	if rel, err := filepath.Rel(mustCwd(), path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func mustCwd() string {
	d, _ := os.Getwd()
	return d
}

func emit(out, s string) {
	if out == "" {
		fmt.Print(s)
		return
	}
	if err := os.WriteFile(out, []byte(s), 0o644); err != nil {
		die(err.Error())
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "xtxt: "+msg)
	os.Exit(2)
}
