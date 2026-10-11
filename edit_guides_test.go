package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// liveRow returns the live row widget showing sub-row sub of display line li, or nil.
func liveRow(pv *PrettyView, li, sub int32) *rowWidget {
	for _, rw := range pv.r.live {
		if rw.line == li && rw.sub == sub {
			return rw
		}
	}
	return nil
}

// guideXs returns the x of every visible indent guide of a live row, left to right.
func guideXs(rw *rowWidget) []float32 {
	var xs []float32
	if rw == nil || rw.rr == nil {
		return nil
	}
	for _, g := range rw.rr.guides {
		if g.Visible() {
			xs = append(xs, g.Position1.X)
		}
	}
	return xs
}

func editorInWindow(t *testing.T, src string, w, h float32, opts ...Option) (*PrettyView, fyne.Window) {
	t.Helper()
	test.NewApp()
	pv := New(append([]Option{WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff})}, opts...)...)
	pv.SetData([]byte(src), FormatJSON)
	win := test.NewWindow(pv)
	win.Resize(fyne.NewSize(w, h))
	pv.Refresh()
	return pv, win
}

// TestEditModeIndentGuidesOnIndentColumns: an editable row draws one guide per indent
// level of its leading whitespace, each centered on the indentation column it marks
// (column i*unit), and a flush line draws none. The unit is detected from the buffer, so a
// four-space document gets guides every four columns, not every two.
func TestEditModeIndentGuidesOnIndentColumns(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int32
		cols []int // expected guide columns
	}{
		{"two-space depth 2", "{\n  \"a\": {\n    \"b\": 1\n  }\n}", 2, []int{0, 2}},
		{"two-space depth 1", "{\n  \"a\": {\n    \"b\": 1\n  }\n}", 1, []int{0}},
		{"flush line", "{\n  \"a\": {\n    \"b\": 1\n  }\n}", 0, nil},
		{"four-space depth 2", "{\n    \"a\": {\n        \"b\": 1\n    }\n}", 2, []int{0, 4}},
		{"tab-indented depth 2", "{\n\t\"a\": {\n\t\t\"b\": 1\n\t}\n}", 2, []int{0, 1}},
		{"blank line inside a block", "{\n  \"a\": {\n\n    \"b\": 1\n  }\n}", 2, []int{0, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pv, win := editorInWindow(t, c.src, 600, 400)
			defer win.Close()
			rw := liveRow(pv, c.line, 0)
			if rw == nil {
				t.Fatalf("line %d has no live row", c.line)
			}
			got := guideXs(rw)
			if len(got) != len(c.cols) {
				t.Fatalf("line %d: %d guides at %v, want %d (cols %v)", c.line, len(got), got, len(c.cols), c.cols)
			}
			m := pv.met
			for i, col := range c.cols {
				left, right := m.ColX(0, col), m.ColX(0, col+1)
				if got[i] <= left || got[i] >= right {
					t.Errorf("guide %d at x=%.1f, want inside column %d's cell (%.1f, %.1f)", i, got[i], col, left, right)
				}
				if want := editGuideX(m, 0, col); got[i] != want {
					t.Errorf("guide %d at x=%.1f, want %.1f", i, got[i], want)
				}
			}
		})
	}
}

// TestEditModeGuidesFollowEdits: guides are recomputed on every keystroke reproject —
// indenting a flush line gives it a guide, and the guide is gone again after undo.
func TestEditModeGuidesFollowEdits(t *testing.T) {
	pv, win := editorInWindow(t, "{\n  \"a\": 1,\n\"b\": 2\n}", 600, 400)
	defer win.Close()
	if n := len(guideXs(liveRow(pv, 2, 0))); n != 0 {
		t.Fatalf("flush line 2 has %d guides before the edit, want 0", n)
	}
	pv.SetCaret(2, 0)
	pv.TypedRune(' ')
	pv.TypedRune(' ')
	if n := len(guideXs(liveRow(pv, 2, 0))); n != 1 {
		t.Errorf("after indenting line 2 by two spaces: %d guides, want 1", n)
	}
	pv.Undo()
	if n := len(guideXs(liveRow(pv, 2, 0))); n != 0 {
		t.Errorf("after undo: %d guides on line 2, want 0", n)
	}
}

// TestEditModeGuidesWrapAndScroll: under soft-wrap only a line's first visual row shows
// guides (continuation rows restart at column 0, where a guide would cross the text); with
// horizontal scrolling the guides stay at their content-space columns, so the scroll moves
// them with the text.
func TestEditModeGuidesWrapAndScroll(t *testing.T) {
	long := strings.Repeat("wxyz ", 60)
	src := "{\n  \"a\": {\n    \"k\": \"" + long + "\"\n  }\n}"

	pv, win := editorInWindow(t, src, 400, 600, WithWrap(WrapWord))
	defer win.Close()
	if rows := pv.doc.RowsOfLine(2); rows < 2 {
		t.Fatalf("line 2 should wrap, got %d rows", rows)
	}
	if n := len(guideXs(liveRow(pv, 2, 0))); n != 2 {
		t.Errorf("wrapped line's first row: %d guides, want 2", n)
	}
	for sub := int32(1); sub < pv.doc.RowsOfLine(2); sub++ {
		rw := liveRow(pv, 2, sub)
		if rw == nil {
			t.Fatalf("continuation row %d not live", sub)
		}
		if n := len(guideXs(rw)); n != 0 {
			t.Errorf("continuation row %d: %d guides, want 0", sub, n)
		}
	}

	pv2, win2 := editorInWindow(t, src, 400, 600)
	defer win2.Close()
	before := guideXs(liveRow(pv2, 2, 0))
	pv2.r.scrollToOffset(fyne.NewPos(300, 0))
	after := guideXs(liveRow(pv2, 2, 0))
	if pv2.viewOffX == 0 {
		t.Fatal("horizontal scroll did not move the view")
	}
	if len(before) != 2 || len(after) != 2 || before[0] != after[0] || before[1] != after[1] {
		t.Errorf("guides moved in content space under horizontal scroll: before %v, after %v", before, after)
	}
}

// TestReadOnlyIndentGuidesUnchanged pins the read-only guides: one per Line.Depth, at the
// left edge of each level's triangle gutter, independent of the editable code path.
func TestReadOnlyIndentGuidesUnchanged(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":{"b":{"c":1}}}`), FormatJSON, 600, 400)
	defer win.Close()
	for li := int32(0); li < int32(len(pv.doc.Lines)); li++ {
		depth := pv.doc.Lines[li].Depth
		got := guideXs(liveRow(pv, li, 0))
		if len(got) != int(depth) {
			t.Fatalf("line %d depth %d: %d guides", li, depth, len(got))
		}
		for i, x := range got {
			if want := pv.met.TriangleX(uint8(i)) + 1; x != want {
				t.Errorf("line %d guide %d at %.1f, want %.1f", li, i, x, want)
			}
		}
	}
}

// TestEditModeGuidesCapped: a pathologically deep indent draws at most maxIndentGuides
// guides per row, the same render-object budget the read-only view keeps.
func TestEditModeGuidesCapped(t *testing.T) {
	src := "{\n  \"a\": 1,\n" + strings.Repeat(" ", 2*(maxIndentGuides+20)) + "\"deep\": 2\n}"
	pv, win := editorInWindow(t, src, 600, 400)
	defer win.Close()
	if n := len(guideXs(liveRow(pv, 2, 0))); n != maxIndentGuides {
		t.Errorf("deep line: %d guides, want the cap %d", n, maxIndentGuides)
	}
}
