package prettyview

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestDebouncerImmediateRunsSynchronously: a non-positive delay runs fn inline on the
// caller's goroutine and arms no timer (the "debouncing disabled" contract both the
// search and edit engines rely on for DebounceFor <= 0).
func TestDebouncerImmediateRunsSynchronously(t *testing.T) {
	var flag atomic.Bool
	db := debouncer{destroyed: &flag}

	ran := 0
	db.schedule(0, func() { ran++ })
	if ran != 1 {
		t.Errorf("d<=0 should run fn exactly once synchronously, ran=%d", ran)
	}
	if db.timer != nil {
		t.Error("d<=0 must not arm a timer")
	}
	db.schedule(-time.Second, func() { ran++ })
	if ran != 2 || db.timer != nil {
		t.Errorf("a negative delay must also run inline with no timer (ran=%d, timer=%v)", ran, db.timer)
	}
}

// TestDebouncerScheduleArmsAndSupersedes: a positive delay arms a timer and bumps the
// generation; supersede cancels it and bumps again. fn must not have run yet (the burst
// hasn't settled), so this drives only the synchronous bookkeeping — no fyne.Do wait.
func TestDebouncerScheduleArmsAndSupersedes(t *testing.T) {
	var flag atomic.Bool
	db := debouncer{destroyed: &flag}

	ran := 0
	g0 := db.gen
	db.schedule(time.Hour, func() { ran++ }) // long delay: never fires during the test
	if db.timer == nil {
		t.Fatal("a positive delay must arm a timer")
	}
	if db.gen == g0 {
		t.Error("schedule must bump the generation to supersede an earlier callback")
	}
	if ran != 0 {
		t.Error("fn must not run until the delay elapses")
	}

	g1 := db.gen
	db.supersede()
	if db.timer != nil {
		t.Error("supersede must stop and clear the pending timer")
	}
	if db.gen == g1 {
		t.Error("supersede must bump the generation")
	}
	// supersede with no armed timer is a bare generation bump, not a panic.
	g2 := db.gen
	db.supersede()
	if db.gen == g2 || db.timer != nil {
		t.Error("supersede on a nil timer must still bump the generation and stay nil")
	}
}
