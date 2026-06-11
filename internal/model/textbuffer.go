package model

import "unicode/utf8"

// TextBuffer is the mutable source-of-truth for v2 edit mode (docs/DESIGN.md §12).
// It is a classic gap buffer over a single []byte: the bytes before the caret live
// in buf[:gapStart], the bytes after it in buf[gapEnd:], and the slack in between
// (the "gap") absorbs the next insert without shifting the tail. Edits while typing
// are caret-local, so the gap sits where the work is and most keystrokes are O(1).
//
// It is deliberately separate from Document. Document stays build-once-immutable with
// zero-copy SrcSegs aliasing its own Src (invariant 3); editing never mutates a
// Document. Instead the buffer accumulates edits and, on a debounced pause, hands a
// fresh contiguous snapshot (Bytes) to the parser, which builds a brand-new immutable
// Document that aliases that snapshot — not this buffer. See DECISION V2-1/V2-2.
//
// Positions: byte offsets are into the logical content (0..Len), not into buf.
// Columns are rune indices within a line, matching the model's (line, col) convention.
type TextBuffer struct {
	buf      []byte
	gapStart int // logical content is buf[:gapStart] + buf[gapEnd:]
	gapEnd   int
}

// minGap is the smallest slack the buffer keeps after a grow, so a run of single-rune
// inserts at one spot doesn't reallocate on every keystroke.
const minGap = 64

// NewTextBuffer returns a buffer seeded with a copy of src. The bytes are copied into
// the buffer's own backing array, so the buffer never aliases src (callers may pass a
// Document.Src that must not be mutated).
func NewTextBuffer(src []byte) *TextBuffer {
	buf := make([]byte, len(src)+minGap)
	copy(buf, src)
	return &TextBuffer{buf: buf, gapStart: len(src), gapEnd: len(buf)}
}

// Len is the logical byte length of the content (excluding the gap).
func (b *TextBuffer) Len() int { return b.gapStart + (len(b.buf) - b.gapEnd) }

// Bytes returns a fresh contiguous copy of the content with the gap collapsed. The
// result aliases nothing in the buffer, so a later edit never mutates a snapshot a
// parser already consumed (the SrcSegs alias-safety the whole design rests on).
func (b *TextBuffer) Bytes() []byte {
	out := make([]byte, b.Len())
	n := copy(out, b.buf[:b.gapStart])
	copy(out[n:], b.buf[b.gapEnd:])
	return out
}

// at returns the logical byte at offset i (0 <= i < Len). It is the only place that
// knows the gap exists; the index helpers below read content exclusively through it.
func (b *TextBuffer) at(i int) byte {
	if i < b.gapStart {
		return b.buf[i]
	}
	return b.buf[i-b.gapStart+b.gapEnd]
}

// Insert writes s at logical byte offset off (clamped to [0, Len]). The gap moves to
// off first, so a run of inserts at the caret stays local.
func (b *TextBuffer) Insert(off int, s []byte) {
	if len(s) == 0 {
		return
	}
	off = clampOff(off, b.Len())
	b.moveGap(off)
	b.ensureGap(len(s))
	copy(b.buf[b.gapStart:], s)
	b.gapStart += len(s)
}

// Delete removes n logical bytes starting at off (both clamped to the content). It is
// the caller's job to pass rune-aligned offsets; deleting a partial rune yields
// invalid UTF-8 that the tolerant parser will fold into the raw fallback.
func (b *TextBuffer) Delete(off, n int) {
	if n <= 0 {
		return
	}
	off = clampOff(off, b.Len())
	if off+n > b.Len() {
		n = b.Len() - off
	}
	b.moveGap(off)
	b.gapEnd += n // the n bytes just past the gap fall into it and are dropped
}

// moveGap repositions the gap so it begins at logical offset to, shifting the bytes it
// passes over to the other side. Go's copy is memmove-safe under overlap.
func (b *TextBuffer) moveGap(to int) {
	switch {
	case to < b.gapStart:
		n := b.gapStart - to
		copy(b.buf[b.gapEnd-n:b.gapEnd], b.buf[to:b.gapStart])
		b.gapStart -= n
		b.gapEnd -= n
	case to > b.gapStart:
		n := to - b.gapStart
		copy(b.buf[b.gapStart:b.gapStart+n], b.buf[b.gapEnd:b.gapEnd+n])
		b.gapStart += n
		b.gapEnd += n
	}
}

// ensureGap guarantees at least n free bytes in the gap, reallocating with generous
// slack (so inserts amortize) when the current gap is too small.
func (b *TextBuffer) ensureGap(n int) {
	if b.gapEnd-b.gapStart >= n {
		return
	}
	postLen := len(b.buf) - b.gapEnd
	contentLen := b.gapStart + postLen
	newGap := n + contentLen/2 + minGap
	nb := make([]byte, contentLen+newGap)
	copy(nb[:b.gapStart], b.buf[:b.gapStart])
	copy(nb[len(nb)-postLen:], b.buf[b.gapEnd:])
	b.buf = nb
	b.gapEnd = len(nb) - postLen
}

// LineColAt converts a logical byte offset to a 0-based (line, col) position, where
// line counts preceding newlines and col is the rune index within the line. Multi-byte
// runes count as one column. The offset is expected to be rune-aligned.
func (b *TextBuffer) LineColAt(byteOff int) (line, col int) {
	byteOff = clampOff(byteOff, b.Len())
	lineStart := 0
	for i := 0; i < byteOff; i++ {
		if b.at(i) == '\n' {
			line++
			lineStart = i + 1
		}
	}
	for i := lineStart; i < byteOff; {
		_, size := b.decodeRune(i)
		i += size
		col++
	}
	return line, col
}

// ByteOffAt is the inverse of LineColAt: it returns the logical byte offset at the
// start of the col-th rune of the given line. A col past the line's end clamps to the
// newline (or end of buffer); a line past the end clamps to Len.
func (b *TextBuffer) ByteOffAt(line, col int) int {
	i, n := 0, b.Len()
	for line > 0 && i < n {
		if b.at(i) == '\n' {
			line--
		}
		i++
	}
	for c := 0; c < col && i < n && b.at(i) != '\n'; c++ {
		_, size := b.decodeRune(i)
		i += size
	}
	return i
}

// decodeRune decodes the rune beginning at logical offset i, reading up to four bytes
// through the gap. size is always >= 1 so scans make progress on invalid UTF-8.
func (b *TextBuffer) decodeRune(i int) (r rune, size int) {
	var tmp [utf8.UTFMax]byte
	n := 0
	for n < len(tmp) && i+n < b.Len() {
		tmp[n] = b.at(i + n)
		n++
	}
	r, size = utf8.DecodeRune(tmp[:n])
	if size <= 0 {
		size = 1
	}
	return r, size
}

func clampOff(off, n int) int {
	if off < 0 {
		return 0
	}
	if off > n {
		return n
	}
	return off
}
