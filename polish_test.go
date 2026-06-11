package prettyview

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// TestCenterOnLineOutOfBoundsNoPanic guards the centerOnLine upper-bound check: a
// stale or bogus line index (e.g. a match recorded before a fold change) must be a
// no-op, not an out-of-range panic.
func TestCenterOnLineOutOfBoundsNoPanic(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":1,"b":2}`), FormatJSON, 400, 300)
	defer win.Close()
	pv.centerOnLine(int32(pv.doc.TotalLines()+10), 0) // past the end
	pv.centerOnLine(-5, 0)                            // before the start
}

// TestExpandToRevealsAndScrolls covers the public ExpandTo end to end: from a byte
// offset deep inside a collapsed subtree it must reveal the target line and scroll
// it into the viewport.
func TestExpandToRevealsAndScrolls(t *testing.T) {
	src := []byte(`{"outer":{"inner":{"target":"FINDME"}},"tail":1}`)
	pv, win := renderInWindow(t, src, FormatJSON, 400, 300)
	defer win.Close()

	pv.CollapseAll()
	off := strings.Index(string(src), `"target"`)
	node := pv.nodeAtByteOffset(off)
	if node == model.NoNode {
		t.Fatal("offset did not resolve to a node")
	}
	line := pv.doc.Nodes[node].HeadLine
	if pv.doc.Visible(line) {
		t.Fatal("target should be hidden after CollapseAll")
	}

	pv.ExpandTo(off)

	if !pv.doc.Visible(line) {
		t.Error("ExpandTo did not reveal the target line")
	}
	rowY := float32(pv.doc.RowOfLine(line)) * pv.met.RowH
	top := pv.r.scroll.Offset.Y
	bot := top + pv.r.scroll.Size().Height
	if rowY < top || rowY > bot {
		t.Errorf("target row y=%.0f not scrolled into viewport [%.0f,%.0f]", rowY, top, bot)
	}
}

// TestKeyboardUpAndBounds completes the keyboard-navigation coverage (Up, and the
// top/bottom clamps), complementing the existing Down/Page/Home/End test.
func TestKeyboardUpAndBounds(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	// Up from a scrolled position moves the viewport up.
	pv.r.scrollToOffset(fyne.NewPos(0, 10*pv.met.RowH))
	before := pv.r.scroll.Offset.Y
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if pv.r.scroll.Offset.Y >= before {
		t.Errorf("Up did not scroll up: %.1f >= %.1f", pv.r.scroll.Offset.Y, before)
	}

	// Up at the very top is clamped to 0 (no negative offset).
	pv.r.scrollToOffset(fyne.NewPos(0, 0))
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if got := pv.r.scroll.Offset.Y; got != 0 {
		t.Errorf("Up at top scrolled to %.1f, want 0", got)
	}

	// Down at the bottom is clamped to the max offset.
	maxY := pv.contentSize().Height - pv.r.scroll.Size().Height
	pv.r.scrollToOffset(fyne.NewPos(0, maxY))
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if got := pv.r.scroll.Offset.Y; got > maxY+0.5 {
		t.Errorf("Down at bottom exceeded max: %.1f > %.1f", got, maxY)
	}
}

// TestWithDefaultCollapseDepthOption checks the construction option actually drives
// auto-collapse on load through the public NewWithData path (the model projection
// has its own test; this guards the widget/option wiring).
func TestWithDefaultCollapseDepthOption(t *testing.T) {
	test.NewApp()
	src := []byte(`{"a":{"b":{"c":1}},"d":[1,2,3]}`)
	full := NewWithData(src, FormatJSON)
	collapsed := NewWithData(src, FormatJSON, WithDefaultCollapseDepth(1))
	if collapsed.doc.TotalVisibleRows() >= full.doc.TotalVisibleRows() {
		t.Errorf("WithDefaultCollapseDepth(1) did not collapse on load: %d >= %d",
			collapsed.doc.TotalVisibleRows(), full.doc.TotalVisibleRows())
	}
}

// TestWrapToggleControl checks the built-in Wrap checkbox drives SetWrap both ways.
func TestWrapToggleControl(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"k":"`+strings.Repeat("x ", 40)+`"}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 400))
	pv.Refresh()

	btn, ok := NewWrapToggle(pv).(fyne.Tappable)
	if !ok {
		t.Fatal("NewWrapToggle is not tappable")
	}
	test.Tap(btn) // toggle on
	if pv.Wrap() != WrapWord {
		t.Error("tapping the toggle did not enable WrapWord")
	}
	test.Tap(btn) // toggle off
	if pv.Wrap() != WrapNone {
		t.Error("tapping the toggle again did not restore WrapNone")
	}
}
