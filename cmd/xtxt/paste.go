package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// clipboardImage reads a PNG from the system clipboard.
//
// Every platform exposes this differently and none of them expose it well, so
// this shells out to whatever the platform already ships rather than pulling in
// a GUI toolkit for one feature.
func clipboardImage() ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return macClipboardImage()
	case "windows":
		return windowsClipboardImage()
	default:
		return unixClipboardImage()
	}
}

// macClipboardImage uses AppleScript, which is present on every Mac. The
// clipboard is written to a temp file because osascript's stdout mangles
// binary data.
func macClipboardImage() ([]byte, error) {
	tmp, err := os.CreateTemp("", "xtxt-paste-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(`
		set outFile to POSIX file %q
		try
			set imageData to the clipboard as «class PNGf»
		on error
			return "no-image"
		end try
		set fh to open for access outFile with write permission
		set eof fh to 0
		write imageData to fh
		close access fh
		return "ok"
	`, path)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, fmt.Errorf("reading the clipboard: %w", err)
	}
	if strings.TrimSpace(string(out)) != "ok" {
		return nil, errNoImage
	}
	return os.ReadFile(path)
}

func windowsClipboardImage() ([]byte, error) {
	tmp, err := os.CreateTemp("", "xtxt-paste-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		$img = [Windows.Forms.Clipboard]::GetImage()
		if ($img -eq $null) { exit 1 }
		$img.Save(%q, [System.Drawing.Imaging.ImageFormat]::Png)
	`, path)

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	if err := cmd.Run(); err != nil {
		return nil, errNoImage
	}
	return os.ReadFile(path)
}

// unixClipboardImage tries Wayland then X11, since a Linux desktop may be
// either and neither tool is guaranteed to be installed.
func unixClipboardImage() ([]byte, error) {
	candidates := [][]string{
		{"wl-paste", "--type", "image/png"},
		{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"},
	}
	var missing []string
	for _, argv := range candidates {
		if _, err := exec.LookPath(argv[0]); err != nil {
			missing = append(missing, argv[0])
			continue
		}
		out, err := exec.Command(argv[0], argv[1:]...).Output()
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	if len(missing) == len(candidates) {
		return nil, fmt.Errorf("no clipboard tool found; install wl-clipboard or xclip")
	}
	return nil, errNoImage
}

type clipboardError string

func (e clipboardError) Error() string { return string(e) }

const errNoImage = clipboardError("no image on the clipboard")

// imageExtension sniffs the format from its magic bytes. Clipboard images are
// usually PNG, but a paste from another app may not be.
func imageExtension(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return ".gif"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) > 12 && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp"
	case bytes.HasPrefix(bytes.TrimSpace(data), []byte("<svg")),
		bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")):
		return ".svg"
	default:
		return ".png"
	}
}

func mimeForExtension(ext string) string {
	switch ext {
	case ".jpg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "image/png"
	}
}

// pasteOptions controls where a pasted image ends up.
type pasteOptions struct {
	// Embed writes the image into the document as a data: URI instead of
	// saving it alongside. Self-contained, at roughly 33% size overhead.
	Embed bool
	// Caption and Alt are optional; alt defaults to the caption.
	Caption string
	Alt     string
	// Width, if set, is passed through to the directive.
	Width string
	// Folder is where a saved image is written, relative to the document.
	// Empty means beside the document, which is what "" from --folder
	// requests explicitly; unset means defaultPasteFolder.
	Folder string
	// FolderSet distinguishes "--folder ''" from "--folder never given".
	FolderSet bool
}

// defaultPasteFolder keeps pasted media out of the document's own directory.
// A folder of notes accumulates screenshots quickly, and a directory holding
// three documents and forty PNGs is unreadable; the media belongs together and
// out of the way. Pass --folder "" to opt out and write beside the document.
const defaultPasteFolder = "assets"

// pasteImage appends the clipboard image to doc and returns the directive it
// wrote. The document is only touched once the image has been written, so a
// failure part-way leaves nothing half-done.
func pasteImage(doc string, opt pasteOptions) (string, error) {
	data, err := clipboardImage()
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errNoImage
	}

	directive, err := writeImage(doc, data, opt)
	if err != nil {
		return "", err
	}

	if err := appendToDocument(doc, directive); err != nil {
		return "", err
	}
	return directive, nil
}

// writeImage stores the image and returns the directive that references it,
// without touching the document. Separated from pasteImage so the placement
// rules can be tested without a clipboard.
func writeImage(doc string, data []byte, opt pasteOptions) (string, error) {
	ext := imageExtension(data)

	if opt.Embed {
		return buildImageDirective(
			"data:"+mimeForExtension(ext)+";base64,"+base64.StdEncoding.EncodeToString(data), opt), nil
	}

	folder := opt.Folder
	if !opt.FolderSet {
		folder = defaultPasteFolder
	}
	dir := filepath.Dir(doc)
	if folder != "" {
		dir = filepath.Join(dir, folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	name, err := uniqueImageName(dir, strings.TrimSuffix(filepath.Base(doc), filepath.Ext(doc)), ext)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}

	// The directive must point where the file actually went. Always a forward
	// slash: this is a document reference, not a filesystem path, and it has to
	// resolve on every platform that opens the file.
	src := name
	if folder != "" {
		src = folder + "/" + name
	}
	return buildImageDirective(src, opt), nil
}

// uniqueImageName finds the first free `<base>-N<ext>` in dir.
func uniqueImageName(dir, base, ext string) (string, error) {
	if base == "" {
		base = "image"
	}
	for i := 1; i < 10000; i++ {
		name := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not find an unused filename in %s", dir)
}

func buildImageDirective(src string, opt pasteOptions) string {
	alt := opt.Alt
	if alt == "" {
		alt = opt.Caption
	}

	var b strings.Builder
	b.WriteString("@image(\n")
	fmt.Fprintf(&b, "    src=%q", src)
	if alt != "" {
		fmt.Fprintf(&b, ",\n    alt=%q", alt)
	}
	if opt.Caption != "" {
		fmt.Fprintf(&b, ",\n    caption=%q", opt.Caption)
	}
	if opt.Width != "" {
		fmt.Fprintf(&b, ",\n    width=%s", opt.Width)
	}
	b.WriteString("\n)")
	return b.String()
}

// appendToDocument adds the directive at the end, separated by a blank line.
func appendToDocument(path, directive string) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existing = nil
	}

	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 {
		trimmed := strings.TrimRight(string(existing), "\n")
		b.Reset()
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString(directive)
	b.WriteString("\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
