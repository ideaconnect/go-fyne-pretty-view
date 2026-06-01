package prettyview

import (
	"os"
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/geometry"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func desktopMouse(x, y float32) *desktop.MouseEvent {
	return &desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y)},
		Button:     desktop.MouseButtonPrimary,
	}
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
