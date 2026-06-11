package prettyview

import (
	"hash/fnv"
	"time"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// This file is the v2 live-format engine (docs/DESIGN.md §12, issue #40). While the
// user types, the widget shows the cheap raw projection of the edit buffer (#39); on a
// debounced pause (or an explicit Reformat) it re-parses the buffer structurally and
// shows the pretty, syntax-colored projection. The buffer bytes are never rewritten by
// the reformat, so the caret's byte offset is stable across the raw<->structured swap.

// SetInputConfig updates the edit-mode formatting knobs after construction, merging
// non-zero fields over the current config (the field-merge WithSearchConfig uses).
// No-op for a read-only widget. Call it on the Fyne goroutine.
func (pv *PrettyView) SetInputConfig(c InputConfig) { c.mergeInto(&pv.cfg.input) }

// SetOnChanged registers a callback invoked (debounced, after the edited text settles)
// with the current buffer text. Use it to mirror the edited content into the host.
// Setting it replaces any previous callback. Fires only for editable widgets.
func (pv *PrettyView) SetOnChanged(fn func(string)) { pv.onChanged = fn }

// Reformat re-parses the edit buffer under the active format and shows the structured,
// pretty-printed projection now, regardless of the AutoFormat mode. Invalid input
// degrades to the raw projection without panicking. No-op for a read-only widget. Call
// it on the Fyne goroutine.
func (pv *PrettyView) Reformat() {
	if !pv.cfg.editable {
		return
	}
	pv.reformatNow()
}

// scheduleReformat debounces the structured reformat after an edit, mirroring the
// search debounce (timer + generation counter + fyne.Do + destroyed guard). It arms a
// timer only when there is settle work to do — auto-format on pause, or an onChanged
// listener. A non-positive DebounceFor settles immediately.
func (pv *PrettyView) scheduleReformat() {
	if !pv.cfg.editable {
		return
	}
	if pv.cfg.input.AutoFormat != AutoFormatOnPause && pv.onChanged == nil {
		return // nothing to settle
	}
	pv.stopEditTimer()
	// Bump the generation so an earlier timer that already fired and queued its fyne.Do
	// closure (which Stop can no longer cancel) recognizes itself as stale and skips.
	pv.editGen++
	d := pv.cfg.input.DebounceFor
	if d <= 0 {
		pv.editSettled()
		return
	}
	gen := pv.editGen
	pv.editTimer = time.AfterFunc(d, func() {
		if pv.destroyed.Load() {
			return
		}
		fyne.Do(func() {
			if pv.destroyed.Load() || gen != pv.editGen {
				return
			}
			pv.editSettled()
		})
	})
}

// editSettled runs once a typing burst settles: reformat (when AutoFormatOnPause) and
// fire onChanged with the settled buffer text.
func (pv *PrettyView) editSettled() {
	if pv.cfg.input.AutoFormat == AutoFormatOnPause {
		pv.reformatNow()
	}
	if pv.onChanged != nil {
		pv.onChanged(string(pv.buf.Bytes()))
	}
}

// reformatNow swaps the displayed document to the structured projection of the current
// buffer (the same render path as SetData). Invalid mid-edit input degrades to the raw
// projection (no panic, no structured swap). The buffer bytes are unchanged, so the
// caret's byte offset is stable; the structured caret is re-placed at line granularity
// here (rune-precise re-anchoring is #41).
func (pv *PrettyView) reformatNow() {
	if !pv.cfg.editable || pv.buf == nil {
		return
	}
	snapshot := pv.buf.Bytes()
	if pv.editStructured && len(snapshot) == pv.lastFmtLen && hash64(snapshot) == pv.lastFmtHash {
		return // already showing the structured form of these exact bytes (idempotent guard)
	}
	pv.caretBuf = pv.caretOff() // exact offset from the current (raw or structured) caret
	nd := parse.Parse(snapshot, pv.cfg.format, pv.cfg.collapseDepth, pv.cfg.tabWidth)
	pv.lastFmtLen, pv.lastFmtHash = len(snapshot), hash64(snapshot)

	if nd.Format == FormatRaw {
		// A structured parse that failed (or genuinely raw content) keeps the edit-raw
		// projection — which, unlike parse.Parse's raw fallback, has the trailing line
		// the caret needs.
		pv.reprojectRaw()
		pv.editStructured = false
		pv.placeCaretFromBuf()
	} else {
		pv.doc = nd
		pv.editStructured = true
		line := pv.doc.LineAtSourceOffset(pv.caretBuf) // coarse; rune-precise column is #41
		pv.sel.focus = modelPos{line: line, col: 0}
		pv.sel.anchor = pv.sel.focus
		pv.sel.active = false
	}
	pv.applyGutter()
	pv.ClearSearch() // matches from the pre-reformat projection are stale
	pv.Refresh()
}

// ensureRawForEdit reverts a structured preview to the raw edit projection before an
// edit or caret move, re-placing the caret from its stable byte offset. All editing
// then happens in the raw space, where the caret is an exact buffer position.
func (pv *PrettyView) ensureRawForEdit() {
	if !pv.editStructured {
		return
	}
	pv.reprojectRaw()
	pv.editStructured = false
	pv.placeCaretFromBuf()
}

// placeCaretFromBuf re-derives the caret (line, col) from caretBuf in the raw
// projection, where display lines map 1:1 to buffer lines.
func (pv *PrettyView) placeCaretFromBuf() {
	line, col := pv.buf.LineColAt(pv.caretBuf)
	pv.sel.focus = modelPos{line: int32(line), col: col}
	pv.sel.anchor = pv.sel.focus
	pv.sel.active = false
	pv.sel.placed = true
}

// stopEditTimer cancels and clears any pending debounced reformat. Best-effort, like
// stopSearchTimer; must run on the Fyne goroutine.
func (pv *PrettyView) stopEditTimer() {
	if pv.editTimer != nil {
		pv.editTimer.Stop()
		pv.editTimer = nil
	}
}

func hash64(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}
