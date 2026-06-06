package prettyview

import (
	"os"
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/geometry"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// TestNodeAtByteOffsetMatchesBruteForce validates the binary-search node lookup
// against the original O(n) deepest-container scan across many offsets of a real
// document — i.e. that SrcStart really is monotonic in id order and the parent walk
// finds the same node.
func TestNodeAtByteOffsetMatchesBruteForce(t *testing.T) {
	src, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	pv := docPV(string(src), FormatJSON)

	brute := func(off int) model.NodeID {
		best := model.NoNode
		o := uint32(off)
		for id := range pv.doc.Nodes {
			n := &pv.doc.Nodes[id]
			if n.SrcEnd == 0 && n.SrcStart == 0 {
				continue
			}
			if o >= n.SrcStart && o < n.SrcEnd {
				if best == model.NoNode || n.Depth > pv.doc.Nodes[best].Depth {
					best = model.NodeID(id)
				}
			}
		}
		return best
	}

	step := len(src)/3000 + 1
	for off := 0; off < len(src); off += step {
		if got, want := pv.nodeAtByteOffset(off), brute(off); got != want {
			t.Fatalf("offset %d: nodeAtByteOffset=%d, brute-force=%d", off, got, want)
		}
	}
}

func desktopMouse(x, y float32) *desktop.MouseEvent {
	return &desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y)},
		Button:     desktop.MouseButtonPrimary,
	}
}

func shiftMouse(x, y float32) *desktop.MouseEvent {
	ev := desktopMouse(x, y)
	ev.Modifier = fyne.KeyModifierShift
	return ev
}

func docPV(src string, f Format) *PrettyView {
	pv := New()
	pv.doc = parse.Parse([]byte(src), f, 0)
	return pv
}

func TestSelectedTextSingleLine(t *testing.T) {
	pv := docPV(`{"key":"value"}`, FormatJSON)
	li := pv.doc.LineAtRow(1) // `"key": "value"`
	pv.sel = selection{anchor: modelPos{li, 0}, focus: modelPos{li, 5}, active: true}
	if got := pv.SelectedText(); got != `"key"` {
		t.Errorf("single-line selection = %q, want %q", got, `"key"`)
	}
}

func TestSelectedTextMultiLine(t *testing.T) {
	pv := docPV(`{"a":1,"b":2}`, FormatJSON)
	// rows: 0 `{`, 1 `"a": 1,`, 2 `"b": 2`, 3 `}`
	la := pv.doc.LineAtRow(1)
	lb := pv.doc.LineAtRow(2)
	pv.sel = selection{anchor: modelPos{la, 0}, focus: modelPos{lb, pv.doc.LineRuneLen(lb)}, active: true}
	want := "\"a\": 1,\n\"b\": 2"
	if got := pv.SelectedText(); got != want {
		t.Errorf("multi-line selection = %q, want %q", got, want)
	}
}

// TestSelectedTextMatchesVisibleDisplay is the oracle for the SelectAll/Copy gather:
// the copied text must equal exactly the displayed text of each visible line joined
// by '\n' (WYSIWYG), even with folded nodes — whose head lines show a summary, not
// their hidden children. Guards the AppendDisplayLine whole-line optimization (P5)
// against regressing to expanded (non-display) text.
func TestSelectedTextMatchesVisibleDisplay(t *testing.T) {
	pv := docPV(`{"outer":{"a":1,"b":2,"c":3},"tail":9}`, FormatJSON)
	pv.doc.CollapseAll() // exercise collapsed fold-heads in the interior
	pv.SelectAll()

	var want strings.Builder
	first := true
	for li := int32(0); li < int32(pv.doc.TotalLines()); li++ {
		if !pv.doc.Visible(li) {
			continue
		}
		if !first {
			want.WriteByte('\n')
		}
		first = false
		want.WriteString(pv.doc.DisplayString(li))
	}
	if got := pv.SelectedText(); got != want.String() {
		t.Errorf("SelectAll copy != visible display:\n got: %q\nwant: %q", got, want.String())
	}
}

