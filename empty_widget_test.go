package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestEmptyWidgetGetters pins the before-shown read API: a constructed-but-never-shown
// widget (renderer nil) answers ScrollOffset/SetScrollOffset safely, reports empty
// Source/Text, and survives a Reparse without a renderer.
func TestEmptyWidgetGetters(t *testing.T) {
	test.NewApp()
	pv := New() // empty document, no renderer yet

	if len(pv.Source()) != 0 {
		t.Errorf("Source() before load = %q, want empty", pv.Source())
	}
	if pv.Text() != "" {
		t.Errorf("Text() before load = %q, want empty", pv.Text())
	}
	if pv.ScrollOffset() != (fyne.Position{}) {
		t.Errorf("ScrollOffset() before show = %v, want zero (renderer-nil guard)", pv.ScrollOffset())
	}
	pv.SetScrollOffset(fyne.NewPos(10, 10)) // no renderer: a safe no-op
	if pv.ScrollOffset() != (fyne.Position{}) {
		t.Error("SetScrollOffset before show should be a no-op")
	}

	// Reparse on the empty document re-reads its (empty) source under the new format,
	// without a renderer, and must not panic. Empty input falls back to raw.
	pv.Reparse(FormatJSON)
	if pv.Format() != FormatRaw {
		t.Errorf("Reparse(FormatJSON) on empty input -> Format() = %v, want raw (empty falls back)", pv.Format())
	}
}
