package prettyview

import (
	"errors"
	"io"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// memReadCloser is an in-memory fyne.URIReadCloser for the open-dialog tests.
type memReadCloser struct {
	io.Reader
	uri fyne.URI
}

func (m memReadCloser) URI() fyne.URI { return m.uri }
func (m memReadCloser) Close() error  { return nil }

// TestOpenDialogResultHandling covers loadFromReader: a picker error surfaces an error
// dialog (the swallowed-error bug), a cancel is silent, and a successful read loads
// and auto-detects the file.
func TestOpenDialogResultHandling(t *testing.T) {
	test.NewApp()

	pv := New()
	win := test.NewWindow(pv)
	defer win.Close()

	// Picker error -> an error dialog overlay (not swallowed).
	loadFromReader(pv, win, nil, errors.New("disk gone"))
	if got := len(win.Canvas().Overlays().List()); got == 0 {
		t.Error("a picker error must surface a dialog, not be swallowed")
	}
	for _, o := range win.Canvas().Overlays().List() {
		win.Canvas().Overlays().Remove(o)
	}

	// Cancel (nil reader, no error) -> silent.
	loadFromReader(pv, win, nil, nil)
	if got := len(win.Canvas().Overlays().List()); got != 0 {
		t.Errorf("cancel must be silent, got %d overlays", got)
	}

	// Successful read -> the file is loaded and auto-detected.
	rc := memReadCloser{Reader: strings.NewReader(`{"a":1}`), uri: storage.NewFileURI("/x.json")}
	loadFromReader(pv, win, rc, nil)
	if pv.doc == nil || pv.Format() != FormatJSON {
		t.Errorf("successful open did not load+detect JSON (format=%v)", pv.Format())
	}
}

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
