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
	// XML/HTML reserialization must re-encode the entities the parser decoded out of text and
	// attribute content, or a reformat would emit invalid markup that can't round-trip (e.g.
	// &amp; -> & on save — issue #81). The viewer's Text()/copy path keeps the decoded form,
	// so this escaping is reformat-only.
	markup := d.Format == FormatXML || d.Format == FormatHTML
	// Same pretty-line routine as Text/copy-subtree (model.AppendPrettyLine), indented by
	// absolute depth and newline-joined, plus a span callback that records each source-backed
	// segment's old→new byte range for the caret remap.
	for li := 0; li < len(d.Lines); li++ {
		if li > 0 {
			out = append(out, '\n')
		}
		if markup {
			out = appendMarkupPrettyLine(d, int32(li), out)
			continue
		}
		out = d.AppendPrettyLine(int32(li), int(d.Lines[li].Depth)*reformatIndentUnit, out,
			func(srcStart, srcEnd uint32, outStart int) {
				spans = append(spans, srcSpan{oldStart: int(srcStart), oldEnd: int(srcEnd), newStart: outStart})
			})
	}
	return out, spans
}

// appendMarkupPrettyLine serializes one display line of an XML/HTML document for Reformat,
// re-encoding the reserved characters the parser decoded out of text and attribute content so
// the rewritten buffer is VALID markup that round-trips (e.g. the model's decoded "&" is
// written back as "&amp;" — issue #81). It mirrors model.AppendPrettyLine's indent + segment
// walk but escapes RoleString segments; every other role (tag punctuation, names, comments,
// doctypes) is already in source form and is copied verbatim. Markup segments are interned
// literals (BufAux), so there are no source spans to record — the XML/HTML caret remap falls
// to the output start, exactly as before (see remapCaretOffset).
func appendMarkupPrettyLine(d *model.Document, li int32, buf []byte) []byte {
	owner := d.Lines[li].Owner
	// Opaque raw text — HTML <script>/<style> bodies and XML CDATA — is re-emitted from the
	// source bytes verbatim: no indent, no whitespace collapse, no entity escaping, so embedded
	// JS/CSS and CDATA round-trip and stay valid (#85). The display still shows the node's
	// collapsed segment; only this serialization path reads the raw span.
	if d.IsRawText(owner) {
		n := &d.Nodes[owner]
		return append(buf, d.Src[n.SrcStart:n.SrcEnd]...)
	}
	indent := int(d.Lines[li].Depth) * reformatIndentUnit
	for k := 0; k < indent; k++ {
		buf = append(buf, ' ')
	}
	// A line owned by an element node carries attribute values (each wrapped in its delimiter
	// quotes); any other line's RoleString segment is text content.
	attrLine := false
	if owner != model.NoNode && int(owner) < len(d.Nodes) {
		k := d.Nodes[owner].Kind
		attrLine = k == model.KindElement || k == model.KindEmptyElement
	}
	for _, s := range d.LineSegs(li) {
		b := d.SegBytes(s)
		if s.Role == model.RoleString {
			buf = appendEscapedMarkupSeg(buf, b, attrLine)
			continue
		}
		buf = append(buf, b...)
	}
	return buf
}

// appendEscapedMarkupSeg escapes one RoleString segment for markup reserialization. An
// attribute value arrives wrapped in its delimiter quotes ("val"); only the inner content is
// escaped (with a literal " becoming &quot; so it can't break the delimiter), the quotes stay.
// A text segment is escaped directly.
func appendEscapedMarkupSeg(buf, b []byte, attrLine bool) []byte {
	if attrLine && len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		buf = append(buf, '"')
		buf = appendEscapedMarkupContent(buf, b[1:len(b)-1], true)
		return append(buf, '"')
	}
	return appendEscapedMarkupContent(buf, b, false)
}

// appendEscapedMarkupContent re-encodes the characters that would otherwise produce invalid
// markup: a bare '&' and '<' in any content, plus a '"' inside an attribute value (its
// delimiter). '>' is left as-is — it is valid unescaped in both text and attribute values —
// so reformatting canonicalizes encodings as little as possible while always staying valid.
func appendEscapedMarkupContent(buf, b []byte, attrValue bool) []byte {
	for _, c := range b {
		switch {
		case c == '&':
			buf = append(buf, "&amp;"...)
		case c == '<':
			buf = append(buf, "&lt;"...)
		case c == '"' && attrValue:
			buf = append(buf, "&quot;"...)
		default:
			buf = append(buf, c)
		}
	}
	return buf
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
