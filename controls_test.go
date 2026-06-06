package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// findEntry returns the first *widget.Entry in o's object tree (the search bar
// has exactly one), so a test can drive its OnSubmitted directly.
func findEntry(o fyne.CanvasObject) *widget.Entry {
	switch w := o.(type) {
	case *widget.Entry:
		return w
	case *fyne.Container:
		for _, c := range w.Objects {
			if e := findEntry(c); e != nil {
				return e
			}
		}
	}
	return nil
}

// TestSearchBarEnterStopsOnFirstMatch guards the fresh-query Enter behavior: when
// Enter beats the debounce on a brand-new query, Search reveals match #1 and the
// bar must NOT immediately advance past it to #2. Pressing Enter again on the same
// (now-applied) query is find-next and does advance.
func TestSearchBarEnterStopsOnFirstMatch(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":"x","b":"x","c":"x"}`, FormatJSON)
	entry := findEntry(NewSearchBar(pv))
	if entry == nil {
		t.Fatal("no entry found in search bar")
	}

	entry.Text = "x" // fresh query; OnSubmitted fires before the debounce ran
	entry.OnSubmitted(entry.Text)
	if a, total, _ := pv.SearchStatus(); total != 3 || a != 1 {
		t.Fatalf("fresh-query Enter: active=%d total=%d, want active 1 of 3 (must not skip #1)", a, total)
	}

	entry.OnSubmitted(entry.Text) // same query -> find-next
	if a, _, _ := pv.SearchStatus(); a != 2 {
		t.Errorf("repeat Enter: active=%d, want 2 (find-next)", a)
	}
}

func TestReparse(t *testing.T) {
	pv := docPV(`{"a":1}`, FormatJSON)
	if pv.Format() != FormatJSON {
		t.Fatalf("initial format = %v", pv.Format())
	}
	pv.Reparse(FormatRaw)
	if pv.Format() != FormatRaw {
		t.Errorf("after Reparse(raw) format = %v, want raw", pv.Format())
	}
	if string(pv.Source()) != `{"a":1}` {
		t.Errorf("Source() = %q, want original bytes", pv.Source())
	}
}

func TestDataChangedHook(t *testing.T) {
	pv := New()
	fired := 0
	pv.SetOnDataChanged(func() { fired++ })
	pv.SetData([]byte(`{"a":1}`), FormatJSON)
	pv.Reparse(FormatRaw)
	if fired != 2 {
		t.Errorf("onDataChanged fired %d times, want 2", fired)
	}
}

func TestSearchChangedHook(t *testing.T) {
	pv := docPV(`{"a":"x","b":"x"}`, FormatJSON)
	fired := 0
	pv.SetOnSearchChanged(func() { fired++ })
	pv.Search(SearchQuery{Text: "x"})
	pv.SearchNext()
	pv.ClearSearch()
	if fired < 3 {
		t.Errorf("onSearchChanged fired %d times, want >= 3", fired)
	}
}

func TestFormatSelectDrivesReparse(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":1}`, FormatJSON)
	sel, ok := NewFormatSelect(pv).(*widget.Select)
	if !ok {
		t.Fatal("NewFormatSelect did not return a *widget.Select")
	}
	if sel.Selected != "json" {
		t.Errorf("initial selection = %q, want json", sel.Selected)
	}
	sel.SetSelected("raw")
	if pv.Format() != FormatRaw {
		t.Errorf("format selector did not reparse: format = %v", pv.Format())
	}
}

func TestToolbarConfigOmitsControls(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":1}`, FormatJSON)

	// Every control off, no window: a valid (empty) object, no panic.
	if NewToolbar(pv, ToolbarConfig{}) == nil {
		t.Error("empty toolbar is nil")
	}
	// Search only.
	if NewToolbar(pv, ToolbarConfig{ShowSearch: true}) == nil {
		t.Error("search-only toolbar is nil")
	}
	// Everything (no window, so Open is omitted but the rest build).
	if NewToolbar(pv, DefaultToolbarConfig()) == nil {
		t.Error("default toolbar is nil")
	}
	// The à-la-carte pieces build too.
	if NewFoldButtons(pv) == nil || NewSearchBar(pv) == nil {
		t.Error("individual control constructors returned nil")
	}
}
