package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// wrapCallbackSource is a document with one long line, so a wrap flip actually
// re-projects rather than short-circuiting on a document that cannot wrap.
func wrapCallbackSource() []byte {
	return []byte(`{"k":"` + strings.Repeat("wxyz ", 40) + `"}`)
}

// TestSetOnWrapChangedFiresBothWays checks the host hook observes every real
// mode change, with the new mode as its argument.
func TestSetOnWrapChangedFiresBothWays(t *testing.T) {
	pv, win := renderInWindow(t, wrapCallbackSource(), FormatJSON, 300, 400)
	defer win.Close()

	var got []WrapMode
	pv.SetOnWrapChanged(func(m WrapMode) { got = append(got, m) })

	pv.SetWrap(WrapWord)
	pv.SetWrap(WrapNone)

	if len(got) != 2 || got[0] != WrapWord || got[1] != WrapNone {
		t.Fatalf("callback saw %v, want [WrapWord WrapNone]", got)
	}
	// The hook must observe the mode the widget actually settled on, not a
	// value the host has to re-read.
	if pv.Wrap() != got[len(got)-1] {
		t.Errorf("Wrap() = %v but the last callback said %v", pv.Wrap(), got[len(got)-1])
	}
}

// TestSetOnWrapChangedSkipsRedundantSet checks a SetWrap to the already-current
// mode is a no-op: no callback, and no reprojection either.
func TestSetOnWrapChangedSkipsRedundantSet(t *testing.T) {
	pv, win := renderInWindow(t, wrapCallbackSource(), FormatJSON, 300, 400)
	defer win.Close()

	fired := 0
	pv.SetOnWrapChanged(func(WrapMode) { fired++ })

	pv.SetWrap(WrapNone) // already the default
	if fired != 0 {
		t.Errorf("redundant SetWrap(WrapNone) fired the callback %d time(s)", fired)
	}
	pv.SetWrap(WrapWord)
	wrappedRows := pv.doc.TotalVisibleRows()
	pv.SetWrap(WrapWord) // redundant again, now from the non-default side
	if fired != 1 {
		t.Errorf("callback fired %d time(s), want 1 (only the real flip)", fired)
	}
	if got := pv.doc.TotalVisibleRows(); got != wrappedRows {
		t.Errorf("redundant SetWrap re-projected: rows %d != %d", got, wrappedRows)
	}
}

// TestSetOnWrapChangedNotFiredByWithWrap checks the construction-time option is
// not reported as a change — the hook exists to observe user toggles, and a host
// that seeds the mode from persisted state must not be told it changed.
func TestSetOnWrapChangedNotFiredByWithWrap(t *testing.T) {
	test.NewApp()
	pv := NewWithData(wrapCallbackSource(), FormatJSON, WithWrap(WrapWord))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 400))
	pv.Refresh()

	fired := 0
	pv.SetOnWrapChanged(func(WrapMode) { fired++ })
	pv.Refresh() // a reflow reconciles the projection; that is not a mode change

	if fired != 0 {
		t.Errorf("callback fired %d time(s) for the WithWrap seed", fired)
	}
	if pv.Wrap() != WrapWord {
		t.Errorf("Wrap() = %v, want WrapWord from WithWrap", pv.Wrap())
	}
	// Seeded to WrapWord, the *first* observable change is the flip back off.
	pv.SetWrap(WrapNone)
	if fired != 1 {
		t.Errorf("callback fired %d time(s) after the flip off, want 1", fired)
	}
}

// TestWrapToggleControlFiresOnWrapChanged checks the bundled toolbar toggle
// routes through SetWrap, so a host that persists the preference sees taps too.
func TestWrapToggleControlFiresOnWrapChanged(t *testing.T) {
	test.NewApp()
	pv := NewWithData(wrapCallbackSource(), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 400))
	pv.Refresh()

	var got []WrapMode
	pv.SetOnWrapChanged(func(m WrapMode) { got = append(got, m) })

	btn, ok := NewWrapToggle(pv).(fyne.Tappable)
	if !ok {
		t.Fatal("NewWrapToggle is not tappable")
	}
	test.Tap(btn)
	test.Tap(btn)

	if len(got) != 2 || got[0] != WrapWord || got[1] != WrapNone {
		t.Fatalf("toolbar taps reported %v, want [WrapWord WrapNone]", got)
	}
}

// TestSetOnWrapChangedReplacesPrevious checks registering a second hook replaces
// the first (documented single-callback semantics), and that a nil clears it.
func TestSetOnWrapChangedReplacesPrevious(t *testing.T) {
	pv, win := renderInWindow(t, wrapCallbackSource(), FormatJSON, 300, 400)
	defer win.Close()

	first, second := 0, 0
	pv.SetOnWrapChanged(func(WrapMode) { first++ })
	pv.SetOnWrapChanged(func(WrapMode) { second++ })
	pv.SetWrap(WrapWord)
	if first != 0 || second != 1 {
		t.Errorf("first=%d second=%d, want 0/1 (the later hook replaces the earlier)", first, second)
	}

	pv.SetOnWrapChanged(nil)
	pv.SetWrap(WrapNone) // must not panic on the cleared hook
	if second != 1 {
		t.Errorf("second fired %d time(s) after SetOnWrapChanged(nil), want 1", second)
	}
}
