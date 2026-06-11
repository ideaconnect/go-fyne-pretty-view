package prettyview

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// newEditPV builds a focused, rendered editable widget with the given InputConfig.
func newEditPV(t *testing.T, cfg InputConfig) (*PrettyView, fyne.Window) {
	t.Helper()
	test.NewApp()
	pv := New(WithEditable(), WithInputConfig(cfg))
	win := test.NewWindow(pv)
	win.Resize(fyne.NewSize(600, 400))
	pv.Refresh()
	pv.FocusGained()
	return pv, win
}

func typeStr(pv *PrettyView, s string) {
	for _, r := range s {
		pv.TypedRune(r)
	}
}

func TestFormatOnPausePrettyPrints(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1,"b":2}`)
	if pv.Format() != FormatRaw {
		t.Fatalf("while typing the doc should be the raw projection, got %v", pv.Format())
	}

	pv.Reformat() // the explicit form of the on-pause action
	if pv.Format() != FormatJSON {
		t.Errorf("after Reformat doc format = %v, want JSON (structured pretty)", pv.Format())
	}
	if pv.doc.TotalLines() < 3 {
		t.Errorf("structured JSON should pretty-print to multiple lines, got %d", pv.doc.TotalLines())
	}
	if !pv.editStructured {
		t.Error("Reformat should switch to the structured projection")
	}

	// Editing after a reformat reverts to raw and applies the edit at the right place.
	pv.FocusGained()
	typeStr(pv, "x")
	if pv.editStructured {
		t.Error("typing after a reformat must drop back to the raw projection")
	}
}

func TestEditDebounceCoalescesToOneReparse(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{DebounceFor: time.Second, AutoFormat: AutoFormatOnPause})
	defer win.Close()

	var settles int
	pv.SetOnChanged(func(string) { settles++ })

	typeStr(pv, `{"a":1}`) // a burst: each keystroke restarts the one-second timer
	if pv.editTimer == nil {
		t.Fatal("a burst of edits should leave exactly one armed timer")
	}
	if settles != 0 {
		t.Fatalf("no settle should have fired during the burst, got %d", settles)
	}

	pv.editSettled() // simulate the single timer fire
	if settles != 1 {
		t.Errorf("a settled burst should fire onChanged exactly once, got %d", settles)
	}
	if pv.Format() != FormatJSON {
		t.Errorf("the single settle should reparse once to JSON, got %v", pv.Format())
	}

	// Idempotent guard: a second settle on unchanged bytes does not re-fire a reparse
	// (it returns early), though onChanged still reports the settled text.
	pv.editSettled()
	if settles != 2 {
		t.Errorf("onChanged fires per settle, got %d", settles)
	}
}

func TestInvalidMidEditDegradesToRaw(t *testing.T) {
	// Non-structured input keeps the raw projection (and never panics on reformat).
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()
	typeStr(pv, "plain text here {[")
	pv.Reformat() // must not panic
	if pv.editStructured {
		t.Error("non-structured input must not enter the structured projection")
	}
	if pv.Format() != FormatRaw {
		t.Errorf("non-structured input should render raw, got %v", pv.Format())
	}

	// Partial JSON mid-type must not panic (the tolerant parser renders it either way),
	// and finishing the value reformats cleanly.
	pv2, win2 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win2.Close()
	typeStr(pv2, `{"a":`)
	pv2.Reformat() // must not panic
	pv2.FocusGained()
	typeStr(pv2, `1}`)
	if got := string(pv2.buf.Bytes()); got != `{"a":1}` {
		t.Fatalf("buffer after completing = %q, want %q", got, `{"a":1}`)
	}
	pv2.Reformat()
	if pv2.Format() != FormatJSON {
		t.Errorf("completed JSON should reformat, got %v", pv2.Format())
	}
}

func TestReparseAfterDestroyIsDropped(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{DebounceFor: time.Second, AutoFormat: AutoFormatOnPause})
	defer win.Close()

	typeStr(pv, "{") // arms the reformat timer
	if pv.editTimer == nil {
		t.Fatal("an edit should arm a reformat timer")
	}
	pv.r.Destroy()
	if !pv.destroyed.Load() {
		t.Error("Destroy must set the destroyed guard")
	}
	if pv.editTimer != nil {
		t.Error("Destroy must stop the pending reformat timer")
	}
}

func TestFormatManualVsOnBlur(t *testing.T) {
	// AutoFormatOff never auto-reformats, but Reformat() does.
	off, w1 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer w1.Close()
	typeStr(off, `{"a":1}`)
	off.editSettled() // a settle is a no-op for the reformat in Off mode
	if off.editStructured {
		t.Error("AutoFormatOff must not auto-reformat")
	}
	off.Reformat()
	if !off.editStructured || off.Format() != FormatJSON {
		t.Error("Reformat() must reformat even in Off mode")
	}

	// AutoFormatOnBlur reformats on FocusLost, not while focused.
	blur, w2 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOnBlur})
	defer w2.Close()
	typeStr(blur, `{"a":1}`)
	if blur.editStructured {
		t.Error("OnBlur must not reformat while focused")
	}
	blur.FocusLost()
	if !blur.editStructured || blur.Format() != FormatJSON {
		t.Error("OnBlur must reformat on FocusLost")
	}
}
