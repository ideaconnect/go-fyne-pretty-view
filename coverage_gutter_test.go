package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestErrorLineGutterTint covers build's error-marker gutter branch: with line numbers on,
// an invalid edit tints the error line's gutter number with the theme error color.
func TestErrorLineGutterTint(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable(), WithLineNumbers(),
		WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	typeStr(pv, "[1 bogus]")
	pv.Reformat() // invalid -> a KindError marker on the buffer line
	st := pv.ParseStatus()
	if st.OK || st.ErrorLine < 0 {
		t.Fatalf("expected an invalid status with a line, got %+v", st)
	}
	if pv.errorColor == nil {
		t.Skip("active theme defines no error color")
	}
	tinted := false
	for _, rw := range pv.r.live {
		if rw.line == int32(st.ErrorLine) && rw.rr != nil && rw.rr.gutter != nil &&
			rw.rr.gutter.Visible() && rw.rr.gutter.Color == pv.errorColor {
			tinted = true
		}
	}
	if !tinted {
		t.Error("the error line's gutter number should be tinted with the error color")
	}
}

// TestWrappedSelectionRenders drives rebuildSelection across the sub-rows of a soft-wrapped
// line: selecting all of a wrapped document paints a rect per visible sub-row.
func TestWrappedSelectionRenders(t *testing.T) {
	test.NewApp()
	long := strings.Repeat("delta echo ", 50)
	pv := New(WithWrap(WrapWord))
	pv.SetData([]byte(`["`+long+`"]`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(260, 220))
	pv.Refresh()

	pv.SelectAll()
	pv.Refresh()
	if !pv.sel.active {
		t.Error("select-all should leave an active selection over the wrapped doc")
	}
	if pv.SelectedText() == "" {
		t.Error("select-all of a wrapped doc should select text")
	}
}
