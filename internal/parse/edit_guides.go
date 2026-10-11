package parse

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// This file derives indent-guide levels for the editable projection. The editable
// projection keeps indentation as literal leading whitespace (every line is a flat depth-0
// KindRawLine leaf, so display runes stay 1:1 with buffer runes), which means Line.Depth is
// always 0 there and cannot drive the guides the read-only view draws. Instead, a line's
// guide level is read from its leading whitespace in columns: in the editable projection a
// space and a tab are each exactly one display column (a tab renders as one placeholder rune,
// #62), so a guide at level i sits on display column i*unit.

// DefaultEditIndentUnit is the indent unit assumed when a buffer offers no evidence of its
// own (no line is indented deeper than the non-blank line before it). It matches the
// two-space indent Reformat writes.
const DefaultEditIndentUnit = 2

// maxEditIndentUnit is the widest step EditIndentUnit will detect. Deeper jumps between
// consecutive lines (continuation alignment, hand-aligned text) are ignored as noise.
const maxEditIndentUnit = 8

// maxEditGuideScan bounds how far EditGuideLevel walks over blank lines to find the
// non-blank neighbours a blank line inherits its guides from, so a pathological run of
// blank lines costs a bounded amount per rendered row. Past the bound the missing
// neighbour counts as level 0.
const maxEditGuideScan = 64

// maxIndentScanCols bounds the leading-whitespace scan of one line. A line indented
// further than this is treated as indented exactly this far (its guide level is capped
// by the caller long before that), so a megabyte of leading spaces is never walked.
const maxIndentScanCols = 1024

// LeadingIndentCols returns the number of leading space/tab columns of line li of an
// editable-projection document and whether the line is blank (empty or whitespace-only,
// a trailing carriage return included). Each space or tab counts as one column, matching
// the editable projection's one-rune-per-byte display of indentation.
func LeadingIndentCols(d *model.Document, li int32) (cols int, blank bool) {
	if li < 0 || int(li) >= len(d.Lines) {
		return 0, true
	}
	n := &d.Nodes[d.Lines[li].Owner]
	src := d.Src[n.SrcStart:n.SrcEnd]
	i := 0
	for i < len(src) && i < maxIndentScanCols && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if i == maxIndentScanCols {
		return i, false // pathologically deep: never walk the rest
	}
	rest := src[i:]
	blank = len(rest) == 0 || (len(rest) == 1 && rest[0] == '\r')
	return i, blank
}

// EditIndentUnit detects the indent unit of an editable-projection document: the most
// frequent positive step (1..8 columns) between the leading indentation of consecutive
// non-blank lines, ties going to the smaller step. A buffer with no such step returns
// DefaultEditIndentUnit. It walks only leading whitespace and uses a fixed-size
// histogram, so it allocates nothing.
func EditIndentUnit(d *model.Document) int {
	var hist [maxEditIndentUnit + 1]int
	prev := -1
	for li := range d.Lines {
		cols, blank := LeadingIndentCols(d, int32(li))
		if blank {
			continue
		}
		if prev >= 0 {
			if step := cols - prev; step > 0 && step <= maxEditIndentUnit {
				hist[step]++
			}
		}
		prev = cols
	}
	unit, best := DefaultEditIndentUnit, 0
	for step := 1; step <= maxEditIndentUnit; step++ {
		if hist[step] > best {
			unit, best = step, hist[step]
		}
	}
	return unit
}

// EditGuideLevel returns how many indent guides line li of an editable-projection document
// shows, given the document's indent unit: floor(indent/unit) for a non-blank line. A blank
// line continues the guides of the block it sits in, the convention most code editors use:
// with up/down the levels of the nearest non-blank lines above and below, it gets up when
// they are equal and min(up, down)+1 when they differ (so a blank line just inside a block,
// or just before its closing line, keeps the block's guide instead of breaking it).
func EditGuideLevel(d *model.Document, li int32, unit int) int {
	if unit < 1 {
		unit = 1
	}
	cols, blank := LeadingIndentCols(d, li)
	if !blank {
		return cols / unit
	}
	up := neighbourLevel(d, li, -1, unit)
	down := neighbourLevel(d, li, +1, unit)
	if up == down {
		return up
	}
	return min(up, down) + 1
}

// neighbourLevel is the guide level of the nearest non-blank line from li in direction dir,
// or 0 when there is none within maxEditGuideScan lines.
func neighbourLevel(d *model.Document, li int32, dir int32, unit int) int {
	for k := int32(1); k <= maxEditGuideScan; k++ {
		j := li + dir*k
		if j < 0 || int(j) >= len(d.Lines) {
			return 0
		}
		if cols, blank := LeadingIndentCols(d, j); !blank {
			return cols / unit
		}
	}
	return 0
}
