package model

import (
	"bytes"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTextBufferEditsMatchStringOps fuzzes a long script of inserts and deletes against
// a trivial reference []byte implementation and asserts byte-equality after every op.
// The alphabet mixes ASCII, tabs, newlines, and multi-byte runes so the gap math is
// exercised across UTF-8 boundaries. Deterministic seed -> reproducible failures.
func TestTextBufferEditsMatchStringOps(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	frags := []string{"a", "Z", "\n", "\t", "é", "日", "ab", "世界", "  "}

	tb := NewTextBuffer([]byte("seed\nline\n"))
	ref := []byte("seed\nline\n")

	for op := 0; op < 4000; op++ {
		if len(ref) == 0 || rng.Intn(2) == 0 {
			// insert
			off := rng.Intn(len(ref) + 1)
			s := []byte(frags[rng.Intn(len(frags))])
			tb.Insert(off, s)
			ref = refInsert(ref, off, s)
		} else {
			// delete
			off := rng.Intn(len(ref))
			n := 1 + rng.Intn(5)
			if off+n > len(ref) {
				n = len(ref) - off
			}
			tb.Delete(off, n)
			ref = refDelete(ref, off, n)
		}
		if got := tb.Bytes(); !bytes.Equal(got, ref) {
			t.Fatalf("op %d: buffer diverged\n got=%q\nwant=%q", op, got, ref)
		}
		if tb.Len() != len(ref) {
			t.Fatalf("op %d: Len()=%d want %d", op, tb.Len(), len(ref))
		}
	}
}

func refInsert(ref []byte, off int, s []byte) []byte {
	out := make([]byte, 0, len(ref)+len(s))
	out = append(out, ref[:off]...)
	out = append(out, s...)
	out = append(out, ref[off:]...)
	return out
}

func refDelete(ref []byte, off, n int) []byte {
	out := make([]byte, 0, len(ref)-n)
	out = append(out, ref[:off]...)
	out = append(out, ref[off+n:]...)
	return out
}

// TestTextBufferBytesIsSnapshot is the alias-safety guard the whole v2 design rests on:
// a snapshot the parser already consumed must never be mutated by a later edit.
func TestTextBufferBytesIsSnapshot(t *testing.T) {
	tb := NewTextBuffer([]byte("hello"))
	snap := tb.Bytes()
	if string(snap) != "hello" {
		t.Fatalf("initial snapshot = %q, want %q", snap, "hello")
	}

	tb.Insert(5, []byte(" world"))
	if string(snap) != "hello" {
		t.Errorf("insert mutated a prior snapshot: %q", snap)
	}
	if got := string(tb.Bytes()); got != "hello world" {
		t.Errorf("after insert Bytes()=%q, want %q", got, "hello world")
	}

	snap2 := tb.Bytes()
	tb.Delete(0, 6) // drop "hello "
	if string(snap2) != "hello world" {
		t.Errorf("delete mutated a prior snapshot: %q", snap2)
	}
	if got := string(tb.Bytes()); got != "world" {
		t.Errorf("after delete Bytes()=%q, want %q", got, "world")
	}
}

// TestTextBufferDeleteClamps exercises Delete's two clamp paths (n past the end, and
// clampOff's lower/upper bounds) — all must mutate safely without panicking.
func TestTextBufferDeleteClamps(t *testing.T) {
	tb := NewTextBuffer([]byte("hello"))
	tb.Delete(3, 100) // n runs past the end -> clamps to the remaining "lo"
	if got := string(tb.Bytes()); got != "hel" {
		t.Errorf("Delete(3,100) = %q, want %q", got, "hel")
	}
	tb.Delete(1000, 1) // off past Len -> clampOff returns Len, nothing left to drop
	if got := string(tb.Bytes()); got != "hel" {
		t.Errorf("Delete at off>Len mutated the buffer: %q", got)
	}
	tb.Delete(-5, 2) // negative off -> clampOff returns 0 -> drops "he"
	if got := string(tb.Bytes()); got != "l" {
		t.Errorf("Delete(-5,2) = %q, want %q", got, "l")
	}
	tb.Delete(0, 0) // n<=0 is a no-op
	if got := string(tb.Bytes()); got != "l" {
		t.Errorf("Delete(_,0) changed the buffer: %q", got)
	}
}

// TestTextBufferEnsureGapGrows: an insert far larger than the initial 64-byte gap forces a
// reallocation; the content must survive the move intact.
func TestTextBufferEnsureGapGrows(t *testing.T) {
	tb := NewTextBuffer([]byte("x"))
	big := strings.Repeat("ab", 200) // 400 bytes >> minGap, so ensureGap reallocates
	tb.Insert(1, []byte(big))
	if got, want := string(tb.Bytes()), "x"+big; got != want {
		t.Errorf("large insert corrupted by realloc: len=%d want=%d", len(got), len(want))
	}
}

// TestTextBufferDecodeInvalidUTF8: invalid bytes decode as width-1 so a scan always makes
// progress (decodeRune's size<=0 guard); two lone 0xff bytes count as two columns.
func TestTextBufferDecodeInvalidUTF8(t *testing.T) {
	tb := NewTextBuffer([]byte{0xff, 0xfe})
	if l, c := tb.LineColAt(2); l != 0 || c != 2 {
		t.Errorf("LineColAt over invalid UTF-8 = (%d,%d), want (0,2)", l, c)
	}
}

// TestTextBufferRuneLineIndex round-trips byte<->(line,col) for every position of a
// fixture containing multi-byte runes, and checks a few absolute offsets by hand.
func TestTextBufferRuneLineIndex(t *testing.T) {
	const content = "héllo\nwörld\n日本語\nx"
	tb := NewTextBuffer([]byte(content))

	lines := strings.Split(content, "\n")
	for li, line := range lines {
		runeLen := utf8.RuneCountInString(line)
		for col := 0; col <= runeLen; col++ {
			off := tb.ByteOffAt(li, col)
			gotLine, gotCol := tb.LineColAt(off)
			if gotLine != li || gotCol != col {
				t.Errorf("round-trip (line=%d,col=%d) -> off=%d -> (line=%d,col=%d)",
					li, col, off, gotLine, gotCol)
			}
		}
	}

	// Absolute checks: multi-byte runes must advance the byte offset by their width.
	if got := tb.ByteOffAt(0, 0); got != 0 {
		t.Errorf("ByteOffAt(0,0)=%d, want 0", got)
	}
	if got := tb.ByteOffAt(0, 2); got != 3 { // h(1) + é(2) = 3 -> first 'l'
		t.Errorf("ByteOffAt(0,2)=%d, want 3 (past the 2-byte é)", got)
	}
	if got := tb.ByteOffAt(2, 1); got != 17 { // "héllo\n"=7, "wörld\n"=7 -> line 2 at 14, then 日(3) = 17
		t.Errorf("ByteOffAt(2,1)=%d, want 17 (past the 3-byte 日)", got)
	}
	if l, c := tb.LineColAt(tb.Len()); l != 3 || c != 1 {
		t.Errorf("LineColAt(Len)=(%d,%d), want (3,1) — last line 'x'", l, c)
	}
}

// TestTextBufferSliceMatchesBytes checks Slice(lo,hi) returns exactly Bytes()[lo:hi],
// including across the gap (an insert moves it), and copies (no aliasing). (#68)
func TestTextBufferSliceMatchesBytes(t *testing.T) {
	b := NewTextBuffer([]byte("hello world foo bar"))
	b.Insert(5, []byte("XYZ")) // moves the gap to offset 8: "helloXYZ world foo bar"
	full := b.Bytes()
	for _, c := range []struct{ lo, hi int }{{0, 3}, {4, 9}, {7, 12}, {0, len(full)}, {len(full) - 2, len(full)}, {5, 5}, {-3, 4}} {
		got := string(b.Slice(c.lo, c.hi))
		lo, hi := max(c.lo, 0), min(c.hi, len(full))
		want := ""
		if hi > lo {
			want = string(full[lo:hi])
		}
		if got != want {
			t.Errorf("Slice(%d,%d) = %q, want %q", c.lo, c.hi, got, want)
		}
	}
	if s := b.Slice(0, 4); len(s) > 0 {
		s[0] = '!'
		if b.Slice(0, 1)[0] == '!' {
			t.Error("Slice aliases the buffer (must copy)")
		}
	}
}

// TestTextBufferRuneAtBefore checks RuneAt/RuneBefore decode correctly (incl. multibyte and
// reading just past the gap) and report size 0 at the ends. (#68)
func TestTextBufferRuneAtBefore(t *testing.T) {
	b := NewTextBuffer([]byte("aé中z")) // a@0(1) é@1(2) 中@3(3) z@6(1), Len=7
	if r, n := b.RuneAt(1); r != 'é' || n != 2 {
		t.Errorf("RuneAt(1) = %q,%d want é,2", r, n)
	}
	if r, n := b.RuneAt(3); r != '中' || n != 3 {
		t.Errorf("RuneAt(3) = %q,%d want 中,3", r, n)
	}
	if _, n := b.RuneAt(7); n != 0 {
		t.Errorf("RuneAt(Len) size = %d, want 0", n)
	}
	if r, n := b.RuneBefore(3); r != 'é' || n != 2 { // rune ending at 3 is é (1..3)
		t.Errorf("RuneBefore(3) = %q,%d want é,2", r, n)
	}
	if r, n := b.RuneBefore(6); r != '中' || n != 3 { // 中 spans 3..6
		t.Errorf("RuneBefore(6) = %q,%d want 中,3", r, n)
	}
	if _, n := b.RuneBefore(0); n != 0 {
		t.Errorf("RuneBefore(0) size = %d, want 0", n)
	}
	// Move the gap so a decode reads from the post-gap region.
	b.Insert(1, []byte("XY")) // "aXYé中z", gap at 3; é now at offset 3
	if r, n := b.RuneAt(3); r != 'é' || n != 2 {
		t.Errorf("post-gap RuneAt(3) = %q,%d want é,2", r, n)
	}
}

// TestTextBufferRuneAtBeforeNoAlloc is the #68 guard: decoding one rune through the gap
// allocates nothing (a stack buffer), unlike the old caret-step path that copied the whole
// buffer via Bytes(). RuneAt/RuneBefore must be zero-alloc regardless of buffer size.
func TestTextBufferRuneAtBeforeNoAlloc(t *testing.T) {
	b := NewTextBuffer([]byte(strings.Repeat("héllo wörld ", 50000))) // ~600 KB
	mid := b.Len() / 2
	if a := testing.AllocsPerRun(200, func() { b.RuneAt(mid); b.RuneBefore(mid) }); a != 0 {
		t.Errorf("RuneAt/RuneBefore allocate %.0f/run, want 0 (decode through the gap, no copy)", a)
	}
}

// TestTextBufferSliceAllocBoundedBySpan is the #68 guard: Slice copies only the requested
// span, so a tiny slice of a large buffer allocates far less than a whole-buffer Bytes().
func TestTextBufferSliceAllocBoundedBySpan(t *testing.T) {
	b := NewTextBuffer(make([]byte, 1<<20)) // 1 MiB
	bytesAlloc := func(fn func()) uint64 {
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		fn()
		runtime.ReadMemStats(&m1)
		return m1.TotalAlloc - m0.TotalAlloc
	}
	small := bytesAlloc(func() { _ = b.Slice(10, 30) }) // 20 bytes
	whole := bytesAlloc(func() { _ = b.Bytes() })       // ~1 MiB
	if small >= whole/100 {
		t.Errorf("Slice(20B) allocated %d B vs Bytes() %d B — Slice must be O(span), far smaller", small, whole)
	}
}
