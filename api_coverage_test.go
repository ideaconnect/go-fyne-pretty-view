package prettyview

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestPublicAPISmoke exercises the thin public mutators/accessors that wrap
// well-tested model logic but were not themselves invoked by other tests.
func TestPublicAPISmoke(t *testing.T) {
	test.NewApp()
	pv := New()

	// SetText loads content (shorthand for SetData with auto-detect).
	pv.SetText(`{"a":{"b":{"c":1}},"d":2}`)
	if pv.Format() != FormatJSON {
		t.Fatalf("SetText auto-detect = %v, want JSON", pv.Format())
	}
	if got := string(pv.Source()); got != `{"a":{"b":{"c":1}},"d":2}` {
		t.Errorf("Source round-trip = %q", got)
	}

	full := pv.doc.TotalVisibleRows()

	// CollapseAll then ExpandAll must round-trip the visible-row count.
	pv.CollapseAll()
	if pv.doc.TotalVisibleRows() >= full {
		t.Error("CollapseAll hid no rows")
	}
	pv.ExpandAll()
	if pv.doc.TotalVisibleRows() != full {
		t.Errorf("ExpandAll did not restore rows: %d != %d", pv.doc.TotalVisibleRows(), full)
	}

	// SetDefaultCollapseDepth affects the NEXT load.
	pv.SetDefaultCollapseDepth(1)
	pv.SetText(`{"a":{"b":{"c":1}},"d":2}`)
	if pv.doc.TotalVisibleRows() >= full {
		t.Error("SetDefaultCollapseDepth(1) did not collapse on the next load")
	}
	pv.SetDefaultCollapseDepth(-3) // clamps to 0
	if pv.cfg.collapseDepth != 0 {
		t.Errorf("SetDefaultCollapseDepth(-3) = %d, want 0", pv.cfg.collapseDepth)
	}

	// Reparse under a forced format reuses the current source.
	pv.SetText("plain text\nlines")
	pv.Reparse(FormatRaw)
	if pv.Format() != FormatRaw {
		t.Errorf("Reparse(FormatRaw) = %v", pv.Format())
	}

	// Wrap accessor + the host-sync hook setters (just record the callbacks).
	if pv.Wrap() != WrapNone {
		t.Errorf("default Wrap = %v", pv.Wrap())
	}
	var dataHook, searchHook, reqHook bool
	pv.SetOnDataChanged(func() { dataHook = true })
	pv.SetOnSearchChanged(func() { searchHook = true })
	pv.SetOnSearchRequested(func() { reqHook = true })
	pv.SetText(`{"x":1}`)             // fires onDataChanged
	pv.Search(SearchQuery{Text: "x"}) // fires onSearchChanged
	if pv.onSearchRequested != nil {
		pv.onSearchRequested() // fires onSearchRequested
	}
	if !dataHook || !searchHook || !reqHook {
		t.Errorf("host hooks not all invoked: data=%v search=%v req=%v", dataHook, searchHook, reqHook)
	}
}

// TestEmptyDocAPISafety checks the public API is safe on a freshly constructed
// (empty) viewer — the nil/empty guards in the accessors.
func TestEmptyDocAPISafety(t *testing.T) {
	test.NewApp()
	pv := New()
	pv.ExpandAll()
	pv.CollapseAll()
	pv.SelectAll()
	pv.ClearSelection()
	if got := pv.SelectedText(); got != "" {
		t.Errorf("empty SelectedText = %q", got)
	}
	pv.ExpandTo(0)
	pv.CopySubtree(0)
}
