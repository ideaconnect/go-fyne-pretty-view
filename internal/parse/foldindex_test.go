package parse

import (
	"os"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// findFoldHead returns the first foldable node whose head line begins with the
// given key text (e.g. `"info"`), or model.NoNode.
func findFoldHead(d *model.Document, keyText string) model.NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold == model.NoNode {
			continue
		}
		segs := d.LineSegs(int32(li))
		if len(segs) > 0 && string(d.SegBytes(segs[0])) == keyText {
			return l.Fold
		}
	}
	return model.NoNode
}

func firstFoldHeadAtDepth(d *model.Document, depth uint8) model.NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold != model.NoNode && l.Depth == depth {
			return l.Fold
		}
	}
	return model.NoNode
}

func loadDoc(t *testing.T, name string, f Format) *model.Document {
	t.Helper()
	src, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(src, f, 0)
}

func TestProjectionRoundTrip(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	total := d.TotalVisibleRows()
	if total == 0 {
		t.Fatal("no visible rows")
	}
	for r := int32(0); r < total; r++ {
		li := d.LineAtRow(r)
		if got := d.RowOfLine(li); got != r {
			t.Fatalf("round-trip failed: row %d -> line %d -> row %d", r, li, got)
		}
	}
}

func TestFoldUnfold(t *testing.T) {
	d := Parse([]byte(`{"a":{"x":1,"y":2},"b":3}`), FormatJSON, 0)
	before := d.TotalVisibleRows()

	a := findFoldHead(d, `"a"`)
	if a == model.NoNode {
		t.Fatal(`could not find foldable node "a"`)
	}
	n := &d.Nodes[a]
	hidden := n.CloseLine - n.HeadLine // lines (HeadLine, CloseLine] hidden on fold

	d.Fold(a)
	if got := d.TotalVisibleRows(); got != before-hidden {
		t.Fatalf("after fold: visible=%d want %d (hidden=%d)", got, before-hidden, hidden)
	}
	// The head line stays visible and now renders its collapsed form.
	if d.RowOfLine(n.HeadLine) < 0 {
		t.Fatal("head line should remain visible after fold")
	}

	d.Unfold(a)
	if got := d.TotalVisibleRows(); got != before {
		t.Fatalf("after unfold: visible=%d want %d", got, before)
	}
}

func TestNestedFold(t *testing.T) {
	// Folding outer then unfolding must keep an inner-collapsed node collapsed.
	d := Parse([]byte(`{"outer":{"inner":{"x":1},"y":2}}`), FormatJSON, 0)
	outer := findFoldHead(d, `"outer"`)
	inner := findFoldHead(d, `"inner"`)
	if outer == model.NoNode || inner == model.NoNode {
		t.Fatal("missing nodes")
	}
	full := d.TotalVisibleRows()

	d.Fold(inner) // collapse inner first (it is visible)
	afterInner := d.TotalVisibleRows()
	d.Fold(outer) // collapse outer
	d.Unfold(outer)
	if got := d.TotalVisibleRows(); got != afterInner {
		t.Fatalf("inner should stay collapsed: visible=%d want %d", got, afterInner)
	}
	if !d.Collapsed(inner) {
		t.Fatal("inner lost its collapsed state")
	}
	d.Unfold(inner)
	if got := d.TotalVisibleRows(); got != full {
		t.Fatalf("after expanding all: visible=%d want %d", got, full)
	}
}

func TestFoldNoAlloc(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	a := firstFoldHeadAtDepth(d, 1)
	if a == model.NoNode {
		t.Fatal("openapi.json must have a foldable node at depth 1 (#78: was a silent skip)")
	}
	allocs := testing.AllocsPerRun(50, func() {
		d.Fold(a)
		d.Unfold(a)
	})
	if allocs > 0 {
		t.Errorf("fold/unfold allocated %.1f times/op (want 0)", allocs)
	}
}

func TestExpandCollapseAll(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	full := d.TotalVisibleRows()

	d.CollapseAll()
	collapsed := d.TotalVisibleRows()
	if collapsed >= full {
		t.Fatalf("collapseAll did not reduce rows: %d >= %d", collapsed, full)
	}
	if collapsed == 0 {
		t.Fatal("collapseAll hid everything; top level should stay visible")
	}

	d.ExpandAll()
	if got := d.TotalVisibleRows(); got != full {
		t.Fatalf("expandAll did not restore: %d != %d", got, full)
	}
}
