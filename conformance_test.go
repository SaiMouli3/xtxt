package xtxt

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the expected conformance output")

// TestConformance runs every case in conformance/cases and compares the parse
// against the recorded expectation. The same directory is the acceptance test
// for the Python and JavaScript SDKs, which is what keeps three independent
// implementations honest about being one format.
func TestConformance(t *testing.T) {
	inputs, err := filepath.Glob("conformance/cases/*.xtxt")
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no conformance cases found: %v", err)
	}
	for _, in := range inputs {
		t.Run(strings.TrimSuffix(filepath.Base(in), ".xtxt"), func(t *testing.T) {
			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			res := ParseString(string(src))
			got := map[string]any{
				"ast":    Canonical(res.Doc),
				"issues": CanonicalIssues(append(append([]Issue{}, res.Issues...), Validate(res.Doc)...)),
			}
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotJSON = append(gotJSON, '\n')

			want := strings.TrimSuffix(in, ".xtxt") + ".json"
			if *update {
				if err := os.WriteFile(want, gotJSON, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			wantJSON, err := os.ReadFile(want)
			if err != nil {
				t.Fatalf("%v (run: go test -run TestConformance -update)", err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("conformance mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					in, gotJSON, wantJSON)
			}
		})
	}
}
