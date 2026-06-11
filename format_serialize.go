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
	for li := 0; li < len(d.Lines); li++ {
		if li > 0 {
			out = append(out, '\n')
		}
		for k := 0; k < int(d.Lines[li].Depth)*reformatIndentUnit; k++ {
			out = append(out, ' ')
		}
		for _, s := range d.LineSegs(int32(li)) {
			if s.Buf == model.BufSrc {
				spans = append(spans, srcSpan{
					oldStart: int(s.Start),
					oldEnd:   int(s.End),
					newStart: len(out),
				})
			}
			out = append(out, d.SegBytes(s)...)
		}
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
	for i, s := range spans {
		switch {
		case oldOff < s.oldStart: // before this token (only reachable at i == 0)
			return clampInt(s.newStart, 0, newLen)
		case oldOff < s.oldEnd: // inside this token: exact
			return clampInt(s.newStart+(oldOff-s.oldStart), 0, newLen)
		case i+1 >= len(spans) || oldOff < spans[i+1].oldStart: // at/after end, before the next
			return clampInt(s.newStart+(s.oldEnd-s.oldStart), 0, newLen)
		}
	}
	last := spans[len(spans)-1]
	return clampInt(last.newStart+(last.oldEnd-last.oldStart), 0, newLen)
}