// TestSelectedTextRawTabsRoundTrip is the regression for the tab copy-fidelity gap
// (P6): raw-mode tab expansion renders tabs as space pads, but a copy must round-trip
// the original '\t' bytes (DESIGN.md §4.3), not the expanded spaces.
func TestSelectedTextRawTabsRoundTrip(t *testing.T) {
	const src = "a\tb\tc\nx\ty"
	pv := docPV(src, FormatRaw)
	pv.SelectAll()
	if got := pv.SelectedText(); got != src {
		t.Errorf("raw tab copy = %q, want %q (tabs must round-trip, not expand to spaces)", got, src)
	}
}

// TestSelectedTextPartialRawTabRoundTrip guards the partial-endpoint copy path
// (not just SelectAll): a mid-line selection that fully covers a raw tab must
// round-trip '\t', while one that cuts through a tab keeps the covered spaces.
// The whole-line path is covered by TestSelectedTextRawTabsRoundTrip.
func TestSelectedTextPartialRawTabRoundTrip(t *testing.T) {
	// "a\tb" renders as 'a' + a 3-space tab pad + 'b' (tabWidth 4): cols a=0,
	// pad=1..3, b=4, runeLen 5.
	pv := docPV("a\tb", FormatRaw)

	// [0,4): 'a' + the whole tab pad but not 'b' -> partial branch, tab restored.
	pv.sel = selection{anchor: modelPos{line: 0, col: 0}, focus: modelPos{line: 0, col: 4}, active: true}
	if got := pv.SelectedText(); got != "a\t" {
		t.Errorf("partial copy covering the tab = %q, want %q (tab must round-trip)", got, "a\t")
	}

	// [0,3): 'a' + only 2 of the 3 pad columns -> tab partially cut, keep spaces.
	pv.sel = selection{anchor: modelPos{line: 0, col: 0}, focus: modelPos{line: 0, col: 3}, active: true}
	if got := pv.SelectedText(); got != "a  " {
		t.Errorf("partial copy cutting the tab = %q, want %q (covered spaces)", got, "a  ")
	}
}

// TestCopySubtreeIncludesFoldedChildren guards CopySubtree/subtreeText (P7):
// serializing a node's subtree must include its children regardless of fold state.
func TestCopySubtreeIncludesFoldedChildren(t *testing.T) {
	const src = `{"outer":{"a":1,"b":2},"tail":9}`
	pv := docPV(src, FormatJSON)
	node := pv.nodeAtByteOffset(strings.Index(src, `{"a"`)) // the inner object's '{'
	if node == model.NoNode {
		t.Fatal("expected a node spanning the offset")
	}
	pv.doc.CollapseAll() // fold everything; the subtree text must still be complete
	got := pv.subtreeText(node)
	if !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) {
		t.Errorf("CopySubtree omitted folded children:\n%s", got)
	}
}

func TestSelectionOrderIndependentOfDirection(t *testing.T) {
	pv := docPV(`{"a":1,"b":2}`, FormatJSON)
	la := pv.doc.LineAtRow(1)
	lb := pv.doc.LineAtRow(2)
	// Anchor after focus (backward drag) must yield the same text.
	pv.sel = selection{anchor: modelPos{lb, pv.doc.LineRuneLen(lb)}, focus: modelPos{la, 0}, active: true}
	want := "\"a\": 1,\n\"b\": 2"
	if got := pv.SelectedText(); got != want {
		t.Errorf("backward selection = %q, want %q", got, want)
	}
}

func TestCopyAfterCollapseIsWYSIWYG(t *testing.T) {
	pv := docPV(`{"a":{"x":1},"b":2}`, FormatJSON)
	a := findFoldHead(pv.doc, `"a"`)
	pv.doc.Fold(a) // row 1 now shows: "a": { 1 item },

	total := pv.doc.TotalVisibleRows()
	first := pv.doc.LineAtRow(0)
	last := pv.doc.LineAtRow(total - 1)
	pv.sel = selection{anchor: modelPos{first, 0}, focus: modelPos{last, pv.doc.LineRuneLen(last)}, active: true}

	got := pv.SelectedText()
	if !strings.Contains(got, "1 item") {
		t.Errorf("collapsed copy should contain the summary; got:\n%s", got)
	}
	if strings.Contains(got, `"x": 1`) {
		t.Errorf("collapsed copy must NOT contain hidden children; got:\n%s", got)
	}
}

