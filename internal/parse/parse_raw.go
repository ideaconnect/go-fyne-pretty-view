package parse

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// rawParser is the universal fallback: it splits the source into physical lines,
// each rendered as a single plain, non-foldable row. It is also used whenever a
// structured parse fails, so malformed input still displays. tabWidth expands tabs
// to spaces so the uniform monospace grid (hit-test, selection, rendering) holds.
type rawParser struct{ tabWidth int }

func (rawParser) Format() Format { return FormatRaw }

// Detect never claims raw during auto-detection; raw is the floor chosen by
// AutoDetect only when no structured parser is confident.
func (rawParser) Detect([]byte) int { return 0 }

func (p rawParser) Parse(src []byte, b *model.Builder) error {
	start := 0
	for start <= len(src) {
		nl := bytes.IndexByte(src[start:], '\n')
		if nl < 0 {
			if start < len(src) {
				b.Leaf(model.KindRawLine, start, len(src), rawLineSegs(src, start, len(src), p.tabWidth))
			}
			break
		}
		end := start + nl
		b.Leaf(model.KindRawLine, start, end, rawLineSegs(src, start, end, p.tabWidth))
		start = end + 1
	}
	return nil
}

// rawLineSegs builds the segments for one raw line, trimming a trailing carriage
// return and expanding tabs to the next tab stop. The common (tab-free) line stays
// a single zero-copy segment; a line with tabs keeps its non-tab runs zero-copy and
// emits an interned space run (deduped in Aux) for each tab.
func rawLineSegs(src []byte, start, end, tabWidth int) []model.Seg {
	segEnd := end
	if segEnd > start && src[segEnd-1] == '\r' {
		segEnd--
	}
	if bytes.IndexByte(src[start:segEnd], '\t') < 0 {
		return []model.Seg{model.SrcSeg(model.RolePlain, start, segEnd)} // no tabs (incl. empty line)
	}
	if tabWidth < 1 {
		tabWidth = 4
	}
	var segs []model.Seg
	col := 0
	runStart := start
	for i := start; i < segEnd; {
		if src[i] == '\t' {
			if i > runStart {
				segs = append(segs, model.SrcSeg(model.RolePlain, runStart, i))
			}
			n := tabWidth - col%tabWidth
			segs = append(segs, model.LitSeg(model.RolePlain, tabPad(n)))
			col += n
			i++
			runStart = i
			continue
		}
		_, sz := utf8.DecodeRune(src[i:])
		i += sz
		col++ // one display cell per rune (uniform monospace grid)
	}
	if segEnd > runStart {
		segs = append(segs, model.SrcSeg(model.RolePlain, runStart, segEnd))
	}
	return segs
}

const tabPadStr = "                                " // 32 spaces

// tabPad returns n spaces (n is 1..tabWidth). The string slice is allocation-free
// for the common case; the builder interns it so repeats cost nothing in Aux.
func tabPad(n int) string {
	if n <= len(tabPadStr) {
		return tabPadStr[:n]
	}
	return strings.Repeat(" ", n)
}

// parseRaw builds a raw document directly (used as a parse-failure fallback).
func parseRaw(src []byte, collapseDepth, tabWidth int) *model.Document {
	b := model.NewBuilder(src, FormatRaw, collapseDepth)
	_ = rawParser{tabWidth: tabWidth}.Parse(src, b)
	return b.Finish()
}

// ctlPlaceholder is the single display rune shown for one grid-hostile byte in the
// edit-raw projection (one cell, never a control character).
const ctlPlaceholder = "·"

// editRawLineSegsInto renders src[start:end] for edit mode, APPENDING into dst (pass dst[:0]
// to reuse a scratch slice across lines — #84): zero-copy SrcSegs for runs of clean bytes,
// and a one-rune placeholder LitSeg for each grid-hostile byte. The total display-rune count
// equals the buffer-rune count, so the caret stays a direct buffer position. It is the
// monochrome fallback for the edit-mode colorizer's long lines (parse_editcolor.go); short
// lines get per-token colors there instead.
func editRawLineSegsInto(dst []model.Seg, src []byte, start, end int) []model.Seg {
	if !hasGridBreaker(src[start:end]) {
		return append(dst, model.SrcSeg(model.RolePlain, start, end)) // clean (incl. empty) line
	}
	runStart := start
	for i := start; i < end; i++ {
		if isGridHostile(src[i]) {
			if i > runStart {
				dst = append(dst, model.SrcSeg(model.RolePlain, runStart, i))
			}
			dst = append(dst, model.LitSeg(model.RolePlain, ctlPlaceholder))
			runStart = i + 1
		}
	}
	if end > runStart {
		dst = append(dst, model.SrcSeg(model.RolePlain, runStart, end))
	}
	return dst
}
