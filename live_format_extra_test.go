package prettyview

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestSetInputConfigMerges(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	pv.SetInputConfig(InputConfig{AutoFormat: AutoFormatOnPause, DebounceFor: 250 * time.Millisecond})
	if pv.cfg.input.AutoFormat != AutoFormatOnPause {
		t.Errorf("SetInputConfig did not merge AutoFormat, got %v", pv.cfg.input.AutoFormat)
	}
	if pv.cfg.input.DebounceFor != 250*time.Millisecond {
		t.Errorf("SetInputConfig did not merge DebounceFor, got %v", pv.cfg.input.DebounceFor)
	}
}

func TestReformatNoOpReadOnly(t *testing.T) {
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	before := string(pv.Source())
	pv.Reformat() // no-op for a read-only widget
	if string(pv.Source()) != before {
		t.Error("Reformat must not change a read-only widget")
	}
}

func TestReformatFiresOnChanged(t *testing.T) {
	// A long debounce keeps the typing-armed settle from firing during the test.
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: time.Hour})
	defer win.Close()

	var got string
	var n int
	pv.SetOnChanged(func(s string) { got = s; n++ })
	typeStr(pv, `{"a":1}`)
	n = 0 // ignore the (suppressed) typing settle
	pv.Reformat()
	if n == 0 {
		t.Error("Reformat should fire onChanged")
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("onChanged after Reformat got %q, want the pretty form", got)
	}
}

func TestSettleRefreshesValidity(t *testing.T) {
	// In the default Off mode a settle refreshes parse validity (no reflow) and fires the
	// validation callback. A long debounce keeps the armed timer from firing concurrently.
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: time.Hour})
	defer win.Close()

	var oks []bool
	pv.SetOnValidationChanged(func(st ParseStatus) { oks = append(oks, st.OK) })
	typeStr(pv, `[1 bad]`)
	pv.editSettled() // Off mode: refreshParseStatus only, no reflow
	if strings.Contains(string(pv.buf.Bytes()), "\n") {
		t.Error("a settle in Off mode must not reflow the buffer")
	}
	if pv.ParseStatus().OK {
		t.Error("settle should have refreshed validity to invalid")
	}
	if len(oks) == 0 || oks[len(oks)-1] {
		t.Errorf("validation callback should report invalid, got %v", oks)
	}
}

func TestOnPauseSettleReformats(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOnPause, DebounceFor: time.Hour})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	pv.editSettled() // OnPause: reflow on settle
	if !strings.Contains(string(pv.buf.Bytes()), "\n") {
		t.Errorf("OnPause settle should reflow the buffer, got %q", pv.buf.Bytes())
	}
}

func TestResolveFormatPinned(t *testing.T) {
	// A widget pinned to a format colors with it (resolveFormat's non-Auto branch).
	test.NewApp()
	pv := New(WithEditable(), WithFormat(FormatJSON))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	typeStr(pv, `{"a":1}`)
	if pv.Format() != FormatJSON {
		t.Errorf("pinned format = %v, want JSON", pv.Format())
	}
}

func TestEditModeHonorsExplicitFormat(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	json := []byte(`{"a":1}`)
	pv.SetData(json, FormatRaw) // explicitly raw: a plain-text editor, no JSON coloring
	if pv.Format() != FormatRaw {
		t.Errorf("SetData(_, FormatRaw) in edit mode: Format() = %v, want FormatRaw", pv.Format())
	}
	pv.SetData(json, FormatJSON)
	if pv.Format() != FormatJSON {
		t.Errorf("SetData(_, FormatJSON) in edit mode: Format() = %v, want FormatJSON", pv.Format())
	}
	pv.SetData(json, FormatAuto) // auto -> detect JSON
	if pv.Format() != FormatJSON {
		t.Errorf("SetData(_, FormatAuto) on JSON: Format() = %v, want FormatJSON", pv.Format())
	}
}

func TestReparseEditModeFormat(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	if pv.Format() != FormatJSON {
		t.Fatalf("typed JSON Format = %v, want JSON", pv.Format())
	}
	pv.Reparse(FormatRaw) // force raw: re-reads the live buffer under the new format
	if pv.Format() != FormatRaw {
		t.Errorf("Reparse(FormatRaw) in edit mode: Format() = %v, want FormatRaw", pv.Format())
	}
	if got := string(pv.Source()); got != `{"a":1}` {
		t.Errorf("Reparse must preserve the buffer bytes, got %q", got)
	}
}

func TestSetDataCancelsPendingSettle(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: time.Hour})
	defer win.Close()

	pv.SetOnChanged(func(string) {}) // makes a typing settle observable, so it arms a timer
	typeStr(pv, "a")
	if pv.editTimer == nil {
		t.Fatal("typing with an onChanged listener should arm a settle timer")
	}
	gen := pv.editGen
	pv.SetData([]byte("new"), FormatAuto)
	if pv.editTimer != nil {
		t.Error("SetData must cancel a pending settle timer")
	}
	if pv.editGen <= gen {
		t.Error("SetData must bump editGen to invalidate an already-queued settle")
	}
}

// TestEditLineErrorGutter exercises lineIsError's edit-mode branch: an invalid line is
// flagged by buffer (display) line, and a valid buffer flags nothing.
func TestEditLineErrorGutter(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, "[1 bad]")
	pv.Reformat() // invalid -> not rewritten, status carries a buffer-line error
	st := pv.ParseStatus()
	if st.OK || st.ErrorLine < 0 {
		t.Fatalf("expected an invalid status with a line, got %+v", st)
	}
	if !pv.lineIsError(int32(st.ErrorLine)) {
		t.Errorf("lineIsError should flag the reported error line %d", st.ErrorLine)
	}
	if pv.lineIsError(int32(st.ErrorLine) + 100) {
		t.Error("lineIsError should not flag an unrelated line")
	}
}
