package prettyview

import (
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
)

// debouncer coalesces a burst of calls into a single deferred action run on the Fyne
// goroutine. It is the shared engine behind the widget's two timers — the keystroke-driven
// search scan (searchDebounced) and the post-edit settle (scheduleReformat) — which had
// byte-identical timer/generation/fyne.Do/destroyed logic before this consolidation.
//
// All four guarantees live here, in one place:
//   - run fn once the burst stops (a newer schedule cancels the pending one),
//   - run it on the Fyne thread (fyne.Do marshals it onto the main event loop, so it
//     serializes with reflow/rebuild, which also run there),
//   - never run a callback superseded by a newer call (generation check), and
//   - never run one after teardown (the shared destroyed flag — Timer.Stop cannot cancel
//     a callback that has already fired; the flag closes that window).
//
// It is NOT safe for concurrent use: every method must be called on the Fyne goroutine.
// The only field the timer's own goroutine reads is destroyed (atomic).
type debouncer struct {
	timer     *time.Timer
	gen       int
	destroyed *atomic.Bool // shared widget-teardown flag (points at PrettyView.destroyed)
}

// schedule runs fn after delay d, coalescing a burst. It first supersedes any pending or
// already-queued callback, then either runs fn immediately (d <= 0, synchronously on the
// caller's goroutine) or arms a timer whose callback marshals fn onto the Fyne goroutine
// and skips itself if superseded or torn down in the meantime.
func (db *debouncer) schedule(d time.Duration, fn func()) {
	db.supersede()
	if d <= 0 {
		fn()
		return
	}
	gen := db.gen
	db.timer = time.AfterFunc(d, func() {
		if db.destroyed.Load() {
			return
		}
		fyne.Do(func() {
			if db.destroyed.Load() || gen != db.gen {
				return
			}
			fn()
		})
	})
}

// supersede is the authoritative invalidation point: it cancels any pending timer and
// bumps the generation so an already-fired-but-queued callback recognizes itself as stale
// and skips. Call it on a new burst (schedule does), an immediate synchronous run, a
// clear, a programmatic reload, or a re-create. Best-effort: it cannot un-run a callback
// already inside fn (the destroyed flag covers teardown). Stopping a nil timer is a no-op,
// so supersede doubles as a bare generation bump when no timer is armed.
func (db *debouncer) supersede() {
	if db.timer != nil {
		db.timer.Stop()
		db.timer = nil
	}
	db.gen++
}
