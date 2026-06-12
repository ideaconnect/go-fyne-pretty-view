package prettyview

import (
	"sync/atomic"
	"testing"
	"time"
)

// These tests exercise the REAL async debounce path (#71): a live time.AfterFunc fires on
// its own goroutine and, via fyne.Do, runs the settle/scan callback — the timer +
// destroyed-guard + generation lines that every other test pokes synchronously around. In
// Fyne's test driver fyne.Do runs the closure synchronously on the (non-main) timer
// goroutine, so the previous suite raced reading pv state alongside it. The fix: synchronize
// through a channel SIGNALLED INSIDE the callback and assert only on channel-delivered data,
// which establishes happens-before and is race-clean.

// TestDebounceSettleFiresAsync: typing arms the debounced settle; the callback must fire via
// the real AfterFunc->fyne.Do path and deliver the settled text. Run under -race.
func TestDebounceSettleFiresAsync(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: 20 * time.Millisecond})
	defer win.Close()

	done := make(chan string, 1)
	pv.SetOnChanged(func(s string) {
		select {
		case done <- s: // editSettled calls onChanged last; the receive synchronizes-with it
		default:
		}
	})

	typeStr(pv, "hi") // arms editDeb.schedule(20ms, editSettled)
	select {
	case s := <-done:
		if s == "" {
			t.Error("async settle delivered empty text")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debounced settle callback never fired via the AfterFunc->fyne.Do path")
	}
}

// NOTE on the SEARCH side of the shared debouncer: its callback (Search -> runSearch ->
// searchDeb.supersede) re-touches the debouncer's own state. In PRODUCTION fyne.Do marshals
// that onto the main event loop — the same goroutine as schedule() — so it is single-threaded
// and safe. Fyne's TEST driver instead runs fyne.Do synchronously on the AfterFunc's own
// goroutine, so an async search callback would touch db.timer concurrently with the arming
// goroutine — a test-driver-only data race that does not exist in production. The edit-side
// async test above covers the AfterFunc->fyne.Do->fn mechanism without that artifact (its
// AutoFormatOff settle does not re-enter the debouncer); the search debouncer's bookkeeping
// is covered synchronously by the tests in search_test.go.

// TestDebounceDestroyDropsCallback: arming a settle then tearing the widget down must drop
// the callback — the destroyed guard (and Stop) ensure no settle fires against freed state
// after Destroy, so no goroutine runs the callback. Closes the shutdown-leak window (#71).
func TestDebounceDestroyDropsCallback(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: 40 * time.Millisecond})
	defer win.Close()

	var fired atomic.Bool
	pv.SetOnChanged(func(string) { fired.Store(true) })

	typeStr(pv, "x") // arms the 40ms timer
	pv.r.Destroy()   // sets destroyed + supersedes: the callback must not run
	time.Sleep(120 * time.Millisecond)
	if fired.Load() {
		t.Error("a settle callback fired after Destroy — the destroyed guard failed")
	}
}