func TestWordBounds(t *testing.T) {
	pv := docPV(`{"hello":"world"}`, FormatJSON)
	li := pv.doc.LineAtRow(1)    // `"hello": "world"`
	a, b := pv.wordBounds(li, 2) // inside "hello"
	if a.col != 1 || b.col != 6 {
		t.Errorf("word bounds = [%d,%d), want [1,6)", a.col, b.col)
	}
}

func TestLineBounds(t *testing.T) {
	pv := docPV(`{"k":"v"}`, FormatJSON)
	li := pv.doc.LineAtRow(1)
	a, b := pv.lineBounds(li)
	if a.col != 0 || b.col != pv.doc.LineRuneLen(li) {
		t.Errorf("line bounds = [%d,%d), want [0,%d)", a.col, b.col, pv.doc.LineRuneLen(li))
	}
}

func TestSelectAllAndClear(t *testing.T) {
	pv := docPV(`{"a":1,"b":2}`, FormatJSON)
	pv.SelectAll()
	got := pv.SelectedText()
	if !strings.Contains(got, `"a": 1`) || !strings.Contains(got, `"b": 2`) {
		t.Errorf("select-all missing content:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n != int(pv.doc.TotalVisibleRows())-1 {
		t.Errorf("select-all newline count = %d, want %d", n, int(pv.doc.TotalVisibleRows())-1)
	}
	pv.ClearSelection()
	if pv.SelectedText() != "" {
		t.Error("clear did not empty the selection")
	}
}

func TestSelectionRectCountBounded(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	pv.SelectAll()
	pv.r.reflow() // selection spans the whole 440k-row doc
	rects := len(pv.r.selLayer.Objects)
	bound := int(600/pv.met.RowH) + 4
	t.Logf("selection rects for select-all of 7.5MB doc = %d (bound %d)", rects, bound)
	if rects > bound {
		t.Errorf("selection rect count %d exceeds visible bound %d", rects, bound)
	}
}

func TestDragSelectionIntegration(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON, 600, 400)
	defer win.Close()

	// Press near the start of row 1's content, drag to row 2's end.
	li1 := pv.doc.LineAtRow(1)
	li2 := pv.doc.LineAtRow(2)
	x1, y1 := geometry.CellOrigin(pv.doc, pv.met, li1, 0)
	x2, y2 := geometry.CellOrigin(pv.doc, pv.met, li2, pv.doc.LineRuneLen(li2))

	pv.MouseDown(desktopMouse(x1, y1+1))
	pv.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x2, y2+1)}})
	pv.DragEnd()

	got := pv.SelectedText()
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Errorf("drag selection missing expected content:\n%s", got)
	}
}

// TestShiftClickWithoutPriorSelection is the regression for bug #8: a shift-click
// on a fresh widget (no caret established) must NOT select from the document
// origin — it places the caret at the click. A subsequent shift-click then
// extends from that caret.
func TestShiftClickWithoutPriorSelection(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON, 600, 400)
	defer win.Close()

	li2 := pv.doc.LineAtRow(2)
	x, y := geometry.CellOrigin(pv.doc, pv.met, li2, 3)
	pv.MouseDown(shiftMouse(x, y+1)) // shift-click, nothing selected before

	if got := pv.SelectedText(); got != "" {
		t.Errorf("shift-click with no prior selection must not select; got %q", got)
	}

	// A second shift-click now extends from the just-placed caret (row 2 -> row 1),
	// and must not reach row 0 ('{').
	li1 := pv.doc.LineAtRow(1)
	x1, y1 := geometry.CellOrigin(pv.doc, pv.met, li1, 0)
	pv.MouseDown(shiftMouse(x1, y1+1))
	got := pv.SelectedText()
	if got == "" || strings.Contains(got, "{") {
		t.Errorf("second shift-click should select between carets (rows 1-2), not from the top; got %q", got)
	}
}

