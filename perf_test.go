package prettyview

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
)

// TestRevealLineMatchesRebuild verifies the incremental reveal leaves the
// projection identical to a full rebuild and does not disturb unrelated folds.
func TestRevealLineMatchesRebuild(t *testing.T) {
	d := parse.Parse([]byte(`{"a":{"b":{"c":"deep"}},"sib":{"x":1,"y":2}}`), FormatJSON, 0)
	sib := findFoldHead(d, `"sib"`)
	b := findFoldHead(d, `"b"`)
	a := findFoldHead(d, `"a"`)
	// Collapse innermost-first so each node is visible when collapsed.
	d.Fold(sib)
	d.Fold(b)
	d.Fold(a)

	deep := lineContaining(d, "deep")
	if deep < 0 || d.Visible(deep) {
		t.Fatal("precondition: deep line should be hidden")
	}

	d.RevealLine(deep)
	got := visSnapshot(d)

	if !d.Visible(deep) {
		t.Error("deep line not revealed")
	}
	if !d.Collapsed(sib) {
		t.Error("revealing a deep line wrongly expanded an unrelated sibling")
	}

	// A full rebuild from the same collapsed bitset must yield identical visibility.
	d.Rebuild()
	for i, v := range got {
		if v != d.Visible(int32(i)) {
			t.Fatalf("incremental reveal diverged from rebuild at line %d", i)
		}
	}
}

// TestReflowBuildsEachRowOnce guards against the double-build regression: one
// reflow must build each visible row exactly once, not twice.
func TestReflowBuildsEachRowOnce(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	visible := pv.r.lastRow - pv.r.firstRow + 1
	atomic.StoreInt64(&debugRowBuilds, 0)
	pv.r.scrollToOffset(fyne.NewPos(0, 4000)) // one reflow at a fresh position
	builds := atomic.LoadInt64(&debugRowBuilds)

	t.Logf("builds=%d for ~%d visible rows", builds, visible)
	// Allow a small margin (recycled vs newly-shown rows), but never ~2x.
	if builds > int64(visible)+2 {
		t.Errorf("reflow built rows %d times for %d visible rows — double-build regressed", builds, visible)
	}
}
