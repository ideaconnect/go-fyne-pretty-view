// Package geometry holds the integer-rounded layout math that maps between model
// positions (line, rune column) and content-space pixels. It is a leaf: it
// depends only on internal/model and the standard library, with no Fyne or view
// state, so the coordinate convention lives in exactly one testable place.
//
// There is exactly one convention, used by the renderer, the hit-test, and the
// selection/match rectangle builders alike:
//
//	RowY(row)        = row * RowH                       (top padding is zero)
//	TextOriginX(d)   = leftPad + triangleSlot + d*step  (where a line's text begins)
//	ColX(d, col)     = TextOriginX(d) + col*CharWidth   (a column's left edge)
//
// Fold triangles live in the gutter at [TextOriginX-triangleSlot, TextOriginX],
// so text always aligns at TextOriginX regardless of whether a row is foldable.
//
// These are CONTENT-space coordinates. The enclosing container.Scroll translates
// content by -Offset on both axes, so nothing here ever adds or subtracts the
// scroll offset; callers convert a viewport pixel to content space once, by
// adding Offset, before calling in.
//
// Precision ceiling. Content X is a float32 (Fyne canvas coordinates are float32
// throughout), whose mantissa represents integers exactly only up to 2^24 ≈
// 16.7M. A column's pixel x is col*CharWidth, so beyond ~16.7M/CharWidth runes
// (on the order of a million-plus characters on a single physical line) the
// col<->pixel mapping loses 1px resolution and character-exact selection on that
// one line may drift by a glyph. This bounds only selection precision on
// pathologically long single lines; the row axis and all normal content are
// unaffected.
package geometry

import (
	"math"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// Metrics holds the measured, integer-rounded cell layout. CharWidth and RowH are
// rounded to whole pixels (matching widget.TextGrid) so selection rectangles
// align with glyphs on long lines and never drift sub-pixel.
type Metrics struct {
	CharWidth float32
	RowH      float32
	TextSize  float32 // font size the cell was measured at

	textH        float32 // measured glyph height (for vertical centering)
	leftPad      float32
	triangleSlot float32
	indentStep   float32
}

func roundf(x float32) float32 { return float32(math.Round(float64(x))) }

// NewMetrics builds Metrics from a measured monospace cell (the advance width of
// one glyph and the glyph height) and the indent step. Tabs are expanded to spaces
// at parse time, so the layout grid is uniformly one CharWidth per column.
func NewMetrics(charWidth, glyphH, indentStep float32) Metrics {
	cw := roundf(charWidth)
	if cw < 1 {
		cw = 1
	}
	rh := roundf(glyphH)
	if rh < 1 {
		rh = 1
	}
	step := roundf(indentStep)
	if step < 1 {
		step = 1
	}
	return Metrics{
		CharWidth:    cw,
		RowH:         rh + 4, // a little vertical breathing room
		textH:        rh,
		leftPad:      6,
		triangleSlot: roundf(cw * 1.4),
		indentStep:   step,
	}
}

// TextY centers a line of text vertically within a row.
func (m Metrics) TextY() float32 { return roundf((m.RowH - m.textH) / 2) }

// TextOriginX is the content-space x where a line's text begins at the given depth.
func (m Metrics) TextOriginX(depth uint8) float32 {
	return m.leftPad + m.triangleSlot + float32(depth)*m.indentStep
}

// TriangleX is the left edge of the fold-triangle gutter for a depth.
func (m Metrics) TriangleX(depth uint8) float32 {
	return m.TextOriginX(depth) - m.triangleSlot
}

// ColX is the content-space left edge of a column on a line of given depth.
func (m Metrics) ColX(depth uint8, col int) float32 {
	return m.TextOriginX(depth) + float32(col)*m.CharWidth
}

// minWrapCols floors a line's soft-wrap budget: even at an indentation so deep that
// almost no width remains, a line wraps to at least this many columns per visual
// row rather than degenerating to one rune per row.
const minWrapCols = 4

// ColsForDepth is the number of text columns available to soft-wrap a line at the
// given depth within a viewport of width viewportW (content space). It mirrors the
// renderer's content-width slack (CharWidth*2) so a wrapped row leaves the same
// right-edge breathing room a non-wrapped line has, and clamps to minWrapCols. The
// view passes the per-depth results into the model, which cannot import geometry.
func (m Metrics) ColsForDepth(depth uint8, viewportW float32) int {
	avail := viewportW - m.TextOriginX(depth) - m.CharWidth*2
	cols := int(math.Floor(float64(avail / m.CharWidth)))
	if cols < minWrapCols {
		cols = minWrapCols
	}
	return cols
}

// ColAtX maps a content-space x to a rune column using half-glyph rounding.
func (m Metrics) ColAtX(depth uint8, x float32) int {
	rel := x - m.TextOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Round(float64(rel / m.CharWidth)))
}

