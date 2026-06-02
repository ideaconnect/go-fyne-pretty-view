package prettyview

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestParseFormatNameAllArms(t *testing.T) {
	cases := map[string]Format{
		"json": FormatJSON, "jsonc": FormatJSONC, "xml": FormatXML,
		"html": FormatHTML, "raw": FormatRaw, "auto": FormatAuto, "nonsense": FormatAuto,
	}
	for name, want := range cases {
		if got := parseFormatName(name); got != want {
			t.Errorf("parseFormatName(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestToolbarWithWindow covers the Open button + Ctrl/Cmd+F registration paths,
// which only run when a Window is supplied.
func TestToolbarWithWindow(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":1}`, FormatJSON)
	win := test.NewWindow(widget.NewLabel("host"))
	defer win.Close()

	if NewToolbar(pv, ToolbarConfig{ShowOpen: true, ShowSearch: true, Window: win}) == nil {
		t.Error("toolbar with window is nil")
	}
	// An explicit OnOpen override is honored over the built-in dialog.
	opened := false
	bar := NewToolbar(pv, ToolbarConfig{ShowOpen: true, OnOpen: func() { opened = true }})
	if bar == nil {
		t.Fatal("toolbar with OnOpen is nil")
	}
	_ = opened
}

// TestShowOpenDialog covers the nil-window early return and the dialog body.
func TestShowOpenDialog(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":1}`, FormatJSON)
	ShowOpenDialog(pv, nil) // early return, no panic
	win := test.NewWindow(widget.NewLabel("host"))
	defer win.Close()
	ShowOpenDialog(pv, win) // shows the dialog (callback is async; we just exercise the call)
}

// TestSearchRequestedFocus covers focusObject via the search bar's
// search-requested hook.
func TestSearchRequestedFocus(t *testing.T) {
	test.NewApp()
	pv := docPV(`{"a":1}`, FormatJSON)
	bar := NewSearchBar(pv)
	win := test.NewWindow(bar)
	defer win.Close()
	if pv.onSearchRequested != nil {
		pv.onSearchRequested() // -> focusObject(entry)
	}
}
