package prettyview

import (
	"os"
	"strings"
	"testing"
)

// TestDesignDocHasV2EditSection is the doc-drift guard for the v2.0.0 design spike
// (issue #36). The "v2 edit model" section of docs/DESIGN.md is the ratified contract
// every other v2.0.0 issue cites; if it is deleted or its load-bearing decisions are
// reworded away, the milestone silently loses its grounding. This asserts the section
// exists and still states: the chosen strategy (gap buffer, rebuild-from-bytes), the
// construction-time-immutable mode (no SetEditable, runtime flip deferred to #54), and
// the seven-invariant trade table with its two consciously-traded invariants.
//
// It mirrors this repo's habit of testing its own docs against the code's contract.
func TestDesignDocHasV2EditSection(t *testing.T) {
	b, err := os.ReadFile("docs/DESIGN.md")
	if err != nil {
		t.Fatalf("read docs/DESIGN.md: %v", err)
	}
	doc := string(b)

	// Each anchor pins one load-bearing claim of the design spike. Keep the strings
	// short and verbatim so an intentional rewrite updates this list deliberately.
	anchors := []struct{ what, substr string }{
		{"section heading", "## 12. v2 edit model"},
		{"rebuild-from-bytes strategy", "never mutate the model"},
		{"gap-buffer choice", "gap buffer"},
		{"construction-time mode", "made once at construction"},
		{"no runtime flip", "no `SetEditable`"},
		{"runtime flip deferred to Future Features", "#54"},
		{"invariant-trade table", "The seven invariants and what v2 trades"},
		{"two traded invariants", "Traded"},
	}
	for _, a := range anchors {
		if !strings.Contains(doc, a.substr) {
			t.Errorf("docs/DESIGN.md missing %s: expected to contain %q", a.what, a.substr)
		}
	}
}
