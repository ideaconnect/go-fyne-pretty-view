package prettyview

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// TestCollapseExpandToDepth covers runtime fold-to-depth at the model level:
// CollapseToDepth(d) collapses foldable nodes at depth >= d (leaving shallower ones),
// ExpandToDepth(d) expands those at depth < d, and the two compose.
func TestCollapseExpandToDepth(t *testing.T) {
	pv := docPV(`{"a":{"b":{"c":1}},"d":2}`, FormatJSON)
	d := pv.doc
	full := d.TotalVisibleRows()
	a := findFoldHead(d, `"a"`) // depth 1
	b := findFoldHead(d, `"b"`) // depth 2

	d.CollapseToDepth(2)
	if d.Collapsed(a) || !d.Collapsed(b) {
		t.Errorf("CollapseToDepth(2): a-collapsed=%v b-collapsed=%v, want false/true",
			d.Collapsed(a), d.Collapsed(b))
	}
	if cLine := lineContaining(d, `"c"`); d.Visible(cLine) {
		t.Error("'c' (depth 3) should be hidden after CollapseToDepth(2)")
	}

	d.ExpandToDepth(3)
	if d.Collapsed(b) {
		t.Error("ExpandToDepth(3) should expand the depth-2 'b' object")
	}
	if got := d.TotalVisibleRows(); got != full {
		t.Errorf("ExpandToDepth(3) rows = %d, want full %d", got, full)
	}
}

// TestPrettyViewFoldToDepth exercises the PrettyView wrappers (which also Refresh).
func TestPrettyViewFoldToDepth(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":{"b":{"c":1}}}`), FormatJSON, 400, 300)
	defer win.Close()

	pv.CollapseToDepth(2)
	if !pv.doc.Collapsed(findFoldHead(pv.doc, `"b"`)) {
		t.Error("pv.CollapseToDepth(2) did not collapse the depth-2 'b' object")
	}
	pv.ExpandToDepth(99)
	if pv.doc.Collapsed(findFoldHead(pv.doc, `"b"`)) {
		t.Error("pv.ExpandToDepth(99) did not expand 'b'")
	}
}

func TestTapTogglesFold(t *testing.T) {
	src, err := os.ReadFile("testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	full := int(pv.doc.TotalVisibleRows())

	// The top-level object's fold head is at row 0, depth 0.
	li := pv.doc.LineAtRow(0)
	if pv.doc.Lines[li].Fold == model.NoNode {
		t.Fatal("expected the first row to be a fold head")
	}
	tx := pv.met.TriangleX(pv.doc.Lines[li].Depth) + 1

	// Tap the triangle: collapses the whole document to one summary row.
	pv.Tapped(&fyne.PointEvent{Position: fyne.NewPos(tx, 1)})
	if got := int(pv.doc.TotalVisibleRows()); got != 1 {
		t.Fatalf("after collapsing root: visible=%d, want 1", got)
	}

	// Tap again: restores.
	pv.Tapped(&fyne.PointEvent{Position: fyne.NewPos(tx, 1)})
	if got := int(pv.doc.TotalVisibleRows()); got != full {
		t.Fatalf("after expanding root: visible=%d, want %d", got, full)
	}

	// A tap on the text area (not the gutter) must NOT toggle.
	textX := pv.met.TextOriginX(pv.doc.Lines[li].Depth) + 5
	pv.Tapped(&fyne.PointEvent{Position: fyne.NewPos(textX, 1)})
	if got := int(pv.doc.TotalVisibleRows()); got != full {
		t.Fatalf("tap on text should not fold: visible=%d, want %d", got, full)
	}
}
