package prettyview

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

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
