package prettyview

import "unicode/utf8"

// This file implements v2 undo/redo (issue #42) as inverse byte-splice operations, not
// whole-document snapshots: the model is rebuildable from the buffer bytes alone, so an
// undo entry only needs the replaced byte range plus the bytes removed/inserted and the
// caret on each side. History is capped (WithUndoLimit) so it respects the memory bound.

// editOp is one undoable change: at off, the bytes `removed` were replaced by the bytes
// `inserted`. Undo reverses it (delete inserted, restore removed); redo re-applies it.
type editOp struct {
	at          int
	removed     []byte
	inserted    []byte
	caretBefore int
	caretAfter  int
	coalescable bool // a simple forward single-rune insert that may merge with the next
}

// editHistory is a bounded undo/redo stack of editOps.
type editHistory struct {
	undo  []editOp
	redo  []editOp
	limit int
}

// recordEdit pushes op onto the undo stack, coalescing a run of single-rune inserts into
// one entry (typing a word is one undo) and evicting the oldest entry past the cap. A
// new edit invalidates the redo stack.
func (pv *PrettyView) recordEdit(op editOp) {
	h := &pv.hist
	if op.coalescable && !pv.coalesceBreak && len(h.undo) > 0 {
		prev := &h.undo[len(h.undo)-1]
		if prev.coalescable && len(prev.removed) == 0 && op.at == prev.at+len(prev.inserted) {
			prev.inserted = append(prev.inserted, op.inserted...)
			prev.caretAfter = op.caretAfter
			h.redo = h.redo[:0]
			pv.coalesceBreak = false
			return
		}
	}
	h.undo = append(h.undo, op)
	if h.limit > 0 && len(h.undo) > h.limit {
		// Drop the oldest entry, keeping the length bounded and releasing its bytes.
		copy(h.undo, h.undo[1:])
		h.undo[len(h.undo)-1] = editOp{}
		h.undo = h.undo[:len(h.undo)-1]
	}
	h.redo = h.redo[:0]
	pv.coalesceBreak = !op.coalescable
}

// Undo reverts the most recent edit (text and caret). No-op in read-only mode or on an
// empty stack. Call it on the Fyne goroutine.
func (pv *PrettyView) Undo() {
	if !pv.cfg.editable || pv.buf == nil || len(pv.hist.undo) == 0 {
		return
	}
	op := pv.hist.undo[len(pv.hist.undo)-1]
	pv.hist.undo = pv.hist.undo[:len(pv.hist.undo)-1]
	pv.buf.Delete(op.at, len(op.inserted))
	pv.buf.Insert(op.at, op.removed)
	pv.hist.redo = append(pv.hist.redo, op)
	pv.coalesceBreak = true
	pv.reprojectRaw()
	pv.setCaretOff(op.caretBefore)
	pv.afterEdit()
}

// Redo re-applies the most recently undone edit. No-op in read-only mode or on an empty
// redo stack. Call it on the Fyne goroutine.
func (pv *PrettyView) Redo() {
	if !pv.cfg.editable || pv.buf == nil || len(pv.hist.redo) == 0 {
		return
	}
	op := pv.hist.redo[len(pv.hist.redo)-1]
	pv.hist.redo = pv.hist.redo[:len(pv.hist.redo)-1]
	pv.buf.Delete(op.at, len(op.removed))
	pv.buf.Insert(op.at, op.inserted)
	pv.hist.undo = append(pv.hist.undo, op)
	pv.coalesceBreak = true
	pv.reprojectRaw()
	pv.setCaretOff(op.caretAfter)
	pv.afterEdit()
}

// isSingleRune reports whether s is exactly one UTF-8 rune.
func isSingleRune(s []byte) bool {
	_, n := utf8.DecodeRune(s)
	return n > 0 && n == len(s)
}

// bufRange returns a copy of the buffer's [lo, hi) bytes (for recording removed text).
func (pv *PrettyView) bufRange(lo, hi int) []byte {
	b := pv.buf.Bytes()
	return append([]byte(nil), b[lo:hi]...)
}
