package prettyview

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestToolbarOpenButtonTap covers NewToolbar's Open-button closure on both paths: an
// OnOpen override is invoked directly, and otherwise the built-in dialog is shown.
func TestToolbarOpenButtonTap(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)

	opened := false
	bar := NewToolbar(pv, ToolbarConfig{ShowOpen: true, OnOpen: func() { opened = true }})
	btns := findToolTipButtons(bar)
	if len(btns) == 0 {
		t.Fatal("toolbar has no buttons")
	}
	btns[0].OnTapped() // the Open button -> OnOpen override branch
	if !opened {
		t.Error("tapping Open with OnOpen set should call OnOpen")
	}

	// With a Window and no OnOpen, the same button routes through the built-in dialog.
	win := test.NewWindow(nil)
	defer win.Close()
	bar2 := NewToolbar(pv, ToolbarConfig{ShowOpen: true, Window: win})
	b2 := findToolTipButtons(bar2)
	if len(b2) == 0 {
		t.Fatal("windowed toolbar has no buttons")
	}
	b2[0].OnTapped() // -> ShowOpenDialog(pv, win); exercises the call, no panic
}
