package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SaiMouli3/xtxt"
)

// A minimal but valid 1x1 PNG.
var testPNG = func() []byte {
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	return data
}()

func TestImageExtension(t *testing.T) {
	cases := map[string][]byte{
		".png":  testPNG,
		".jpg":  {0xFF, 0xD8, 0xFF, 0xE0},
		".gif":  []byte("GIF89a..."),
		".webp": append([]byte("RIFF____WEBP"), 0),
		".svg":  []byte("<svg xmlns=\"...\"></svg>"),
	}
	for want, data := range cases {
		if got := imageExtension(data); got != want {
			t.Errorf("imageExtension(%q) = %s, want %s", data[:min(len(data), 8)], got, want)
		}
	}
	// Anything unrecognised is treated as PNG, which is what clipboards
	// overwhelmingly produce.
	if got := imageExtension([]byte("not an image")); got != ".png" {
		t.Errorf("unknown data should default to .png, got %s", got)
	}
}

func TestBuildImageDirective(t *testing.T) {
	got := buildImageDirective("shot-1.png", pasteOptions{Caption: "A screenshot", Width: "400"})
	for _, want := range []string{`src="shot-1.png"`, `alt="A screenshot"`, `caption="A screenshot"`, "width=400"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// The directive it writes must parse back, or paste produces broken files.
	res := parseDirective(t, got)
	if res != "shot-1.png" {
		t.Errorf("directive did not round-trip: got src %q", res)
	}
}

func TestBuildImageDirectiveEscapesQuotes(t *testing.T) {
	got := buildImageDirective("a.png", pasteOptions{Caption: `He said "hi"`})
	if strings.Contains(got, `caption="He said "hi""`) {
		t.Fatalf("quotes in a caption were not escaped:\n%s", got)
	}
	if !strings.Contains(got, `\"hi\"`) {
		t.Errorf("expected escaped quotes in:\n%s", got)
	}
}

func TestUniqueImageName(t *testing.T) {
	dir := t.TempDir()
	first, err := uniqueImageName(dir, "note", ".png")
	if err != nil {
		t.Fatal(err)
	}
	if first != "note-1.png" {
		t.Errorf("first name = %q, want note-1.png", first)
	}
	if err := os.WriteFile(filepath.Join(dir, first), testPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := uniqueImageName(dir, "note", ".png")
	if err != nil {
		t.Fatal(err)
	}
	if second != "note-2.png" {
		t.Errorf("second name = %q, want note-2.png (must not overwrite)", second)
	}
}

func TestAppendToDocument(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "note.xtxt")
	if err := os.WriteFile(doc, []byte("# Title\n\nSome prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := appendToDocument(doc, "@image(src=\"a.png\")"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(doc)
	want := "# Title\n\nSome prose.\n\n@image(src=\"a.png\")\n"
	if string(got) != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}

	// Appending twice must not run the directives together.
	if err := appendToDocument(doc, "@image(src=\"b.png\")"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(doc)
	if !strings.Contains(string(got), "a.png\")\n\n@image") {
		t.Errorf("directives were not separated by a blank line:\n%s", got)
	}
}

func TestAppendToMissingDocumentCreatesIt(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "new.xtxt")
	if err := appendToDocument(doc, "@image(src=\"a.png\")"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "@image(src=\"a.png\")\n" {
		t.Errorf("got %q", got)
	}
}

func TestMimeForExtension(t *testing.T) {
	for ext, want := range map[string]string{
		".png": "image/png", ".jpg": "image/jpeg", ".gif": "image/gif",
		".webp": "image/webp", ".svg": "image/svg+xml", ".xyz": "image/png",
	} {
		if got := mimeForExtension(ext); got != want {
			t.Errorf("mimeForExtension(%q) = %q, want %q", ext, got, want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseDirective parses a directive with the real parser and returns its src,
// so the paste command cannot emit something the parser rejects.
func parseDirective(t *testing.T, source string) string {
	t.Helper()
	res := xtxt.ParseString(source)
	if res.HasErrors() {
		t.Fatalf("generated directive does not parse: %+v\n%s", res.Issues, source)
	}
	if len(res.Doc.Nodes) != 1 {
		t.Fatalf("expected one node, got %d", len(res.Doc.Nodes))
	}
	return res.Doc.Nodes[0].Args.Resolve("src")
}

// Numbering is per directory, so each folder starts at -1.
// Pasted media lands in a subfolder by default, and the directive points at it.
// The two have to agree: writing to one place and referencing another is worse
// than either choice on its own.
func TestPasteWritesIntoAFolderAndPointsAtIt(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "notes.xtxt")
	if err := os.WriteFile(doc, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		opt      pasteOptions
		wantSrc  string
		wantFile string
	}{
		{"default", pasteOptions{}, "assets/notes-1.png", "assets/notes-1.png"},
		{"explicit", pasteOptions{Folder: "media", FolderSet: true}, "media/notes-1.png", "media/notes-1.png"},
		{"opt out", pasteOptions{Folder: "", FolderSet: true}, "notes-1.png", "notes-1.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directive, err := writeImage(doc, testPNG, tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(directive, `src="`+tc.wantSrc+`"`) {
				t.Errorf("directive %q does not reference %q", directive, tc.wantSrc)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.wantFile)); err != nil {
				t.Errorf("file not written where the directive points: %v", err)
			}
		})
	}
}
