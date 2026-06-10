package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// findText returns the first *canvas.Text in o's tree (the search bar's match
// counter), for tests that assert what it displays.
func findText(o fyne.CanvasObject) *canvas.Text {
	switch w := o.(type) {
	case *canvas.Text:
		return w
	case *fyne.Container:
		for _, c := range w.Objects {
			if t := findText(c); t != nil {
				return t
			}
		}
	}
	return nil
}

// findEntry returns the first entry in o's object tree (the search bar has exactly
// one), so a test can drive its OnSubmitted directly.
func findEntry(o fyne.CanvasObject) *widget.Entry {
	switch w := o.(type) {
	case *searchEntry:
		return &w.Entry
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

// findSearchEntry returns the *searchEntry in o's tree, for tests that drive its
// Esc / Shift+Enter key handling directly.
func findSearchEntry(o fyne.CanvasObject) *searchEntry {
	switch w := o.(type) {
	case *searchEntry:
		return w
	case *fyne.Container:
		for _, c := range w.Objects {
			if e := findSearchEntry(c); e != nil {
				return e
			}
		}
	}
	return nil
}

// findButtonByText returns the first *widget.Button in o's tree with the given label.
func findButtonByText(o fyne.CanvasObject, text string) *widget.Button {
	switch w := o.(type) {
	case *widget.Button:
		if w.Text == text {
			return w
		}
	case *fyne.Container:
		for _, c := range w.Objects {
			if b := findButtonByText(c, text); b != nil {
				return b
			}
		}
	}
	return nil
}

// TestSearchBarCaseToggle: the "Aa" toggle makes the bar issue case-sensitive
// queries, changing the result set.
func TestSearchBarCaseToggle(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":"Alpha","b":"alpha"}`), FormatJSON)
	bar := NewSearchBar(pv)
	entry, caseBtn := findEntry(bar), findButtonByText(bar, "Aa")
	if entry == nil || caseBtn == nil {
		t.Fatal("search bar missing entry or case toggle")
	}
	entry.Text = "alpha"
	entry.OnSubmitted(entry.Text)
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf("case-insensitive total = %d, want 2 (Alpha, alpha)", total)
	}
	caseBtn.OnTapped() // case-sensitive on; re-runs the query
	if _, total, _ := pv.SearchStatus(); total != 1 {
		t.Errorf("case-sensitive total = %d, want 1 (alpha only)", total)
	}
}

// TestSearchBarRegexToggle: the ".*" toggle makes the bar issue regex queries.
func TestSearchBarRegexToggle(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":"x1","b":"x2","c":"yy"}`), FormatJSON)
	bar := NewSearchBar(pv)
	entry, regexBtn := findEntry(bar), findButtonByText(bar, ".*")
	if entry == nil || regexBtn == nil {
		t.Fatal("search bar missing entry or regex toggle")
	}
	entry.Text = `x.`
	entry.OnSubmitted(entry.Text)
	if _, total, _ := pv.SearchStatus(); total != 0 {
		t.Errorf("plain `x.` total = %d, want 0 (no literal 'x.' substring)", total)
	}
	regexBtn.OnTapped() // regex on; re-runs the query
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf("regex `x.` total = %d, want 2 (x1, x2)", total)
	}
}

// TestSearchBarSurfacesBadRegex: with the regex toggle on, an invalid pattern shows
// a "bad regex" indicator in the match counter (distinct from a 0/0 no-match).
func TestSearchBarSurfacesBadRegex(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":"x"}`), FormatJSON)
	bar := NewSearchBar(pv)
	entry, regexBtn, count := findEntry(bar), findButtonByText(bar, ".*"), findText(bar)
	if entry == nil || regexBtn == nil || count == nil {
		t.Fatal("search bar missing entry, regex toggle, or counter")
	}
	regexBtn.OnTapped() // regex mode
	entry.Text = "("    // invalid pattern
	entry.OnSubmitted(entry.Text)
	if pv.SearchError() == nil {
		t.Fatal("expected a SearchError for the invalid pattern")
	}
	if count.Text != "bad regex" {
		t.Errorf("counter = %q for an invalid regex, want \"bad regex\"", count.Text)
	}
}

// TestSearchBarShiftEnterAndEsc: Shift+Enter finds previous; Esc clears.
func TestSearchBarShiftEnterAndEsc(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":"x","b":"x","c":"x"}`), FormatJSON)
	bar := NewSearchBar(pv)
	se := findSearchEntry(bar)
	if se == nil {
		t.Fatal("search bar missing searchEntry")
	}
	se.Text = "x"
	se.OnSubmitted("x")
	if a, total, _ := pv.SearchStatus(); a != 1 || total != 3 {
		t.Fatalf("after Enter active=%d total=%d, want 1/3", a, total)
	}
	// Shift+Enter -> previous (wraps from #1 to #3).
	se.shiftHeld = true
	se.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if a, _, _ := pv.SearchStatus(); a != 3 {
		t.Errorf("after Shift+Enter active=%d, want 3 (prev wrap)", a)
	}
	// Esc clears the query and the entry text.
	se.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if _, total, _ := pv.SearchStatus(); total != 0 {
		t.Errorf("after Esc total=%d, want 0", total)
	}
	if se.Text != "" {
		t.Errorf("after Esc entry text = %q, want empty", se.Text)
	}
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
