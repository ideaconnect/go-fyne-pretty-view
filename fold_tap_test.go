package prettyview

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

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
	tx := pv.met.triangleX(pv.doc.Lines[li].Depth) + 1

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
	textX := pv.met.textOriginX(pv.doc.Lines[li].Depth) + 5
	pv.Tapped(&fyne.PointEvent{Position: fyne.NewPos(textX, 1)})
	if got := int(pv.doc.TotalVisibleRows()); got != full {
		t.Fatalf("tap on text should not fold: visible=%d, want %d", got, full)
	}
}
