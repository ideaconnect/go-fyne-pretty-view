package prettyview

import "testing"

func TestParseStatusReportsErrorLine(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `[1 bogus]`) // recoverable-but-invalid JSON
	pv.Reformat()
	st := pv.ParseStatus()
	if st.OK {
		t.Fatal("invalid JSON should report ParseStatus().OK == false")
	}
	if st.ErrorLine < 0 || st.ErrorLine >= pv.doc.TotalLines() {
		t.Errorf("ErrorLine = %d, want a valid display line", st.ErrorLine)
	}
	if !pv.lineIsError(int32(st.ErrorLine)) {
		t.Errorf("the reported ErrorLine %d is not a KindError marker line", st.ErrorLine)
	}

	// Valid JSON reports OK.
	pv2, win2 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win2.Close()
	typeStr(pv2, `[1,2,3]`)
	pv2.Reformat()
	if got := pv2.ParseStatus(); !got.OK || got.ErrorLine != -1 {
		t.Errorf("valid JSON ParseStatus = %+v, want {OK:true ErrorLine:-1}", got)
	}
}

// TestReadOnlyParseStatus is the #87 regression: a read-only viewer (no WithEditable) must
// surface tolerant-parse validity via ParseStatus()/SetOnValidationChanged, not just on
// screen. Previously SetData skipped setParseStatus for viewers, so a recovered-error load
// reported the {OK:true} construction default.
func TestReadOnlyParseStatus(t *testing.T) {
	pv := New() // read-only
	var fired []ParseStatus
	pv.SetOnValidationChanged(func(st ParseStatus) { fired = append(fired, st) })

	pv.SetData([]byte(`[1 bogus]`), FormatJSON) // tolerant parse leaves a KindError marker
	st := pv.ParseStatus()
	if st.OK {
		t.Fatal("read-only load of recoverable-but-invalid JSON should report OK == false")
	}
	if st.ErrorLine < 0 || st.ErrorLine >= pv.doc.TotalLines() || !pv.lineIsError(int32(st.ErrorLine)) {
		t.Errorf("ErrorLine = %d is not a KindError display line", st.ErrorLine)
	}
	if len(fired) != 1 || fired[0].OK {
		t.Errorf("SetOnValidationChanged should fire once with OK=false for a read-only load; got %+v", fired)
	}

	// A subsequent valid load flips back to OK and fires the transition.
	pv.SetData([]byte(`[1,2,3]`), FormatJSON)
	if got := pv.ParseStatus(); !got.OK || got.ErrorLine != -1 {
		t.Errorf("valid read-only load ParseStatus = %+v, want {OK:true ErrorLine:-1}", got)
	}
	if len(fired) != 2 || !fired[1].OK {
		t.Errorf("fixing the data should fire OK=true; transitions = %+v", fired)
	}
}

func TestValidationMarkerNoCaretMove(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `[1 bogus]`)
	pv.Reformat() // structured + a KindError marker line
	if pv.ParseStatus().OK {
		t.Fatal("precondition: invalid")
	}

	// Recording validity / rendering the marker must not move the caret.
	before := pv.sel.focus
	pv.setParseStatus(ParseStatus{OK: false, ErrorLine: pv.ParseStatus().ErrorLine})
	if pv.sel.focus != before {
		t.Error("validation feedback must not move the caret")
	}
	if !pv.lineIsError(int32(pv.ParseStatus().ErrorLine)) {
		t.Error("the error line should carry a marker")
	}
}

func TestValidationChangedFires(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	var oks []bool
	pv.SetOnValidationChanged(func(st ParseStatus) { oks = append(oks, st.OK) })

	typeStr(pv, `[1 bogus]`)
	pv.Reformat() // OK(init) -> false: fires false
	if len(oks) != 1 || oks[0] {
		t.Fatalf("after an invalid reformat, transitions = %v, want [false]", oks)
	}

	// Fix it (edit in the raw projection) and reformat -> false -> true: fires true.
	pv.FocusGained()
	typeStr(pv, "x") // reverts to raw
	pv.SelectAll()
	pv.editDelete(false) // clear the buffer
	typeStr(pv, `[1,2]`)
	pv.Reformat()
	if len(oks) != 2 || oks[1] != true {
		t.Errorf("fixing the JSON should fire true; transitions = %v, want [false true]", oks)
	}
}
