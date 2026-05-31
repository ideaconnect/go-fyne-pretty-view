package prettyview

import "math"

// metrics holds the integer-rounded layout measurements that map between model
// positions and content-space pixels. charWidth and rowH are rounded to whole
// pixels (matching widget.TextGrid) so selection rectangles align with glyphs on
// long lines and never drift sub-pixel. All x contributions are integral too, so
// a column's left edge is always an exact multiple away from the text origin.
//
// There is exactly one coordinate convention, used by the renderer, the
// hit-test, and the selection/match rectangle builders alike:
//
//	rowY(row)        = row * rowH                       (top padding is zero)
//	textOriginX(d)   = leftPad + triangleSlot + d*step  (where a line's text begins)
//	colX(d, col)     = textOriginX(d) + col*charWidth   (a column's left edge)
//
// Fold triangles live in the gutter at [textOriginX-triangleSlot, textOriginX],
// so text always aligns at textOriginX regardless of whether a row is foldable.
//
// These are CONTENT-space coordinates. The enclosing container.Scroll translates
// content by -Offset on both axes, so nothing here ever adds or subtracts the
// scroll offset; callers convert a viewport pixel to content space once, by
// adding Offset, before calling into this file.
type metrics struct {
	charWidth    float32
	rowH         float32
	textSize     float32 // font size the cell was measured at
	textH        float32 // measured glyph height (for vertical centering in a row)
	leftPad      float32
	triangleSlot float32
	indentStep   float32
	tabWidth     int
}

// textY centers a line of text vertically within a row.
func (m metrics) textY() float32 { return roundf((m.rowH - m.textH) / 2) }

func roundf(x float32) float32 { return float32(math.Round(float64(x))) }

// newMetrics builds metrics from a measured monospace cell size (the advance
// width of one glyph and the glyph height) and the config.
func newMetrics(cfg config, charWidth, glyphH float32) metrics {
	cw := roundf(charWidth)
	if cw < 1 {
		cw = 1
	}
	rh := roundf(glyphH)
	if rh < 1 {
		rh = 1
	}
	step := roundf(cfg.indentStep)
	if step < 1 {
		step = 1
	}
	tw := cfg.tabWidth
	if tw < 1 {
		tw = 4
	}
	return metrics{
		charWidth:    cw,
		rowH:         rh + 4, // a little vertical breathing room
		textH:        rh,
		leftPad:      6,
		triangleSlot: roundf(cw * 1.4),
		indentStep:   step,
		tabWidth:     tw,
	}
}

func (m metrics) textOriginX(depth uint8) float32 {
	return m.leftPad + m.triangleSlot + float32(depth)*m.indentStep
}

// triangleX returns the left edge of the fold-triangle gutter for a depth.
func (m metrics) triangleX(depth uint8) float32 {
	return m.textOriginX(depth) - m.triangleSlot
}

// colX returns the content-space left edge of a column on a line of given depth.
func (m metrics) colX(depth uint8, col int) float32 {
	return m.textOriginX(depth) + float32(col)*m.charWidth
}

// colAtX maps a content-space x to a rune column using half-glyph rounding.
func (m metrics) colAtX(depth uint8, x float32) int {
	rel := x - m.textOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Round(float64(rel / m.charWidth)))
}

func (m metrics) rowY(row int) float32 { return float32(row) * m.rowH }
func (m metrics) rowAtY(y float32) int { return int(math.Floor(float64(y / m.rowH))) }

// firstVisibleCol / lastVisibleCol bound the columns intersecting a horizontal
// window [x0, x1) (viewport in content space) for a line of given depth. Used by
// the renderer to cull text to the visible column range (invariant M-2).
func (m metrics) firstVisibleCol(depth uint8, x0 float32) int {
	rel := x0 - m.textOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Floor(float64(rel / m.charWidth)))
}

func (m metrics) lastVisibleCol(depth uint8, x1 float32) int {
	rel := x1 - m.textOriginX(depth)
	if rel <= 0 {
		return 0
	}
	return int(math.Ceil(float64(rel / m.charWidth)))
}

// modelPos is a position in the document: a stable display-line index and a rune
// column into that line's displayed text. Line indices never change after parse
// (only visibility does), so a modelPos survives folding; a hidden line is
// snapped to the nearest visible ancestor at use sites.
type modelPos struct {
	line int32
	col  int
}

// hitTest maps a content-space pixel to a model position. Out-of-range rows clamp
// to the document's start/end (mirroring Fyne's own entry/selectable behavior).
func (d *Document) hitTest(m metrics, contentX, contentY float32) modelPos {
	total := d.fold.TotalVisibleRows()
	if total == 0 {
		return modelPos{line: -1}
	}
	row := m.rowAtY(contentY)
	if row < 0 {
		row = 0
	}
	if int32(row) >= total {
		li := d.fold.lineAtRow(total - 1)
		return modelPos{line: li, col: d.lineRuneLen(li)}
	}
	li := d.fold.lineAtRow(int32(row))
	col := m.colAtX(d.Lines[li].Depth, contentX)
	if n := d.lineRuneLen(li); col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return modelPos{line: li, col: col}
}

// cellOrigin returns the content-space top-left pixel of (line, col): the inverse
// of hitTest at a column's left edge.
func (d *Document) cellOrigin(m metrics, line int32, col int) (float32, float32) {
	row := d.fold.rowOfLine(line)
	return m.colX(d.Lines[line].Depth, col), m.rowY(int(row))
}
