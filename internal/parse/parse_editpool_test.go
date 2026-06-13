package parse

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// The EditPool (#80) reuses the Document arenas + buffer snapshot across keystroke reprojects.
// Its entire correctness contract is "a pooled reproject yields a Document byte-identical to a
// fresh ParseEditableColored of the same bytes" — pooling reuses backing storage, never changes
// output. TestEditPoolEqualsFresh is the keystone that enforces exactly that across a long,
// randomized edit sequence (so the pool's grow/shrink/reset paths are all exercised), on every
// caret/render-relevant field and accessor. If pooling ever diverges, this fails loudly.

func cloneBytes(b []byte) []byte { return append([]byte(nil), b...) }

func truncForLog(b []byte) string {
	if len(b) > 120 {
		return string(b[:120]) + "…"
	}
	return string(b)
}

// assertDocsIdentical compares every field and accessor the caret, selection, search, fold and
// renderer rely on. Src/Aux/Nodes/Lines/Segs are exported arenas (deep-compared by value, so a
// reused slice with extra capacity still matches a fresh one); lineRunes/lineASCII/fold are
// private, checked through their public accessors.
func assertDocsIdentical(t *testing.T, f Format, step int, buf []byte, got, want *model.Document) bool {
	t.Helper()
	fail := func(format string, args ...any) bool {
		t.Errorf("fmt=%v step=%d buf=%q: "+format, append([]any{f, step, truncForLog(buf)}, args...)...)
		return false
	}
	if !bytes.Equal(got.Src, want.Src) {
		return fail("Src differs")
	}
	if !bytes.Equal(got.Aux, want.Aux) {
		return fail("Aux differs: pooled=%q fresh=%q", got.Aux, want.Aux)
	}
	if !reflect.DeepEqual(got.Nodes, want.Nodes) {
		return fail("Nodes differ:\n pooled=%+v\n fresh =%+v", got.Nodes, want.Nodes)
	}
	if !reflect.DeepEqual(got.Lines, want.Lines) {
		return fail("Lines differ:\n pooled=%+v\n fresh =%+v", got.Lines, want.Lines)
	}
	if !reflect.DeepEqual(got.Segs, want.Segs) {
		return fail("Segs differ:\n pooled=%+v\n fresh =%+v", got.Segs, want.Segs)
	}
	if got.MaxLineRunes != want.MaxLineRunes {
		return fail("MaxLineRunes %d != %d", got.MaxLineRunes, want.MaxLineRunes)
	}
	if got.MaxDepth != want.MaxDepth {
		return fail("MaxDepth %d != %d", got.MaxDepth, want.MaxDepth)
	}
	if got.Format != want.Format {
		return fail("Format %v != %v", got.Format, want.Format)
	}
	if got.TotalLines() != want.TotalLines() {
		return fail("TotalLines %d != %d", got.TotalLines(), want.TotalLines())
	}
	for i := 0; i < got.TotalLines(); i++ {
		li := int32(i)
		if got.LineString(li) != want.LineString(li) {
			return fail("line %d string: pooled=%q fresh=%q", i, got.LineString(li), want.LineString(li))
		}
		if got.LineRuneLen(li) != want.LineRuneLen(li) {
			return fail("line %d rune len %d != %d", i, got.LineRuneLen(li), want.LineRuneLen(li))
		}
		if got.LineIsByteGrid(li) != want.LineIsByteGrid(li) {
			return fail("line %d byte-grid %v != %v", i, got.LineIsByteGrid(li), want.LineIsByteGrid(li))
		}
		if got.Visible(li) != want.Visible(li) {
			return fail("line %d visible %v != %v", i, got.Visible(li), want.Visible(li))
		}
		if got.RowOfLine(li) != want.RowOfLine(li) {
			return fail("line %d rowOfLine %d != %d", i, got.RowOfLine(li), want.RowOfLine(li))
		}
	}
	if got.TotalVisibleRows() != want.TotalVisibleRows() {
		return fail("TotalVisibleRows %d != %d", got.TotalVisibleRows(), want.TotalVisibleRows())
	}
	return true
}

