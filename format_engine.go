package prettyview

import (
	"bytes"
	"time"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// This file is the v2 live-format engine (docs/DESIGN.md §12). In edit mode the widget
// always shows a syntax-colored, layout-preserving projection of the edit buffer (the
// caret stays an exact buffer position). Typing never reflows; it only re-colors. An
// explicit Reformat (or, opt-in, an auto-format on a typing pause / blur) pretty-prints by
// REWRITING the buffer bytes in place and remapping the caret once, so the prettified
// layout persists while you keep typing. Reformat only rewrites a structured, VALID parse;
// raw or invalid input is left exactly as typed (just recolored), so it can never corrupt
// in-progress text.

// SetInputConfig updates the edit-mode formatting knobs after construction, merging
// non-zero fields over the current config. No effect on a read-only widget. Call it on the
// Fyne goroutine.
func (pv *PrettyView) SetInputConfig(c InputConfig) { c.mergeInto(&pv.cfg.input) }

// SetOnChanged registers a callback invoked (debounced, after the edited text settles, and
// after each Reformat) with the current buffer text. Use it to mirror the edited content
// into the host. Setting it replaces any previous callback. Fires only for editable widgets.
func (pv *PrettyView) SetOnChanged(fn func(string)) { pv.onChanged = fn }

// Reformat pretty-prints the edit buffer NOW: it re-parses under the active format and, if
// the parse is structured and valid, rewrites the buffer to the indented form and remaps
// the caret to the same token (so the caret "stays in place"). Raw, invalid, or JSONC input
// is left untouched — only its colors/validity refresh — so a prettify never deletes
// content (JSONC is exempt because the parser does not yet retain every comment). It runs
// regardless of the AutoFormat mode and never panics. No-op for a read-only widget. Call it
// on the Fyne goroutine.
func (pv *PrettyView) Reformat() {
	if !pv.cfg.editable {
		return
	}
	pv.reformat()
	if pv.onChanged != nil {
		pv.onChanged(string(pv.buf.Bytes()))
	}
}

// scheduleReformat debounces the settle after an edit, mirroring the search debounce
// (timer + generation counter + fyne.Do + destroyed guard). It arms a timer only when a
// settle has observable work: auto-format on pause, or an onChanged / onValidation
// listener. A non-positive DebounceFor settles immediately. The settle never reflows by
// default (AutoFormatOff) — it only refreshes validity and fires onChanged.
func (pv *PrettyView) scheduleReformat() {
	if !pv.cfg.editable || pv.r == nil {
		return // not shown yet: the settle's work (validity + repaint) needs a live renderer
	}
	if pv.cfg.input.AutoFormat != AutoFormatOnPause && pv.onChanged == nil && pv.onValidation == nil {
		return // nothing observes a settle
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

// editSettled runs once a typing burst settles: refresh live parse validity (or, when
// AutoFormatOnPause is opted into, reflow), then fire onChanged with the settled text. The
// buffer layout is left exactly as typed unless AutoFormatOnPause is set.
func (pv *PrettyView) editSettled() {
	// Above the MaxEditBytes cap, skip the per-pause reparse (a full reparse on every pause
	// is infeasible for a very large buffer); an explicit Reformat still runs.
	if !pv.aboveEditCap() {
		if pv.cfg.input.AutoFormat == AutoFormatOnPause {
			pv.reformat()
		} else {
			pv.refreshParseStatus()
		}
	}
	if pv.onChanged != nil {
		pv.onChanged(string(pv.buf.Bytes()))
	}
}

// reformat re-parses the edit buffer and, for a structured & valid parse, rewrites the
// buffer to its pretty-printed bytes and remaps the caret once. Raw/invalid input is left
// untouched (only colors + validity refresh). It does NOT fire onChanged (callers do). The
// whole-buffer rewrite is recorded as one undo unit, so Ctrl+Z reverts a reformat.
func (pv *PrettyView) reformat() {
	if !pv.cfg.editable || pv.buf == nil {
		return
	}
	snapshot := pv.buf.Bytes()
	nd := parse.Parse(snapshot, pv.resolveFormat(snapshot), pv.cfg.collapseDepth, pv.cfg.tabWidth)
	status := pv.statusFor(nd, snapshot)
	pv.setParseStatus(status)

	if nd.Format == FormatRaw || nd.Format == FormatJSONC || !status.OK {
		// Never rewrite the buffer when it would risk losing content: genuinely raw input,
		// structured-but-invalid input (prettifying it would corrupt the in-progress text),
		// or JSONC — whose comments the structured parser only partially retains, so a
		// rewrite could silently delete them. Refresh colors + gutter only; the bytes and
		// caret are left exactly as is. (Lossless JSONC reformat awaits full comment
		// retention in the parser; see docs/DESIGN.md §12.)
		pv.reprojectRaw()
		pv.applyGutter()
		pv.refreshContent()
		pv.refreshSelectionView()
		return
	}

	pretty, spans := serializePretty(nd)
	if bytes.Equal(pretty, snapshot) {
		// Already pretty: no buffer change, no caret jump. Refresh colors idempotently.
		pv.reprojectRaw()
		pv.applyGutter()
		pv.refreshContent()
		pv.refreshSelectionView()
		return
	}

	off := pv.caretOff() // exact buffer offset in the current (colorized-raw) projection
	newOff := remapCaretOffset(spans, off, len(pretty))
	pv.coalesceBreak = true // a reformat ends the current typing run for undo
	pv.recordEdit(editOp{
		at:          0,
		removed:     append([]byte(nil), snapshot...),
		inserted:    append([]byte(nil), pretty...),
		caretBefore: off,
		caretAfter:  newOff,
	})
	pv.buf.Delete(0, pv.buf.Len())
	pv.buf.Insert(0, pretty)
	pv.reprojectRaw()
	pv.setCaretOff(newOff)
	pv.applyGutter()
	pv.ClearSearch() // matches from the pre-reformat layout are stale
	pv.refreshContent()
	pv.revealCaret()
}

// refreshParseStatus recomputes the live validity of the buffer (without reflowing) and,
// on a change, repaints so the gutter error tint follows. Used on a settle in the default
// (no auto-reformat) mode.
func (pv *PrettyView) refreshParseStatus() {
	prev := pv.parseStatus
	pv.setParseStatus(pv.statusFor(nil, pv.buf.Bytes()))
	if pv.parseStatus != prev {
		pv.applyGutter()
		pv.refreshContent()
	}
}

// statusFor computes the validity of snapshot's structured parse, with ErrorLine expressed
// as a buffer (== display) line so the gutter tint and status read the right row. nd, if
// non-nil, is a parse of snapshot already in hand (avoids a second parse).
func (pv *PrettyView) statusFor(nd *model.Document, snapshot []byte) ParseStatus {
	if nd == nil {
		nd = parse.Parse(snapshot, pv.resolveFormat(snapshot), pv.cfg.collapseDepth, pv.cfg.tabWidth)
	}
	for li := range nd.Lines {
		o := nd.Lines[li].Owner
		if o != model.NoNode && int(o) < len(nd.Nodes) && nd.Nodes[o].Kind == model.KindError {
			line, _ := pv.buf.LineColAt(int(nd.Nodes[o].SrcStart))
			return ParseStatus{OK: false, ErrorLine: line}
		}
	}
	return ParseStatus{OK: true, ErrorLine: -1}
}

// resolveFormat picks the concrete format for coloring/validity: the active content format
// (curFormat — the last explicit SetData/Reparse format, or the construction default), or
// the auto-detected one when that is FormatAuto. Auto-detecting live lets a fresh editor
// adopt colors as soon as the typed text looks like JSON/XML, while an explicit format is
// honored exactly (e.g. SetData(jsonBytes, FormatRaw) stays an uncolored plain-text editor).
func (pv *PrettyView) resolveFormat(src []byte) Format {
	if pv.curFormat != FormatAuto {
		return pv.curFormat
	}
	return parse.AutoDetect(src)
}

// stopEditTimer cancels and clears any pending debounced settle. Best-effort, like
// stopSearchTimer; must run on the Fyne goroutine.
func (pv *PrettyView) stopEditTimer() {
	if pv.editTimer != nil {
		pv.editTimer.Stop()
		pv.editTimer = nil
	}
}
