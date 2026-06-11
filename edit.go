package prettyview

import (
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// This file implements v2 edit mode (docs/DESIGN.md §12). The widget edits a separate
// gap buffer (pv.buf) and never mutates a parsed Document. While editing, the displayed
// document is the *raw* projection of the buffer, so display lines map 1:1 to buffer
// lines and the caret (sel.focus) is exactly a buffer (line, col). Structured
// re-formatting and live syntax colors arrive on a debounced pause in a later issue.
//
// The mode is fixed at construction (WithEditable); there is no runtime setter and the
// widget renders no toggle — see DECISION V2-3.

// Editable reports whether this widget was constructed as an editor (WithEditable).
// The input-vs-output mode is fixed at construction and cannot change for the widget's
// lifetime; there is deliberately no SetEditable. A read-only widget behaves
// byte-for-byte like a v1 viewer.
func (pv *PrettyView) Editable() bool { return pv.cfg.editable }

// editKey handles the keys edit mode owns, returning true if it consumed the event.
// Keys it returns false for (Escape, PageUp/Down, …) fall through to the read-only
// navigation in TypedKey.
func (pv *PrettyView) editKey(ev *fyne.KeyEvent) bool {
	switch ev.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		pv.editInsert([]byte{'\n'})
	case fyne.KeyBackspace:
		pv.editDelete(false)
	case fyne.KeyDelete:
		pv.editDelete(true)
	case fyne.KeyLeft:
		pv.caretStepRune(false)
	case fyne.KeyRight:
		pv.caretStepRune(true)
	case fyne.KeyUp:
		pv.keyMoveCaret(-1, keepCol, false, false)
	case fyne.KeyDown:
		pv.keyMoveCaret(1, keepCol, false, false)
	case fyne.KeyHome:
		pv.keyMoveCaret(0, 0, true, false)
	case fyne.KeyEnd:
		pv.keyMoveCaret(0, 0, false, true)
	case fyne.KeySpace:
		// A space is inserted via TypedRune(' '); swallow the key so it does not page
		// down (the read-only meaning of Space).
		return true
	default:
		return false
	}
	return true
}

// caretOff is the caret's byte offset in the buffer. An unplaced caret is offset 0.
// While the structured (reformatted) projection is shown, the caret's (line, col) is in
// structured space and does not map 1:1 to the buffer, so the stable caretBuf is used;
// in the raw projection the offset is read directly from sel.focus.
func (pv *PrettyView) caretOff() int {
	if !pv.sel.placed {
		return 0
	}
	if pv.editStructured {
		return pv.caretBuf
	}
	return pv.buf.ByteOffAt(int(pv.sel.focus.line), pv.sel.focus.col)
}

// setCaretOff collapses the selection to a caret at byte offset off (re-derived as a
// buffer (line, col)). Used after every edit, where the buffer is the source of truth.
func (pv *PrettyView) setCaretOff(off int) {
	line, col := pv.buf.LineColAt(off)
	pv.sel.focus = modelPos{line: int32(line), col: col}
	pv.sel.anchor = pv.sel.focus
	pv.sel.active = false
	pv.sel.placed = true
	pv.sel.grab = grabNone
}

// selectionByteRange returns the active selection as a [lo, hi) buffer byte range.
func (pv *PrettyView) selectionByteRange() (lo, hi int, ok bool) {
	if !pv.sel.active {
		return 0, 0, false
	}
	a := pv.buf.ByteOffAt(int(pv.sel.anchor.line), pv.sel.anchor.col)
	b := pv.buf.ByteOffAt(int(pv.sel.focus.line), pv.sel.focus.col)
	if a > b {
		a, b = b, a
	}
	return a, b, a != b
}