// randomEdit returns buf with one random byte-level edit applied. It is deliberately NOT
// rune-aware: a delete that splits a multi-byte rune is fine, because both the pooled and the
// fresh path see the identical resulting bytes — the projection is byte-driven and tolerant.
func randomEdit(rng *rand.Rand, buf []byte) []byte {
	insertAt := func(pos int, ins []byte) []byte {
		out := make([]byte, 0, len(buf)+len(ins))
		out = append(out, buf[:pos]...)
		out = append(out, ins...)
		return append(out, buf[pos:]...)
	}
	n := len(buf)
	pos := 0
	if n > 0 {
		pos = rng.Intn(n + 1)
	}
	switch rng.Intn(7) {
	case 0: // single ASCII rune
		return insertAt(pos, []byte{byte('a' + rng.Intn(26))})
	case 1: // newline (changes line count)
		return insertAt(pos, []byte{'\n'})
	case 2: // multibyte + grid-hostile bytes (tab, control, DEL)
		return insertAt(pos, []byte("中\t\x01\x7fz"))
	case 3: // multi-line paste
		return insertAt(pos, []byte("p\nq\nr"))
	case 4: // structural JSON/markup bytes
		return insertAt(pos, []byte(`{"k":<v/>}`))
	case 5: // delete one byte
		if n == 0 {
			return buf
		}
		d := rng.Intn(n)
		return append(buf[:d:d], buf[d+1:]...)
	default: // delete a range
		if n == 0 {
			return buf
		}
		a := rng.Intn(n)
		b := a + rng.Intn(n-a+1)
		return append(buf[:a:a], buf[b:]...)
	}
}

func TestEditPoolEqualsFresh(t *testing.T) {
	formats := []Format{FormatJSON, FormatJSONC, FormatXML, FormatHTML, FormatRaw}
	pools := make(map[Format]*EditPool, len(formats))
	for _, f := range formats {
		pools[f] = NewEditPool()
	}
	rng := rand.New(rand.NewSource(0xC0FFEE))
	buf := []byte(`{"a":[1,2],"b":"x","c":true,"d":null}` + "\n<r id=\"1\">t&amp;x</r>")
	for step := 0; step < 3000; step++ {
		buf = randomEdit(rng, buf)
		if len(buf) > 4000 { // keep the sweep fast; equality is size-independent
			buf = buf[:2000]
		}
		for _, f := range formats {
			got := pools[f].Reproject(cloneBytes(buf), f, 0)
			want := ParseEditableColored(cloneBytes(buf), f, 0)
			if !assertDocsIdentical(t, f, step, buf, got, want) {
				return // first divergence already logged with full context
			}
		}
	}
}

// TestEditPoolSteadyStateAllocs is the AllocsPerRun acceptance guard (#80): after warming, a
// pooled reproject of a large (over-budget, monochrome) buffer must allocate only a small
// constant — the arenas are reused, not re-allocated — and that constant must NOT grow with the
// buffer size. A regression to the fresh path blows this back up to the per-buffer arena churn.
func TestEditPoolSteadyStateAllocs(t *testing.T) {
	big := makeJSON(LiveColorBudgetBytes + (1 << 20)) // over budget => monochrome
	if WithinLiveColorBudget(len(big)) {
		t.Fatalf("fixture not over budget")
	}
	pool := NewEditPool()
	pool.Reproject(big, FormatJSON, 0) // warm: first call sizes the arenas
	pool.Reproject(big, FormatJSON, 0) // settle capacities
	const wantMax = 8
	if n := testing.AllocsPerRun(50, func() { _ = pool.Reproject(big, FormatJSON, 0) }); n > wantMax {
		t.Errorf("steady-state pooled reproject allocated %.0f times, want <= %d (arenas not reused?)", n, wantMax)
	}

	big2 := makeJSON(2 * (LiveColorBudgetBytes + (1 << 20)))
	pool2 := NewEditPool()
	pool2.Reproject(big2, FormatJSON, 0)
	pool2.Reproject(big2, FormatJSON, 0)
	n1 := testing.AllocsPerRun(50, func() { _ = pool.Reproject(big, FormatJSON, 0) })
	n2 := testing.AllocsPerRun(50, func() { _ = pool2.Reproject(big2, FormatJSON, 0) })
	if n2 > n1+2 {
		t.Errorf("alloc count scaled with buffer size: %.0f at 1x vs %.0f at 2x (allocations not decoupled from length)", n1, n2)
	}
}

