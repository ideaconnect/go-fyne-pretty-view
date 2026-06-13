package parse

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// EditPool reuses the Document arenas and the buffer snapshot across the live editable
// reproject, so a keystroke reprojection allocates ~nothing in steady state instead of
// re-allocating the whole Document (Nodes/Lines/Segs/Aux/fold) plus a fresh buffer snapshot
// every keystroke (issue #80). It is single-goroutine and stateful: the *Document it returns
// and the snapshot that Document zero-copies into are BOTH reset in lockstep on the next
// Reproject, so a caller must not retain either across calls (the editable widget reads its
// document synchronously on the Fyne goroutine and never stores it). Pooling reuses backing
// storage only — the Document it produces is byte-identical to the free ParseEditableColored,
// which the equality fuzz (parse_editpool_test.go) pins.
//
// This is the allocation/GC-pressure win of #80. Per-keystroke wall time stays O(buffer): the
// buffer snapshot copy and the rune-count extent pass are inherently O(buffer); a true
// work-proportional-to-edit splice (which would revisit the immutable-Document decision) is a
// separate, deferred follow-up.
type EditPool struct {
	bld      *model.Builder
	snapshot []byte
}

// NewEditPool returns an empty pool. The first Reproject allocates the arenas; every subsequent
// one reuses them.
func NewEditPool() *EditPool { return &EditPool{} }

// Snapshot returns the pool's reusable snapshot backing slice, intended for
// buf.BytesInto(pool.Snapshot()) so the materialized buffer reuses one allocation too.
func (p *EditPool) Snapshot() []byte { return p.snapshot }

// Reproject rebuilds the editable-mode colored projection of src into the pool's reused arenas
// and returns the pooled *Document, valid only until the next Reproject. src must be the slice
// the Document will zero-copy into (typically buf.BytesInto(p.Snapshot())); the pool retains it
// as the snapshot. The result equals ParseEditableColored(src, format, collapseDepth).
func (p *EditPool) Reproject(src []byte, format Format, collapseDepth int) *model.Document {
	src = clampEditableSrc(src)
	p.snapshot = src
	if p.bld == nil {
		p.bld = model.NewPooledBuilder(src, format, collapseDepth)
	} else {
		p.bld.ResetBuilder(src, format, collapseDepth)
	}
	parseEditableInto(p.bld, src, format)
	return p.bld.Finish()
}
