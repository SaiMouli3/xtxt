package xtxt

import (
	"os"
	"path/filepath"
	"strings"
)

// maxIncludeDepth bounds nesting even when no cycle exists.
const maxIncludeDepth = 16

// Resolve expands @include and @embed directives by splicing the referenced
// documents in place. base is the directory that relative sources resolve
// against, and is also a boundary: a source that escapes it, or an absolute
// path, is refused. Rendering a document must not become a way to read
// arbitrary files off the machine.
//
// @include splices the target's nodes unchanged. @embed does the same but
// demotes the target's headings so it nests beneath the including section.
func Resolve(doc *Document, base string) (*Document, []Issue) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return doc, []Issue{{Error, 0, "cannot resolve base directory: " + err.Error()}}
	}
	// Resolve the base through symlinks too, so that comparing it against a
	// resolved target is apples to apples. Without this, any base reached via a
	// symlink (macOS /var, most CI checkouts) rejects its own files.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	r := &resolver{base: abs, active: map[string]bool{}}
	out := &Document{Version: doc.Version}
	out.Nodes = r.expand(doc.Nodes, 0, 0)
	return out, r.issues
}

type resolver struct {
	base   string
	active map[string]bool
	issues []Issue
}

func (r *resolver) fail(line int, msg string) {
	r.issues = append(r.issues, Issue{Error, line, msg})
}

func (r *resolver) expand(nodes []Node, depth, demote int) []Node {
	var out []Node
	for _, n := range nodes {
		if n.Kind == KindDirective && (n.Name == "include" || n.Name == "embed") {
			out = append(out, r.splice(n, depth, demote)...)
			continue
		}
		if demote > 0 && n.Kind == KindHeading {
			n.Level = min(n.Level+demote, 6)
		}
		out = append(out, n)
	}
	return out
}

func (r *resolver) splice(n Node, depth, demote int) []Node {
	src := n.Args.Resolve("src")
	if src == "" {
		r.fail(n.Line, "@"+n.Name+" has no src")
		return nil
	}
	if depth >= maxIncludeDepth {
		r.fail(n.Line, "@"+n.Name+" nested more than "+itoa(maxIncludeDepth)+" deep")
		return nil
	}

	path, err := r.safePath(src)
	if err != nil {
		r.fail(n.Line, "@"+n.Name+" "+src+": "+err.Error())
		return nil
	}
	if r.active[path] {
		r.fail(n.Line, "@"+n.Name+" "+src+": include cycle")
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		r.fail(n.Line, "@"+n.Name+" "+src+": "+err.Error())
		return nil
	}

	sub := ParseString(string(data))
	for _, i := range sub.Issues {
		i.Message = src + ": " + i.Message
		r.issues = append(r.issues, i)
	}

	next := demote
	if n.Name == "embed" {
		next = demote + headingDepthAt(n)
	}

	r.active[path] = true
	nodes := r.expand(sub.Doc.Nodes, depth+1, next)
	delete(r.active, path)
	return nodes
}

// headingDepthAt is how far to demote an embedded document. One level is enough
// to make the embed a child of the surrounding section without flattening its
// own internal hierarchy.
func headingDepthAt(Node) int { return 1 }

// safePath joins src to the base directory and refuses anything that leaves it.
func (r *resolver) safePath(src string) (string, error) {
	if filepath.IsAbs(src) {
		return "", errRefused("absolute paths are not allowed")
	}
	if strings.Contains(src, "://") {
		return "", errRefused("remote sources are not allowed")
	}
	joined := filepath.Join(r.base, src)
	clean, err := filepath.EvalSymlinks(joined)
	if err != nil {
		clean = filepath.Clean(joined) // file may not exist yet; report that below
	}
	rel, err := filepath.Rel(r.base, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errRefused("path escapes the document directory")
	}
	return joined, nil
}

type errRefused string

func (e errRefused) Error() string { return string(e) }
