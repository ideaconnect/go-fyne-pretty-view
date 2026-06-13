package parse

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// makeJSON builds a minified JSON object of at least target bytes, with a mix of keys,
// numbers, strings, bools and nulls so the colorizer (when it runs) produces every role.
// It is intentionally a single physical line (minified), which is the #65 worst case: the
// whole buffer is one line the live colorizer would otherwise re-lex on every keystroke.
func makeJSON(target int) []byte {
	var b strings.Builder
	b.Grow(target + 64)
	b.WriteByte('{')
	first := true
	for i := 0; b.Len() < target; i++ {
		if !first {
			b.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&b, "\"key%d\":[%d,\"val%d\",true,null]", i, i, i)
	}
	b.WriteByte('}')
	return []byte(b.String())
}

// bytesAllocated runs fn once and returns the heap bytes it allocated. #65's pathology is a
// ~22 MB transient span array plus per-token []model.Seg churn, i.e. a bytes-allocated cost
// the colorizer incurs over the whole buffer per keystroke; AllocsPerRun would only count
// allocation operations (the lexer presizes one span slice, so the op count barely moves),
// so we measure bytes — the dimension #65 actually reported (261 MB) and the fix removes.
func bytesAllocated(fn func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// assertDisplayRunesEqualBufferRunes pins the HARD caret-exactness invariant: per physical
// line, the display runes (what the caret column counts) equal the buffer runes of that
// line. It holds identically on both sides of the budget cliff (colored vs monochrome), so
// a budget-induced switch to the monochrome split never shifts a caret position.
func assertDisplayRunesEqualBufferRunes(t *testing.T, src []byte, d *model.Document) {
	t.Helper()
	lines := strings.Split(string(src), "\n")
	if d.TotalLines() != len(lines) {
		t.Fatalf("TotalLines = %d, want %d (one display line per physical line)", d.TotalLines(), len(lines))
	}
	for i, ln := range lines {
		if got, want := d.LineRuneLen(int32(i)), utf8.RuneCount([]byte(ln)); got != want {
			t.Errorf("line %d: display runes %d, want buffer runes %d", i, got, want)
		}
	}
}

// maxOverBudgetAllocBytesPerByte caps how many heap bytes the over-budget (monochrome)
// reproject may allocate per buffer byte. Measured: a ~3 MB over-budget buffer allocates
// ~17.7 MB monochrome (~5.9x) vs ~91.7 MB colored (~30x) — the colored path's bulk is the
// transient span array + per-token segment churn the budget gate removes (#65). 12x leaves
// comfortable headroom above the monochrome cost while staying far below the colored cost,
// so this guard fails CI if whole-buffer lexing is reintroduced on the hot path.
const maxOverBudgetAllocBytesPerByte = 12

// T1 — over-budget caps allocation BYTES (the #65 regression lock). If a future change
// re-enables whole-buffer color lexing above the budget, the per-reproject bytes blow back
// up to the ~30x colored level and this fails.
func TestParseEditableColored_BudgetCapsAllocs(t *testing.T) {
	big := makeJSON(LiveColorBudgetBytes + (1 << 20)) // just over budget
	if WithinLiveColorBudget(len(big)) {
		t.Fatalf("test buffer (%d bytes) is not over budget (%d)", len(big), LiveColorBudgetBytes)
	}
	limit := uint64(len(big)) * maxOverBudgetAllocBytesPerByte
	// Average a few runs to smooth GC/MemStats jitter.
	const runs = 5
	var total uint64
	for i := 0; i < runs; i++ {
		total += bytesAllocated(func() { _ = ParseEditableColored(big, FormatJSON, 0) })
	}
	avg := total / runs
	if avg > limit {
		t.Fatalf("over-budget reproject allocated %d bytes/run (%.1fx buffer), want <= %d (%dx) — whole-buffer lexing back on hot path?",
			avg, float64(avg)/float64(len(big)), limit, maxOverBudgetAllocBytesPerByte)
	}
}

// T2 — below budget still colors. Guards that the budget fix did not satisfy T1 by simply
// disabling color everywhere: a sub-budget JSON buffer must still carry non-plain roles.
func TestParseEditableColored_BelowBudgetColors(t *testing.T) {
	src := makeJSON(1 << 10)
	if !WithinLiveColorBudget(len(src)) {
		t.Fatalf("sub-budget test buffer (%d bytes) is unexpectedly over budget", len(src))
	}
	d := ParseEditableColored(src, FormatJSON, 0)
	colored := false
	for li := 0; li < d.TotalLines(); li++ {
		for _, s := range d.LineSegs(int32(li)) {
			if s.Role != model.RolePlain {
				colored = true
			}
		}
	}
	if !colored {
		t.Fatalf("below-budget JSON has no colored (non-RolePlain) segment; live color regressed")
	}
}

// T3 — the 1:1 display-runes==buffer-runes invariant holds on both sides of the budget
// cliff (just under, exactly at, and one byte over). This is the HARD caret-exactness
// invariant: switching to the monochrome split above budget must not move any caret column.
func TestEditProjection_RunesMatchBuffer_AcrossBudget(t *testing.T) {
	for _, n := range []int{1 << 10, LiveColorBudgetBytes, LiveColorBudgetBytes + 1} {
		src := makeJSON(n)
		d := ParseEditableColored(src, FormatJSON, 0)
		assertDisplayRunesEqualBufferRunes(t, src, d)
	}
}

// T4 — the over-budget projection never reflows and never fails: it is a flat
// KindRawLine-per-physical-line document with every Line.Fold == NoNode, even for a
// deliberately-invalid mid-edit JSON blob, and it never panics.
func TestParseEditableColored_OverBudgetFlatAndTolerant(t *testing.T) {
	// A truncated / unbalanced JSON blob padded past the budget: mid-edit invalid input.
	blob := makeJSON(LiveColorBudgetBytes + (1 << 20))
	blob = blob[:len(blob)-1]                   // drop the closing brace: unbalanced
	blob = append(blob, []byte("\n{\"x\":")...) // an extra partial line at the end
	d := ParseEditableColored(blob, FormatJSON, 0)

	lines := strings.Split(string(blob), "\n")
	if d.TotalLines() != len(lines) {
		t.Fatalf("over-budget invalid blob: TotalLines = %d, want %d", d.TotalLines(), len(lines))
	}
	for i := 0; i < len(d.Lines); i++ {
		if d.Lines[i].Fold != model.NoNode {
			t.Errorf("over-budget line %d has Fold %v, want NoNode (no folding => no reflow)", i, d.Lines[i].Fold)
		}
	}
	// And the caret-exactness invariant still holds on the invalid, over-budget input.
	assertDisplayRunesEqualBufferRunes(t, blob, d)
}

// BenchmarkLiveColorizeKeystroke_OverBudget anchors the per-keystroke reproject cost for a
// just-over-budget buffer (the #65 117 ms / 261 MB pathology). With the budget gate the
// allocation drops from the colored ~30x to the monochrome ~6x; the bench keeps that win
// visible and tracked over time. Compare against a sub-budget run to see the colored cost.
func BenchmarkLiveColorizeKeystroke_OverBudget(b *testing.B) {
	src := makeJSON(LiveColorBudgetBytes + (1 << 20))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ParseEditableColored(src, FormatJSON, 0)
	}
}
