package parse

import (
	"cmp"
	"slices"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// This file is the host colorizer extension point: a host that knows a line-oriented grammar
// the library does not (INI-style config, share URIs, log lines, ...) supplies a function that
// reports color spans over the raw bytes, and FormatRaw text is colored with them in both the
// read-only raw projection and the editable colorized-raw projection. The host owns the
// grammar; the library owns the layout: spans only ever pick a color role for bytes that are
// already on screen, so they can never change a line's text, its rune count, or the 1:1
// display<->buffer mapping the editor's caret depends on.

// Colorizer reports color spans over src, APPENDING them to dst and returning the result
// (the strconv.Append* idiom, so a steady-state call can reuse the previous call's backing
// array instead of allocating). Offsets are byte offsets into src. src must not be retained
// or mutated.
type Colorizer = func(dst []model.ColorSpan, src []byte) []model.ColorSpan

// hostColor holds a host colorizer and the two scratch slices it reuses across calls: the
// host's raw output (handed back as the next call's dst) and the sanitized internal spans.
type hostColor struct {
	fn    Colorizer
	raw   []model.ColorSpan
	spans []colorSpan
}

// newHostColor wraps c, or returns nil when c is nil (no host colorizer).
func newHostColor(c Colorizer) *hostColor {
	if c == nil {
		return nil
	}
	return &hostColor{fn: c}
}

// spansFor runs the host colorizer over src and returns its spans sanitized for the
// segment builders. The result is valid until the next call.
func (h *hostColor) spansFor(src []byte) []colorSpan {
	h.raw = h.call(src)
	h.spans = sanitizeSpans(h.spans[:0], h.raw, len(src))
	return h.spans
}

// call invokes the host colorizer, degrading a panic to "no spans" (monochrome text) so a
// buggy host grammar can never take the widget down, on load or on any keystroke.
func (h *hostColor) call(src []byte) (out []model.ColorSpan) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	return h.fn(h.raw[:0], src)
}

// sanitizeSpans turns arbitrary host spans into the ascending, non-overlapping, in-range
// form the segment builders require, appending to dst. raw is sorted in place (stably, by
// Start) only if it is not already ordered. Each span is clamped to [0, n); an empty or
// inverted span is dropped; overlaps resolve first-wins (a span is trimmed to start where
// the previous kept span ended, so one nested inside an earlier span disappears); a role
// outside the palette becomes RolePlain. Rune boundaries are the segment builders' job:
// they only change role at the start of a rune.
func sanitizeSpans(dst []colorSpan, raw []model.ColorSpan, n int) []colorSpan {
	if !slices.IsSortedFunc(raw, byStart) {
		slices.SortStableFunc(raw, byStart)
	}
	prevEnd := 0
	for _, s := range raw {
		start := max(s.Start, prevEnd)
		end := min(s.End, n)
		if end <= start {
			continue
		}
		role := s.Role
		if role >= model.NumColorRoles {
			role = model.RolePlain
		}
		dst = append(dst, colorSpan{start, end, role})
		prevEnd = end
	}
	return dst
}

func byStart(a, b model.ColorSpan) int { return cmp.Compare(a.Start, b.Start) }

// ParseWithColorizer is Parse plus an optional host colorizer for raw text: when c is
// non-nil and the format (after auto-detection) has no structured parser, src is split into
// raw lines exactly as Parse would (tabs expanded to tabWidth, a trailing CR trimmed) and
// colored by c's spans. Any other case is exactly Parse. A structured format whose parse
// fails still falls back to plain raw: the host grammar is not applied to text the host did
// not declare raw.
func ParseWithColorizer(src []byte, format Format, collapseDepth, tabWidth int, c Colorizer) *model.Document {
	if c == nil {
		return Parse(src, format, collapseDepth, tabWidth)
	}
	src = stripBOM(src)
	if format == FormatAuto {
		format = AutoDetect(src)
	}
	if parserFor(format) != nil {
		return Parse(src, format, collapseDepth, tabWidth)
	}
	src = clampEditableSrc(src) // the same uint32 offset ceiling Parse applies
	if tabWidth < 1 {
		tabWidth = 4
	}
	spans := newHostColor(c).spansFor(src)
	b := model.NewBuilder(src, FormatRaw, collapseDepth)
	var scratch []model.Seg
	si := 0
	forEachLine(src, false, func(start, end int) {
		scratch = rawColorLineSegsInto(scratch[:0], src, start, end, tabWidth, spans, &si)
		b.Leaf(model.KindRawLine, start, end, scratch)
	})
	return b.Finish()
}

// rawColorLineSegsInto is rawLineSegs with color: the read-only raw line src[start:end]
// (trailing CR trimmed, tabs expanded to the next tab stop) split into runs that share a
// span role, appended to dst. Roles are sampled once per rune, at its first byte, so a span
// boundary inside a multi-byte rune snaps to the next rune start instead of splitting it.
// A line longer than maxColorLineBytes stays monochrome, like the editable colorizer, so a
// pathological line can never approach the per-line segment-count limit. si is the
// caller's running cursor into spans (lines are walked in order).
func rawColorLineSegsInto(dst []model.Seg, src []byte, start, end, tabWidth int, spans []colorSpan, si *int) []model.Seg {
	segEnd := end
	if segEnd > start && src[segEnd-1] == '\r' {
		segEnd--
	}
	if len(spans) == 0 || segEnd-start > maxColorLineBytes {
		return append(dst, rawLineSegs(src, start, end, tabWidth)...)
	}
	col := 0
	runStart := start
	var runRole model.ColorRole
	haveRun := false
	flush := func(to int) {
		if haveRun && to > runStart {
			dst = append(dst, model.SrcSeg(runRole, runStart, to))
		}
		haveRun = false
	}
	for i := start; i < segEnd; {
		for *si < len(spans) && spans[*si].end <= i {
			*si++
		}
		role := model.RolePlain
		if *si < len(spans) && spans[*si].start <= i {
			role = spans[*si].role
		}
		if src[i] == '\t' {
			flush(i)
			n := tabWidth - col%tabWidth
			dst = append(dst, model.LitSeg(role, tabPad(n)))
			col += n
			i++
			continue
		}
		if !haveRun || role != runRole {
			flush(i)
			runStart, runRole, haveRun = i, role, true
		}
		_, sz := utf8.DecodeRune(src[i:])
		i += sz
		col++ // one display cell per rune (uniform monospace grid)
	}
	flush(segEnd)
	if len(dst) == 0 {
		dst = append(dst, model.SrcSeg(model.RolePlain, start, segEnd)) // empty line
	}
	return dst
}
