package xtxt

import (
	"encoding/json"
	"html/template"
	"os"
	"strings"
)

// Plugin renders one directive that the core does not know about. A plugin is
// declaration, not code: a name and an HTML template. That is enough for the
// embed-a-service case (@youtube, @spotify, @github) which is most of what
// plugins are actually used for, and it means loading a document can never
// execute anything.
type Plugin struct {
	Name string `json:"name"`
	HTML string `json:"html"`
	Text string `json:"text,omitempty"` // terminal fallback; defaults to the src or first arg
}

// Plugins maps a directive name to its renderer.
type Plugins map[string]*compiledPlugin

type compiledPlugin struct {
	Plugin
	tmpl *template.Template
}

// PluginData is what a plugin template sees.
type PluginData struct {
	Name   string
	Args   map[string]string
	Arg    []string          // positional arguments
	Fields map[string]string // payload parsed as Key: value
	Text   string            // raw payload
	Line   int
}

// LoadPlugins reads a plugin manifest: a JSON array of {name, html, text}.
// Values interpolated into a template are escaped by html/template, so a
// document cannot inject markup through a plugin's arguments.
func LoadPlugins(path string) (Plugins, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Plugin
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return CompilePlugins(list)
}

// CompilePlugins prepares plugins for use.
func CompilePlugins(list []Plugin) (Plugins, error) {
	out := Plugins{}
	for _, p := range list {
		t, err := template.New(p.Name).Parse(p.HTML)
		if err != nil {
			return nil, err
		}
		out[p.Name] = &compiledPlugin{Plugin: p, tmpl: t}
	}
	return out, nil
}

// FindPluginManifest looks for xtxt.plugins.json beside a document and then in
// each parent directory, the way tools find their config.
func FindPluginManifest(start string) string {
	dir := start
	for i := 0; i < 24; i++ {
		candidate := dir + "/xtxt.plugins.json"
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := parentDir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func parentDir(p string) string {
	if i := strings.LastIndex(strings.TrimSuffix(p, "/"), "/"); i > 0 {
		return p[:i]
	}
	return p
}

// Names lists the directives this plugin set declares.
func (p Plugins) Names() []string {
	out := make([]string, 0, len(p))
	for name := range p {
		out = append(out, name)
	}
	return out
}

func (p Plugins) render(n Node) (string, bool) {
	cp, ok := p[n.Name]
	if !ok {
		return "", false
	}
	data := PluginData{Name: n.Name, Text: n.Text, Line: n.Line, Args: map[string]string{}}
	for _, a := range n.Args {
		if a.Key == "" {
			data.Arg = append(data.Arg, a.Value)
			continue
		}
		data.Args[a.Key] = a.Value
	}
	if len(data.Arg) > 0 {
		if _, has := data.Args["src"]; !has {
			data.Args["src"] = data.Arg[0]
		}
	}
	if f := n.Fields(); len(f) > 0 {
		data.Fields = f.Map()
	}
	var b strings.Builder
	if err := cp.tmpl.Execute(&b, data); err != nil {
		return "", false
	}
	return b.String(), true
}
