package prettyview

import (
	"os"
	"testing"
)

// findFoldHead returns the first foldable node whose head line begins with the
// given key text (e.g. `"info"`), or NoNode.
func findFoldHead(d *Document, keyText string) NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold == NoNode {
			continue
		}
		segs := d.lineSegs(int32(li))
		if len(segs) > 0 && string(d.segBytes(segs[0])) == keyText {
			return l.Fold
		}
	}
	return NoNode
}

func firstFoldHeadAtDepth(d *Document, depth uint8) NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold != NoNode && l.Depth == depth {
			return l.Fold
		}
	}
	return NoNode
}

func loadDoc(t *testing.T, name string, f Format) *Document {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return parseDocument(src, f, 0)
}

func TestProjectionRoundTrip(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	fi := d.fold
	total := fi.TotalVisibleRows()
	if total == 0 {
		t.Fatal("no visible rows")
	}
	for r := int32(0); r < total; r++ {
		li := fi.lineAtRow(r)
		if got := fi.rowOfLine(li); got != r {
			t.Fatalf("round-trip failed: row %d -> line %d -> row %d", r, li, got)
		}
	}
}

func TestFoldUnfold(t *testing.T) {
	d := parseDocument([]byte(`{"a":{"x":1,"y":2},"b":3}`), FormatJSON, 0)
	fi := d.fold
	before := fi.TotalVisibleRows()

	a := findFoldHead(d, `"a"`)
	if a == NoNode {
		t.Fatal(`could not find foldable node "a"`)
	}
	n := &d.Nodes[a]
	hidden := n.CloseLine - n.HeadLine // lines (HeadLine, CloseLine] hidden on fold

	fi.fold(d, a)
	if got := fi.TotalVisibleRows(); got != before-hidden {
		t.Fatalf("after fold: visible=%d want %d (hidden=%d)", got, before-hidden, hidden)
	}
	// The head line stays visible and now renders its collapsed form.
	if fi.rowOfLine(n.HeadLine) < 0 {
		t.Fatal("head line should remain visible after fold")
	}

	fi.unfold(d, a)
	if got := fi.TotalVisibleRows(); got != before {
		t.Fatalf("after unfold: visible=%d want %d", got, before)
	}
}

func TestNestedFold(t *testing.T) {
	// Folding outer then unfolding must keep an inner-collapsed node collapsed.
	d := parseDocument([]byte(`{"outer":{"inner":{"x":1},"y":2}}`), FormatJSON, 0)
	fi := d.fold
	outer := findFoldHead(d, `"outer"`)
	inner := findFoldHead(d, `"inner"`)
	if outer == NoNode || inner == NoNode {
		t.Fatal("missing nodes")
	}
	full := fi.TotalVisibleRows()

	fi.fold(d, inner) // collapse inner first (it is visible)
	afterInner := fi.TotalVisibleRows()
	fi.fold(d, outer) // collapse outer
	fi.unfold(d, outer)
	if got := fi.TotalVisibleRows(); got != afterInner {
		t.Fatalf("inner should stay collapsed: visible=%d want %d", got, afterInner)
	}
	if !fi.collapsed.get(inner) {
		t.Fatal("inner lost its collapsed state")
	}
	fi.unfold(d, inner)
	if got := fi.TotalVisibleRows(); got != full {
		t.Fatalf("after expanding all: visible=%d want %d", got, full)
	}
}

func TestFoldNoAlloc(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	a := firstFoldHeadAtDepth(d, 1)
	if a == NoNode {
		t.Skip("no foldable node at depth 1")
	}
	allocs := testing.AllocsPerRun(50, func() {
		d.fold.fold(d, a)
		d.fold.unfold(d, a)
	})
	if allocs > 0 {
		t.Errorf("fold/unfold allocated %.1f times/op (want 0)", allocs)
	}
}

func TestExpandCollapseAll(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	full := d.fold.TotalVisibleRows()

	d.fold.collapseAll(d)
	collapsed := d.fold.TotalVisibleRows()
	if collapsed >= full {
		t.Fatalf("collapseAll did not reduce rows: %d >= %d", collapsed, full)
	}
	if collapsed == 0 {
		t.Fatal("collapseAll hid everything; top level should stay visible")
	}

	d.fold.expandAll(d)
	if got := d.fold.TotalVisibleRows(); got != full {
		t.Fatalf("expandAll did not restore: %d != %d", got, full)
	}
}
