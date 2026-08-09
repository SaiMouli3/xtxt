package xtxt

import "testing"

// The highlighter runs over untrusted document text and writes HTML, so the
// escaping cases matter more than the colouring ones.
func TestHighlightEscapesEverything(t *testing.T) {
	got := HighlightHTML(`a < b && c > "<script>"`, "go")
	for _, bad := range []string{"<script>", "a < b", "&& c >"} {
		if contains(got, bad) {
			t.Fatalf("unescaped %q in %q", bad, got)
		}
	}
	if !contains(got, "&lt;") || !contains(got, "&amp;") {
		t.Fatalf("expected escapes, got %q", got)
	}
}

func TestHighlightUnknownLanguageIsEscapedOnly(t *testing.T) {
	got := HighlightHTML(`if (a < b) return "x";`, "klingon")
	if contains(got, "<span") {
		t.Fatalf("unknown language should not be highlighted: %q", got)
	}
	if !contains(got, "&lt;") {
		t.Fatalf("unknown language must still be escaped: %q", got)
	}
}

func TestHighlightUnterminatedStringStopsAtNewline(t *testing.T) {
	// A stray quote must not colour the rest of the listing.
	got := HighlightHTML("x := \"oops\nfunc real() {}", "go")
	if !contains(got, `<span class="tok-kw">func</span>`) {
		t.Fatalf("code after an unterminated string lost its highlighting: %q", got)
	}
}

func TestHighlightKeywordsNeedWordBoundaries(t *testing.T) {
	got := HighlightHTML("iffy := formatter", "go")
	if contains(got, "tok-kw") {
		t.Fatalf("substring of a keyword was highlighted: %q", got)
	}
}

func TestHighlightAliases(t *testing.T) {
	if HighlightHTML("def f(): pass", "py") != HighlightHTML("def f(): pass", "python") {
		t.Fatal("alias py should resolve to python")
	}
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
