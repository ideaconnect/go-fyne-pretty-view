package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestSearchVariantsAndReveal covers runSearch's case-insensitive, regex-with-literal-
// prefix, and capped paths, plus step/revealActive navigation to an active match.
func TestSearchVariantsAndReveal(t *testing.T) {
	src := `{"a":"Hello hello HELLO world","b":"hello"}`
	pv, win := renderInWindow(t, []byte(src), FormatJSON, 400, 300)
	defer win.Close()

	// Case-insensitive plain search exercises the lowercase-needle path.
	pv.Search(SearchQuery{Text: "hello", CaseSensitive: false})
	if _, total, _ := pv.SearchStatus(); total < 3 {
		t.Errorf("case-insensitive 'hello' total = %d, want >= 3", total)
	}
	pv.SearchNext() // step forward -> revealActive centers on a match
	pv.SearchPrev()

	// A regex with a literal head ("Hel") drives the LiteralPrefix prefilter.
	pv.Search(SearchQuery{Text: "Hel+o", Mode: SearchRegex, CaseSensitive: true})
	if _, total, _ := pv.SearchStatus(); total < 1 {
		t.Errorf("regex 'Hel+o' total = %d, want >= 1", total)
	}
	// A regex with no literal head still runs (prefilter disabled).
	pv.Search(SearchQuery{Text: ".ello", Mode: SearchRegex})
	if pv.SearchError() != nil {
		t.Errorf("valid regex set an error: %v", pv.SearchError())
	}
}

// TestSearchCapsMatches covers the MaxMatches cap path (scanPlain sets capped=true).
func TestSearchCapsMatches(t *testing.T) {
	test.NewApp()
	pv := New(WithSearchConfig(SearchConfig{MaxMatches: 1}))
	pv.SetData([]byte(`{"a":"l l l l l"}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()

	pv.Search(SearchQuery{Text: "l"})
	if _, _, capped := pv.SearchStatus(); !capped {
		t.Error("a MaxMatches=1 search over many hits should report capped")
	}
}

// TestMultiLineSelectionRenders drives rebuildSelection across several display lines: a
// select-all of a multi-line document paints a selection rect per visible line.
func TestMultiLineSelectionRenders(t *testing.T) {
	pv, win := renderInWindow(t, []byte("{\n  \"a\": 1,\n  \"b\": 2\n}"), FormatJSON, 400, 300)
	defer win.Close()

	pv.SelectAll() // spans every line -> per-line selection rectangles
	pv.Refresh()
	if pv.SelectedText() == "" {
		t.Error("select-all of a multi-line doc should select text")
	}
	if !pv.sel.active {
		t.Error("select-all should leave an active selection")
	}
}

// TestLineNumbersWithWrapRender renders a long line under WrapWord with the line-number
// gutter on, so the first visual row of a line shows its number (layoutLineNumber) and
// each wrap-continuation row blanks the gutter (lineNumHide).
func TestLineNumbersWithWrapRender(t *testing.T) {
	test.NewApp()
	long := strings.Repeat("alpha bravo ", 60) // wraps to several visual rows
	pv := New(WithLineNumbers(), WithWrap(WrapWord))
	pv.SetData([]byte(`["`+long+`"]`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(260, 220))
	pv.Refresh()

	if pv.met.GutterWidth() <= 0 {
		t.Fatal("WithLineNumbers should widen the gutter")
	}
	if pv.doc.TotalVisibleRows() < 3 {
		t.Fatalf("a long line under WrapWord should make several rows, got %d", pv.doc.TotalVisibleRows())
	}

	// At least one live row renders a visible gutter line number (layoutLineNumber on the
	// first visual row of a line, under wrap).
	shown := false
	for _, rw := range pv.r.live {
		if rw.rr != nil && rw.rr.gutter != nil && rw.rr.gutter.Visible() && rw.rr.gutter.Text != "" {
			shown = true
			break
		}
	}
	if !shown {
		t.Error("expected at least one row to render a gutter line number under wrap")
	}
}
