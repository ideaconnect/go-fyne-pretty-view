package prettyview

import (
	"strings"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// This file implements v2 edit mode (docs/DESIGN.md §12). The widget edits a separate
// gap buffer (pv.buf) and never mutates a parsed Document. While editing, the displayed
// document is always the syntax-colored, layout-preserving projection of the buffer
// (see reprojectRaw), so display lines map 1:1 to buffer lines and the caret (sel.focus)
// is exactly a buffer (line, col). An explicit Reformat (format_engine.go) pretty-prints
// by rewriting the buffer bytes in place; it never swaps to a separate display mode.
//
// The mode is fixed at construction (WithEditable); there is no runtime setter and the
// widget renders no toggle — see DECISION V2-3.

// Editable reports whether this widget was constructed as an editor (WithEditable).
// The input-vs-output mode is fixed at construction and cannot change for the widget's
// lifetime; there is deliberately no SetEditable. A read-only widget behaves
// byte-for-byte like a v1 viewer.
func (pv *PrettyView) Editable() bool { return pv.cfg.editable }

// Caret returns the caret's current display position as (line, col) — a 0-based display
// line and a rune column within it. For a fresh widget with no caret placed it is (0, 0).
func (pv *PrettyView) Caret() (line, col int) {
	return int(pv.sel.focus.line), pv.sel.focus.col
}

// SetCaret moves the caret to (line, col), clamping col into the line and revealing it,
// and returns true. It returns false (leaving the caret put) when line is out of
// [0, TotalLines), mirroring ScrollToLine's out-of-range contract. It collapses any
// selection. It works in read-only mode too, but only as a logical navigation position
// (it scrolls/centers the line); no caret bar is drawn — the visible caret is editor-only.
// Call it on the Fyne goroutine.
func (pv *PrettyView) SetCaret(line, col int) bool {
	if pv.doc == nil || line < 0 || line >= pv.doc.TotalLines() {
		return false
	}
	li := int32(line)
	col = clamp(col, 0, pv.doc.LineRuneLen(li)) // reuse the keyboard caret's clamp
	pv.sel.focus = modelPos{line: li, col: col}
	pv.sel.anchor = pv.sel.focus
	pv.sel.active = false
	pv.sel.placed = true
	pv.coalesceBreak = true
	pv.centerOnLine(li, col)
	pv.refreshSelectionView()
	return true
}

// aboveEditCap reports whether the edit buffer has exceeded the configured MaxEditBytes,
// above which auto-format-on-pause is suppressed (explicit Reformat still runs).
func (pv *PrettyView) aboveEditCap() bool {
	cap := pv.cfg.input.MaxEditBytes
	return cap > 0 && pv.buf != nil && pv.buf.Len() > cap
}

// Paste inserts the clipboard text at the caret, replacing any active selection. Line
// endings are normalized to LF (so a multi-line paste makes real display lines); any
// remaining control bytes render as safe placeholders in the live projection and as
// visible escapes once the structured reformat runs. It is one undo unit. No-op for a
// read-only widget. Call it on the Fyne goroutine.
func (pv *PrettyView) Paste() {
	if !pv.cfg.editable {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	content := app.Clipboard().Content()
	if content == "" {
		return
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	pv.editInsert([]byte(content)) // replaces the selection and records one undo unit
}

// Cut copies the current selection to the clipboard and deletes it, as one undo unit.
// No-op for a read-only widget or when nothing is selected. Call it on the Fyne goroutine.
func (pv *PrettyView) Cut() {
	if !pv.cfg.editable || pv.selectedText() == "" {
		return
	}
	pv.CopySelection()
	pv.editDelete(false) // an active selection is removed as a single undo unit
}

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

// caretOff is the caret's byte offset in the buffer. An unplaced caret is offset 0. The
// displayed projection is always the colorized-raw line split of the buffer, so the buffer
// (line, col) maps 1:1 and the offset is read directly from the live sel.focus.
func (pv *PrettyView) caretOff() int {
	if !pv.sel.placed {
		return 0
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

// selectionByteRange returns the active selection as a [lo, hi) buffer byte range. The
// displayed projection maps 1:1 onto the buffer, so both endpoints are direct buffer
// offsets.
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
	caretBefore := pv.caretOff()
	selLo, selHi, hasSel := pv.selectionByteRange()
	// Reject an edit that would grow the buffer past the MaxEditBytes cap (a delete or a
	// same-size replace is always allowed; only net growth is capped).
	if c := pv.cfg.input.MaxEditBytes; c > 0 {
		grow := len(s)
		if hasSel {
			grow -= selHi - selLo
		}
		if grow > 0 && pv.buf.Len()+grow > c {
			return
		}
	}
	at := caretBefore
	var removed []byte
	if hasSel {
		removed = pv.bufRange(selLo, selHi)
		pv.buf.Delete(selLo, selHi-selLo)
		at = selLo
	}
	pv.buf.Insert(at, s)
	pv.recordEdit(editOp{
		at:          at,
		removed:     removed,
		inserted:    append([]byte(nil), s...),
		caretBefore: caretBefore,
		caretAfter:  at + len(s),
		coalescable: removed == nil && isSingleRune(s) && s[0] != '\n',
	})
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
	caretBefore := pv.caretOff()
	selLo, selHi, hasSel := pv.selectionByteRange()
	if hasSel {
		removed := pv.bufRange(selLo, selHi)
		pv.buf.Delete(selLo, selHi-selLo)
		pv.recordEdit(editOp{at: selLo, removed: removed, caretBefore: caretBefore, caretAfter: selLo})
		pv.reprojectRaw()
		pv.setCaretOff(selLo)
		pv.afterEdit()
		return
	}
	at := caretBefore
	if forward {
		_, n := pv.buf.RuneAt(at) // decode one rune through the gap, no whole-buffer copy (#68)
		if n == 0 {
			return // caret at end of buffer
		}
		removed := pv.buf.Slice(at, at+n)
		pv.buf.Delete(at, n)
		pv.recordEdit(editOp{at: at, removed: removed, caretBefore: caretBefore, caretAfter: at})
		pv.reprojectRaw()
		pv.setCaretOff(at)
	} else {
		_, n := pv.buf.RuneBefore(at)
		if n == 0 {
			return // caret at start of buffer
		}
		removed := pv.buf.Slice(at-n, at)
		pv.buf.Delete(at-n, n)
		pv.recordEdit(editOp{at: at - n, removed: removed, caretBefore: caretBefore, caretAfter: at - n})
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
	pv.coalesceBreak = true // a caret move ends the current typing run for undo
	if !pv.sel.placed {     // first arrow just places the caret at the top of the buffer
		pv.setCaretOff(0)
		pv.revealCaret()
		return
	}
	at := pv.caretOff()
	if forward {
		_, n := pv.buf.RuneAt(at) // decode through the gap; no whole-buffer copy per arrow (#68)
		if n == 0 {
			return // at end of buffer
		}
		at += n
	} else {
		_, n := pv.buf.RuneBefore(at)
		if n == 0 {
			return // at start of buffer
		}
		at -= n
	}
	pv.setCaretOff(at)
	pv.revealCaret()
}

// keyMoveCaret moves the caret like keyExtend (wrap-aware vertical / line-bound moves)
// but collapses the selection, so a plain arrow in edit mode moves without selecting.
func (pv *PrettyView) keyMoveCaret(dRows, col int, toLineStart, toLineEnd bool) {
	pv.coalesceBreak = true // a caret move ends the current typing run for undo
	pv.keyExtend(dRows, col, toLineStart, toLineEnd)
	pv.sel.anchor = pv.sel.focus
	pv.sel.active = false
	pv.refreshSelectionView()
}

// reprojectRaw rebuilds the displayed document as the syntax-colored, layout-preserving
// line-split projection of the edit buffer (DECISION V2-1: never mutate a Document; rebuild
// from bytes). The colorizer assigns roles in place without reflowing, so display lines map
// 1:1 to buffer lines (the caret stays exact) and highlighting is live on every keystroke.
// The new Document zero-copies into the buffer snapshot's own bytes, so invariant 3 holds.
func (pv *PrettyView) reprojectRaw() {
	src := pv.buf.Bytes()
	pv.doc = parse.ParseEditableColored(src, pv.resolveFormat(src), pv.cfg.collapseDepth)
}

// rerenderProjection repaints the live colorized-raw projection in place: re-color the
// buffer, resize the gutter for the (possibly new) line count, and refresh content +
// selection — without rewriting bytes or moving the caret. It is the shared tail of
// reformat()'s two no-rewrite branches (raw/JSONC/invalid input, and already-pretty input).
func (pv *PrettyView) rerenderProjection() {
	pv.reprojectRaw()
	pv.applyGutter()
	pv.refreshContent()
	pv.refreshSelectionView()
}

// afterEdit resizes/reflows for the new line count and keeps the caret in view.
func (pv *PrettyView) afterEdit() {
	pv.applyGutter() // the line count (and so the gutter digit width) may have changed
	pv.refreshContent()
	pv.revealCaret()
	pv.scheduleReformat() // a typing pause refreshes live validity + fires onChanged (no reflow)
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