// TestShiftClickExtendsFromRealTopClick guards the inverse: a genuine plain click
// at (line 0, col 0) followed by a shift-click DOES select from the top — the fix
// must distinguish a real top click from the zero-value default (so a bool flag is
// required, not an "anchor == {0,0}" heuristic). Raw text is used so line 0 is
// selectable text rather than a fold-triangle row.
func TestShiftClickExtendsFromRealTopClick(t *testing.T) {
	pv, win := renderInWindow(t, []byte("alpha\nbeta\ngamma"), FormatRaw, 600, 400)
	defer win.Close()

	x0, y0 := geometry.CellOrigin(pv.doc, pv.met, pv.doc.LineAtRow(0), 0)
	pv.MouseDown(desktopMouse(x0, y0+1)) // real caret at the document top -> modelPos{0,0}

	li2 := pv.doc.LineAtRow(2)
	x2, y2 := geometry.CellOrigin(pv.doc, pv.met, li2, pv.doc.LineRuneLen(li2))
	pv.MouseDown(shiftMouse(x2, y2+1))

	if got := pv.SelectedText(); !strings.Contains(got, "alpha") {
		t.Errorf("shift-click after a real top click should select from the top; got %q", got)
	}
}

// TestShiftClickAfterClearDoesNotSelectFromTop ensures Escape (ClearSelection)
// resets the caret so a following shift-click is a no-op, not a top selection.
func TestShiftClickAfterClearDoesNotSelectFromTop(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON, 600, 400)
	defer win.Close()

	// Establish a selection, then clear it (as Escape does).
	pv.SelectAll()
	pv.ClearSelection()

	li2 := pv.doc.LineAtRow(2)
	x, y := geometry.CellOrigin(pv.doc, pv.met, li2, 3)
	pv.MouseDown(shiftMouse(x, y+1))
	if got := pv.SelectedText(); got != "" {
		t.Errorf("shift-click after ClearSelection must not select from the top; got %q", got)
	}
}

// TestSetDataClearsStaleSelection is the regression for the stale-selection bug:
// loading a new (smaller) document must drop the old selection — both so the
// cleared text is empty and so resolving a now-out-of-range line can't panic.
func TestSetDataClearsStaleSelection(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON, 600, 400)
	defer win.Close()

	pv.SelectAll()
	if pv.SelectedText() == "" {
		t.Fatal("precondition: expected a selection")
	}
	pv.SetData([]byte(`{}`), FormatJSON) // far fewer lines than the old selection spanned
	if got := pv.SelectedText(); got != "" {
		t.Errorf("selection should be cleared after SetData; got %q", got)
	}
}

// TestSetDataEmptyClearsHighlightRects checks the empty-document reflow branch
// drops leftover selection rectangles instead of leaving them painted.
func TestSetDataEmptyClearsHighlightRects(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON, 600, 400)
	defer win.Close()

	pv.SelectAll()
	pv.r.reflow()
	if len(pv.r.selLayer.Objects) == 0 {
		t.Fatal("precondition: expected selection rectangles on screen")
	}
	pv.SetData([]byte(""), FormatRaw) // empty document -> total visible rows 0
	pv.r.reflow()
	if n := len(pv.r.selLayer.Objects); n != 0 {
		t.Errorf("empty document should clear selection rectangles; got %d", n)
	}
}

// TestApplyHitHonorsWordGrab covers the fix shared by Dragged and the edge
// auto-scroll: with a word grab active, advancing the hit position must extend the
// selection to whole words (not collapse the focus to a single character — the bug
// that surfaced specifically during edge auto-scroll).
func TestApplyHitHonorsWordGrab(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"hello":"world"}`), FormatJSON, 600, 400)
	defer win.Close()

	li := pv.doc.LineAtRow(1) // `"hello": "world"`
	a, b := pv.wordBounds(li, 2)
	pv.sel.grab, pv.sel.grabA, pv.sel.grabB = grabWord, a, b
	pv.sel.anchor, pv.sel.focus = a, b
	pv.sel.active = true

	disp := pv.doc.DisplayString(li)
	wcol := strings.Index(disp, "world") // ASCII: byte index == rune column
	if wcol < 0 {
		t.Fatal("could not locate 'world' in the displayed line")
	}
	pv.applyHit(modelPos{line: li, col: wcol + 1}) // a position inside "world"

	got := pv.SelectedText()
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("word-grab drag should span whole words 'hello'..'world'; got %q", got)
	}
}
