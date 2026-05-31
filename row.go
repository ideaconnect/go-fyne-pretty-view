package prettyview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

const maxIndentGuides = 32

// rowWidget renders exactly one display line. It is the only object that ever
// holds document text, and only ~viewport-many of them exist at once (they are
// recycled by the renderer). A row positions its children in content space: the
// row itself is placed at content x = 0, so a glyph at column c sits at the
// absolute content x given by metrics.colX — the enclosing scroll then translates
// everything by the horizontal offset.
type rowWidget struct {
	widget.BaseWidget
	pv   *PrettyView
	line int32 // display-line index this row currently shows (-1 = unused)
	rr   *rowRenderer
}

func newRowWidget(pv *PrettyView) *rowWidget {
	r := &rowWidget{pv: pv, line: -1}
	r.ExtendBaseWidget(r)
	return r
}

// setLine binds the row to a display line and repaints it.
func (r *rowWidget) setLine(line int32) {
	r.line = line
	r.Refresh()
}

func (r *rowWidget) CreateRenderer() fyne.WidgetRenderer {
	if r.rr == nil {
		r.rr = &rowRenderer{row: r}
	}
	return r.rr
}

// maxTextWidth reports the widest single canvas.Text currently emitted by this
// row, used by the memory tests to verify long-line culling (invariant M-2).
func (r *rowWidget) maxTextWidth() float32 {
	if r.rr == nil {
		return 0
	}
	var w float32
	for _, t := range r.rr.texts {
		if t.Visible() {
			if tw := t.MinSize().Width; tw > w {
				w = tw
			}
		}
	}
	return w
}

type rowRenderer struct {
	row      *rowWidget
	guides   []*canvas.Line
	triangle *canvas.Text
	texts    []*canvas.Text
	objects  []fyne.CanvasObject
}

func (rr *rowRenderer) Destroy()                     {}
func (rr *rowRenderer) Objects() []fyne.CanvasObject { return rr.objects }
func (rr *rowRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, rr.row.pv.met.rowH)
}

// Layout is a no-op: children are positioned absolutely in content space by build.
func (rr *rowRenderer) Layout(fyne.Size) {}

func (rr *rowRenderer) Refresh() {
	rr.build()
	canvas.Refresh(rr.row)
}

// build (re)configures the row's pooled canvas objects for its current line,
// culling text to the visible column window so no single canvas.Text is ever
// wider than the viewport (invariant M-2).
func (rr *rowRenderer) build() {
	r := rr.row
	pv := r.pv
	rr.objects = rr.objects[:0]
	if r.line < 0 || pv.doc == nil || int(r.line) >= len(pv.doc.Lines) {
		rr.hideFrom(0)
		rr.triangleHide()
		rr.hideGuides(0)
		return
	}
	m := pv.met
	line := &pv.doc.Lines[r.line]
	depth := line.Depth

	// Indent guides: one subtle vertical rule per nesting level.
	rr.layoutGuides(depth, m, pv.guideColor)

	// Fold triangle in the gutter.
	if line.Fold != NoNode {
		collapsed := pv.doc.fold.collapsed.get(line.Fold)
		rr.layoutTriangle(depth, m, collapsed, pv.palette[RoleMuted])
	} else {
		rr.triangleHide()
	}

	// Colored, horizontally-culled text runs.
	firstCol := m.firstVisibleCol(depth, pv.viewOffX)
	lastCol := m.lastVisibleCol(depth, pv.viewOffX+pv.viewW)
	if lastCol <= firstCol {
		lastCol = firstCol + 1
	}
	hardCap := 2 * (lastCol - firstCol + 2)

	ti := 0
	col := 0
	emitted := 0
	for _, seg := range pv.doc.displaySegs(r.line) {
		runes := []rune(string(pv.doc.segBytes(seg)))
		segStart := col
		segEnd := col + len(runes)
		col = segEnd
		// Intersect [segStart,segEnd) with the visible column window.
		a, b := segStart, segEnd
		if a < firstCol {
			a = firstCol
		}
		if b > lastCol {
			b = lastCol
		}
		if a >= b {
			continue
		}
		text := string(runes[a-segStart : b-segStart])
		t := rr.text(ti)
		ti++
		t.Text = text
		t.TextSize = m.textSize
		t.TextStyle = fyne.TextStyle{Monospace: true}
		t.Color = pv.palette[seg.Role]
		t.Move(fyne.NewPos(m.colX(depth, a), m.textY()))
		t.Resize(t.MinSize())
		t.Show()
		emitted += b - a
		if emitted >= hardCap {
			break
		}
	}
	rr.hideFrom(ti)

	// Assemble objects: guides (lowest), triangle, then text (highest).
	for _, g := range rr.guides {
		if g.Visible() {
			rr.objects = append(rr.objects, g)
		}
	}
	if rr.triangle != nil && rr.triangle.Visible() {
		rr.objects = append(rr.objects, rr.triangle)
	}
	for i := 0; i < ti; i++ {
		rr.objects = append(rr.objects, rr.texts[i])
	}
}

func (rr *rowRenderer) layoutGuides(depth uint8, m metrics, c color.Color) {
	n := int(depth)
	if n > maxIndentGuides {
		n = maxIndentGuides
	}
	for i := 0; i < n; i++ {
		g := rr.guide(i)
		x := m.textOriginX(uint8(i)) - m.triangleSlot + 1
		g.StrokeColor = c
		g.StrokeWidth = 1
		g.Position1 = fyne.NewPos(x, 0)
		g.Position2 = fyne.NewPos(x, m.rowH)
		g.Show()
	}
	rr.hideGuides(n)
}

func (rr *rowRenderer) layoutTriangle(depth uint8, m metrics, collapsed bool, c color.Color) {
	if rr.triangle == nil {
		rr.triangle = canvas.NewText("", c)
	}
	if collapsed {
		rr.triangle.Text = "▶"
	} else {
		rr.triangle.Text = "▼"
	}
	rr.triangle.TextSize = m.textSize * 0.8
	rr.triangle.Color = c
	rr.triangle.Move(fyne.NewPos(m.triangleX(depth), m.textY()))
	rr.triangle.Resize(rr.triangle.MinSize())
	rr.triangle.Show()
}

func (rr *rowRenderer) triangleHide() {
	if rr.triangle != nil {
		rr.triangle.Hide()
	}
}

// --- pooled-object accessors (grow on demand, hide surplus) ---

func (rr *rowRenderer) text(i int) *canvas.Text {
	for i >= len(rr.texts) {
		rr.texts = append(rr.texts, canvas.NewText("", color.White))
	}
	return rr.texts[i]
}

func (rr *rowRenderer) hideFrom(i int) {
	for ; i < len(rr.texts); i++ {
		rr.texts[i].Hide()
	}
}

func (rr *rowRenderer) guide(i int) *canvas.Line {
	for i >= len(rr.guides) {
		rr.guides = append(rr.guides, canvas.NewLine(color.Gray{0x40}))
	}
	return rr.guides[i]
}

func (rr *rowRenderer) hideGuides(i int) {
	for ; i < len(rr.guides); i++ {
		rr.guides[i].Hide()
	}
}
