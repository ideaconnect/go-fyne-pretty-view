package prettyview

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// This file serializes a structured Document back into pretty-printed BYTES, for the v2
// buffer-rewriting Reformat (the editor prettifies the edit buffer in place rather than
// swapping to a separate display projection). The bytes are the same text the viewer would
// render: each display line, indented two spaces per nesting depth (the convention Text()
// and subtreeText() already use — the live indentation is a pixel offset, never in the
// segment bytes), with its expanded segment text, joined by newlines.

// reformatIndentUnit is the spaces-per-depth-level used when serializing a Document back to
// pretty text. Two spaces matches Text()/subtreeText and json.MarshalIndent's common style.
const reformatIndentUnit = 2

// srcSpan maps a byte range of the OLD buffer (a Document's Src) to its position in the
// freshly serialized pretty output, so the caret's byte offset can be remapped exactly.
type srcSpan struct {
	oldStart, oldEnd, newStart int
}

// serializePretty renders d to pretty-printed bytes and, for every source-backed (BufSrc)
// segment, an srcSpan from its old Src range to its new output range. Synthesized (BufAux)
// segments — structural punctuation, summaries, indentation — have no source range and
// contribute no span, so a caret in synthesized text remaps to the nearest token boundary.
// Spans come out ascending (lines and segments are emitted in source order), which
// remapCaretOffset relies on.
func serializePretty(d *model.Document) (out []byte, spans []srcSpan) {
	out = make([]byte, 0, len(d.Src)+len(d.Src)/4+16)
	spans = make([]srcSpan, 0, len(d.Segs)) // one span per source-backed segment, at most (#77)
	// Same pretty-line routine as Text/copy-subtree (model.AppendPrettyLine), indented by
	// absolute depth and newline-joined, plus a span callback that records each source-backed
	// segment's old→new byte range for the caret remap.
	for li := 0; li < len(d.Lines); li++ {
		if li > 0 {
			out = append(out, '\n')
		}
		out = d.AppendPrettyLine(int32(li), int(d.Lines[li].Depth)*reformatIndentUnit, out,
			func(srcStart, srcEnd uint32, outStart int) {
				spans = append(spans, srcSpan{oldStart: int(srcStart), oldEnd: int(srcEnd), newStart: outStart})
			})
	}
	return out, spans
}

// remapCaretOffset maps a caret byte offset in the OLD buffer to the equivalent offset in
// the serialized pretty output, using the source spans serializePretty produced. An offset
// inside a token maps exactly (same rune); an offset in removed whitespace or synthesized
// punctuation clamps to the nearest token boundary; with no spans (e.g. XML/HTML, whose
// tokens carry no source offsets) the caret falls to the start of the output. The result
// is always clamped into [0, newLen].
func remapCaretOffset(spans []srcSpan, oldOff, newLen int) int {
	if len(spans) == 0 {
		return 0
	}
	// Spans are ascending and non-overlapping, so binary-search the last span starting at or
	// before oldOff instead of scanning all of them (#77 — there is one span per token, so a
	// multi-MB reformat has millions).
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if spans[mid].oldStart <= oldOff {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	idx := lo - 1
	if idx < 0 { // oldOff is before the first token
		return clamp(spans[0].newStart, 0, newLen)
	}
	s := spans[idx]
	if oldOff < s.oldEnd { // inside this token: exact same rune
		return clamp(s.newStart+(oldOff-s.oldStart), 0, newLen)
	}
	// at/after the token's end (removed whitespace or synthesized punctuation): clamp to its end
	return clamp(s.newStart+(s.oldEnd-s.oldStart), 0, newLen)
}
