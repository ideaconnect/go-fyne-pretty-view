package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// TestE2EUserFlow drives the viewer the way a user would — through real simulated
// input events on a real widget tree (Fyne's in-process test driver, the native-GUI
// analog of a browser automation tool) — rather than calling PrettyView's methods
// directly. It exercises a full flow: collapse a node by tapping its fold triangle,
// then type a query into a real search box that finds the now-hidden match and
// auto-reveals it, then tick the Wrap checkbox to switch to soft-wrap.
func TestE2EUserFlow(t *testing.T) {
	test.NewApp()

	src := []byte(`{"info":{"title":"Demo","tags":["a","b","c"]},"count":3}`)
	pv := NewWithData(src, FormatJSON)

	// A real, minimal UI: a search entry wired to the viewer + the built-in Wrap
	// checkbox, with the viewer filling the rest — like a host app would assemble.
	search := widget.NewEntry()
	search.OnChanged = func(s string) { pv.Search(SearchQuery{Text: s}) }
	wrap := NewWrapToggle(pv).(*widget.Button)
	ui := container.NewBorder(container.NewVBox(search, wrap), nil, nil, nil, pv)

	win := test.NewWindow(ui)
	defer win.Close()
	win.Resize(fyne.NewSize(520, 420))

	// --- 1. Collapse "info" by tapping its fold triangle in the gutter. ---
	info := findFoldHead(pv.doc, `"info"`)
	if info == model.NoNode {
		t.Fatal("fixture has no \"info\" object")
	}
	before := pv.doc.TotalVisibleRows()
	head := pv.doc.Nodes[info].HeadLine
	depth := pv.doc.Lines[head].Depth
	triangle := fyne.NewPos(
		pv.met.TriangleX(depth)+2,                  // inside the triangle hot-zone
		pv.met.RowY(int(pv.doc.RowOfLine(head)))+1, // this row
	)
	test.TapAt(pv, triangle)

	if !pv.doc.Collapsed(info) {
		t.Fatal("tapping the fold triangle did not collapse \"info\"")
	}
	if pv.doc.TotalVisibleRows() >= before {
		t.Errorf("collapse did not hide rows: %d >= %d", pv.doc.TotalVisibleRows(), before)
	}

	// --- 2. Type a query that only matches inside the collapsed subtree. ---
	test.Type(search, "tags")

	if _, total, _ := pv.SearchStatus(); total == 0 {
		t.Fatal("typing \"tags\" into the search box found no matches")
	}
	if pv.doc.Collapsed(info) {
		t.Error("search did not auto-reveal the match (\"info\" is still collapsed)")
	}

	// --- 3. Tick the Wrap checkbox; the viewer switches to soft-wrap. ---
	test.Tap(wrap)

	if !pv.doc.WrapActive() {
		t.Error("ticking the Wrap checkbox did not enable soft-wrap")
	}
}