func (m Metrics) RowY(row int) float32 { return float32(row) * m.RowH }
func (m Metrics) RowAtY(y float32) int { return int(math.Floor(float64(y / m.RowH))) }

// FirstVisibleCol / LastVisibleCol bound the columns intersecting a horizontal
// window [x0, x1) (viewport in content space) for a line of given depth. Used by
// the renderer to cull text to the visible column range.
func (m Metrics) FirstVisibleCol(depth uint8, x0 float32) int {
	rel := x0 - m.TextOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Floor(float64(rel / m.CharWidth)))
}

func (m Metrics) LastVisibleCol(depth uint8, x1 float32) int {
	rel := x1 - m.TextOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Ceil(float64(rel / m.CharWidth)))
}

// HitTest maps a content-space pixel to a model position (display line + rune
// column). Out-of-range rows clamp to the document's start/end. A returned line
// of -1 means the document is empty. Under soft-wrap the clicked visual row is one
// sub-row of a wrapped line, so the column is offset by that sub-row's start.
func HitTest(d *model.Document, m Metrics, contentX, contentY float32) (line int32, col int) {
	total := d.TotalVisibleRows()
	if total == 0 {
		return -1, 0
	}
	row := m.RowAtY(contentY)
	if row < 0 {
		row = 0
	}
	if int32(row) >= total {
		// A click below all content resolves onto the last line, but the column
		// still honors contentX (clamped) rather than always snapping to the line
		// end — so a below-and-left click maps to a near-start column, not EOL.
		row = int(total - 1)
	}
	li, sub := d.LineAndSubRowAtRow(int32(row))
	local := m.ColAtX(d.Lines[li].Depth, contentX) // column offset within this (sub-)row's text
	if local < 0 {
		local = 0
	}
	col = local
	if d.WrapActive() {
		start, end := wrapRowSpan(d, li, sub)
		col = int(start) + local
		if col > int(end) {
			col = int(end) // a click past the row's text stays within the row, not the next one
		}
	}
	if n := d.LineRuneLen(li); col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return li, col
}

// CellOrigin returns the content-space top-left pixel of (line, col): the inverse
// of HitTest at a column's left edge. Under soft-wrap it resolves which sub-row
// holds col and offsets x by that sub-row's start column.
func CellOrigin(d *model.Document, m Metrics, line int32, col int) (x, y float32) {
	depth := d.Lines[line].Depth
	row := d.RowOfLine(line) // first (top) visual row of the line
	local := col
	if d.WrapActive() {
		breaks := d.WrapBreaks(line, nil)
		sub := 0
		for sub < len(breaks)-2 && col >= int(breaks[sub+1]) {
			sub++
		}
		local = col - int(breaks[sub])
		row += int32(sub)
	}
	return m.ColX(depth, local), m.RowY(int(row))
}

// wrapRowSpan returns the [start, end) displayed-rune-column range of sub-row sub
// of line li. Caller must have checked d.WrapActive().
func wrapRowSpan(d *model.Document, li, sub int32) (start, end int32) {
	breaks := d.WrapBreaks(li, nil) // [0, b1, …, lineLen]; row k spans [breaks[k], breaks[k+1])
	if sub < 0 {
		sub = 0
	}
	if int(sub) > len(breaks)-2 {
		sub = int32(len(breaks) - 2)
	}
	return breaks[sub], breaks[sub+1]
}