// TestEditPoolBudgetCrossing confirms pooling is transparent across the #65 color budget: the
// same pool, driven under->over->under the 2 MiB cliff, equals a fresh parse at each step and
// keeps the right colorization (colored below budget, monochrome above).
func TestEditPoolBudgetCrossing(t *testing.T) {
	// Pretty (short lines, each under maxColorLineBytes) so the below-budget side is genuinely
	// colored — a single minified megabyte line is rendered monochrome regardless of the budget.
	under := makeJSONPretty(LiveColorBudgetBytes - (1 << 20)) // colored
	over := makeJSONPretty(LiveColorBudgetBytes + (1 << 20))  // monochrome
	if !WithinLiveColorBudget(len(under)) || WithinLiveColorBudget(len(over)) {
		t.Fatalf("budget fixtures sized wrong: under=%d over=%d budget=%d", len(under), len(over), LiveColorBudgetBytes)
	}
	hasColor := func(d *model.Document) bool {
		for _, s := range d.Segs {
			if s.Role != model.RolePlain {
				return true
			}
		}
		return false
	}
	pool := NewEditPool()
	for i, src := range [][]byte{under, over, under, over} {
		got := pool.Reproject(cloneBytes(src), FormatJSON, 0)
		want := ParseEditableColored(cloneBytes(src), FormatJSON, 0)
		assertDocsIdentical(t, FormatJSON, i, src, got, want)
		colored := hasColor(got)
		if within := WithinLiveColorBudget(len(src)); within != colored {
			t.Errorf("step %d (%d bytes, within=%v): colored=%v, want %v", i, len(src), within, colored, within)
		}
	}
}

// TestEditPoolAuxBounded catches the highest-likelihood pooling bug: if ResetBuilder forgets to
// clear the intern cache, the 2nd reproject re-resolves "·" to a stale offset into the truncated
// Aux. Reprojecting content with grid-hostile bytes many times must keep Aux byte-for-byte equal
// to a fresh parse every time (never doubling or pointing at stale offsets).
func TestEditPoolAuxBounded(t *testing.T) {
	src := []byte("a\tb\x01c\nx\x7fy中z\nlast")
	pool := NewEditPool()
	want := ParseEditableColored(cloneBytes(src), FormatJSON, 0)
	for i := 0; i < 200; i++ {
		got := pool.Reproject(cloneBytes(src), FormatJSON, 0)
		if !bytes.Equal(got.Aux, want.Aux) {
			t.Fatalf("iteration %d: Aux grew/diverged: pooled=%q (len %d) fresh=%q (len %d) — intern cache not reset?",
				i, got.Aux, len(got.Aux), want.Aux, len(want.Aux))
		}
		if got.LineString(0) != want.LineString(0) {
			t.Fatalf("iteration %d: line 0 string diverged (stale Aux offset): %q vs %q", i, got.LineString(0), want.LineString(0))
		}
	}
}

