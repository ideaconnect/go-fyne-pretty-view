package prettyview

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// TestSearchBarCappedAndToggleOff covers three NewSearchBar branches the existing tests do
// not: the capped match-counter ("n/m+"), turning a toggle back off (toggleImportance ->
// Low), and Shift+Enter on a CHANGED query (onPrev re-applies, then steps back).
func TestSearchBarCappedAndToggleOff(t *testing.T) {
	test.NewApp()
	pv := New(WithSearchConfig(SearchConfig{MaxMatches: 1}))
	pv.SetData([]byte(`{"a":"l l l l l"}`), FormatJSON)
	bar := NewSearchBar(pv)
	entry := findEntry(bar)
	if entry == nil {
		t.Fatal("search bar has no entry")
	}

	// A MaxMatches=1 search over many hits caps -> the counter's capped branch ("+").
	entry.Text = "l"
	entry.OnSubmitted("l")
	if _, _, capped := pv.SearchStatus(); !capped {
		t.Fatal("expected a capped search")
	}

	// Toggling the case button on then off exercises toggleImportance(false).
	caseBtn := findButtonByText(bar, "Aa")
	if caseBtn == nil {
		t.Fatal("search bar has no case toggle")
	}
	caseBtn.OnTapped() // on
	caseBtn.OnTapped() // off -> toggleImportance(false)

	// Shift+Enter with a query different from the applied one re-applies it, then steps back.
	se := findSearchEntry(bar)
	entry.Text = "a" // differs from the applied "l", without going through OnChanged
	se.onPrev()      // query changed -> pv.Search(new) + pv.SearchPrev()
}

// TestFocusObjectNonFocusable covers focusObject's not-Focusable early return.
func TestFocusObjectNonFocusable(t *testing.T) {
	test.NewApp()
	focusObject(canvas.NewRectangle(color.Black)) // a rectangle is not fyne.Focusable -> no-op
}