// editInsert writes s at the caret, first removing any active selection.
func (pv *PrettyView) editInsert(s []byte) {
	if !pv.cfg.editable || pv.buf == nil || len(s) == 0 {
		return
	}
	pv.ensureRawForEdit()
	at := pv.caretOff()
	if lo, hi, ok := pv.selectionByteRange(); ok {
		pv.buf.Delete(lo, hi-lo)
		at = lo
	}
	pv.buf.Insert(at, s)
	pv.reprojectRaw()
	pv.setCaretOff(at + len(s))
	pv.afterEdit()
}

// editDelete removes the active selection, or one rune before (Backspace) / at
// (Delete) the caret. Deleting across a line start joins lines, since the buffer is
// flat bytes and '\n' is just another byte.
func (pv *PrettyView) editDelete(forward bool) {
	if !pv.cfg.editable || pv.buf == nil {
		return
	}
	pv.ensureRawForEdit()
	if lo, hi, ok := pv.selectionByteRange(); ok {
		pv.buf.Delete(lo, hi-lo)
		pv.reprojectRaw()
		pv.setCaretOff(lo)
		pv.afterEdit()
		return
	}
	at := pv.caretOff()
	src := pv.buf.Bytes()
	if forward {
		if at >= len(src) {
			return
		}
		_, n := utf8.DecodeRune(src[at:])
		pv.buf.Delete(at, n)
		pv.reprojectRaw()
		pv.setCaretOff(at)
	} else {
		if at <= 0 {
			return
		}
		_, n := utf8.DecodeLastRune(src[:at])
		pv.buf.Delete(at-n, n)
		pv.reprojectRaw()
		pv.setCaretOff(at - n)
	}
	pv.afterEdit()
}

// caretStepRune moves the caret one rune left/right, collapsing any selection. It walks
// raw byte offsets, so it crosses line boundaries naturally.
func (pv *PrettyView) caretStepRune(forward bool) {
	if pv.buf == nil {
		return
	}
	pv.ensureRawForEdit()
	if !pv.sel.placed { // first arrow just places the caret at the top of the buffer
		pv.setCaretOff(0)
		pv.revealCaret()
		return
	}
	at := pv.caretOff()
	src := pv.buf.Bytes()
	if forward {
		if at >= len(src) {
			return
		}
		_, n := utf8.DecodeRune(src[at:])
		at += n
	} else {
		if at <= 0 {
			return
		}
		_, n := utf8.DecodeLastRune(src[:at])
		at -= n
	}
	pv.setCaretOff(at)
	pv.revealCaret()
}

// keyMoveCaret moves the caret like keyExtend (wrap-aware vertical / line-bound moves)
// but collapses the selection, so a plain arrow in edit mode moves without selecting.
func (pv *PrettyView) keyMoveCaret(dRows, col int, toLineStart, toLineEnd bool) {
	pv.ensureRawForEdit()
	pv.keyExtend(dRows, col, toLineStart, toLineEnd)
	pv.sel.anchor = pv.sel.focus
	pv.sel.active = false
	pv.refreshSelectionView()
}

// reprojectRaw rebuilds the displayed document as the raw line-split projection of the
// edit buffer (DECISION V2-1: never mutate a Document; rebuild from bytes). The new
// Document zero-copies into the buffer snapshot's own bytes, so invariant 3 holds.
func (pv *PrettyView) reprojectRaw() {
	pv.doc = parse.ParseEditable(pv.buf.Bytes(), pv.cfg.collapseDepth, pv.cfg.tabWidth)
}

// afterEdit resizes/reflows for the new line count and keeps the caret in view.
func (pv *PrettyView) afterEdit() {
	pv.applyGutter() // the line count (and so the gutter digit width) may have changed
	pv.refreshContent()
	pv.revealCaret()
	pv.scheduleReformat() // a typing pause re-parses into the structured projection (#40)
	if pv.onDataChanged != nil {
		pv.onDataChanged()
	}
}

// revealCaret scrolls so the caret is visible and repaints the caret/selection.
func (pv *PrettyView) revealCaret() {
	if pv.sel.placed {
		pv.centerOnLine(pv.sel.focus.line, pv.sel.focus.col)
	}
	pv.refreshSelectionView()
}