// TestEditPoolSlackNotRead writes poison into the arena capacity left behind after the pool
// shrinks (large buffer then tiny one). A correct consumer indexes by len/SegCount, never cap,
// so the next reproject must still equal a fresh parse despite the garbage in the slack.
func TestEditPoolSlackNotRead(t *testing.T) {
	pool := NewEditPool()
	big := []byte(strings.Repeat("alpha\tbeta\x01\n", 400)) // many lines => large arenas
	pool.Reproject(cloneBytes(big), FormatJSON, 0)

	small := []byte("a\nb")
	got := pool.Reproject(cloneBytes(small), FormatJSON, 0)

	// Poison every arena's slack [len:cap] with values that would corrupt or panic if ever read.
	if ns := got.Nodes[:cap(got.Nodes)]; len(ns) > len(got.Nodes) {
		for i := len(got.Nodes); i < len(ns); i++ {
			ns[i] = model.Node{Parent: 1 << 30, Subtree: -1, HeadLine: 1 << 30, CloseLine: 1 << 30, SrcStart: 1 << 31, SrcEnd: 1 << 31}
		}
	}
	if ls := got.Lines[:cap(got.Lines)]; len(ls) > len(got.Lines) {
		for i := len(got.Lines); i < len(ls); i++ {
			ls[i] = model.Line{Owner: 1 << 30, SegFirst: 1 << 31, SegCount: 0xffff}
		}
	}
	if ss := got.Segs[:cap(got.Segs)]; len(ss) > len(got.Segs) {
		for i := len(got.Segs); i < len(ss); i++ {
			ss[i] = model.Segment{Start: 1 << 31, End: 1 << 31, Role: model.RolePlain, Buf: model.BufSrc}
		}
	}
	if as := got.Aux[:cap(got.Aux)]; len(as) > len(got.Aux) {
		for i := len(got.Aux); i < len(as); i++ {
			as[i] = 0xAA
		}
	}

	got2 := pool.Reproject(cloneBytes(small), FormatJSON, 0)
	want := ParseEditableColored(cloneBytes(small), FormatJSON, 0)
	assertDocsIdentical(t, FormatJSON, 0, small, got2, want)
}

// makeJSONPretty builds a pretty-printed (many short lines) JSON buffer of at least target
// bytes — the shape where line-level work would help, complementing makeJSON's minified single
// line. Used only to characterize the reproject benchmark across both buffer shapes.
func makeJSONPretty(target int) []byte {
	var b strings.Builder
	b.Grow(target + 64)
	b.WriteString("{\n")
	for i := 0; b.Len() < target; i++ {
		fmt.Fprintf(&b, "  \"key%d\": [%d, \"val%d\", true, null],\n", i, i, i)
	}
	b.WriteString("  \"end\": 0\n}")
	return []byte(b.String())
}

// BenchmarkReprojectPerKeystroke measures a single over-budget reproject, pooled vs fresh, on
// both buffer shapes (#80). The pooled allocs/op (ReportAllocs) collapse to a small constant on
// BOTH shapes — the acceptance evidence. Honest scope: ns/op is the win only modestly (the
// buffer-snapshot copy and the rune-count extent pass stay O(buffer)); the latency-proportional
// -to-edit splice is the deferred follow-up. Minified is the #65 worst case where no incremental
// approach helps; Pretty is the unbenchmarked many-line shape.
func BenchmarkReprojectPerKeystroke(b *testing.B) {
	shapes := []struct {
		name string
		src  []byte
	}{
		{"Minified3MB", makeJSON(3 << 20)},
		{"Pretty3MB", makeJSONPretty(3 << 20)},
	}
	for _, s := range shapes {
		src := s.src
		b.Run(s.name+"/Fresh", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = ParseEditableColored(src, FormatJSON, 0)
			}
		})
		b.Run(s.name+"/Pooled", func(b *testing.B) {
			pool := NewEditPool()
			pool.Reproject(src, FormatJSON, 0) // warm
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = pool.Reproject(src, FormatJSON, 0)
			}
		})
	}
}
