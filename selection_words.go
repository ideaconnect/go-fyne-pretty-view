package prettyview

import (
	"unicode"
	"unicode/utf8"
)

// runeClass groups characters for word selection: a double-click extends over a
// run of the same class.
type runeClass uint8

const (
	classWord runeClass = iota // letters, digits, underscore
	classSpace
	classOther
)

func classOf(r rune) runeClass {
	switch {
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return classWord
	case unicode.IsSpace(r):
		return classSpace
	default:
		return classOther
	}
}

// displayRunes decodes a line's currently-displayed text into pv.wordScratch,
// reusing the backing array across calls so a word/line drag does not allocate a
// fresh string + []rune of the whole line on every pointer event.
func (pv *PrettyView) displayRunes(li int32) []rune {
	buf := pv.wordScratch[:0]
	for _, s := range pv.doc.DisplaySegs(li) {
		b := pv.doc.SegBytes(s)
		for i := 0; i < len(b); {
			if b[i] < utf8.RuneSelf {
				buf = append(buf, rune(b[i]))
				i++
				continue
			}
			r, sz := utf8.DecodeRune(b[i:])
			buf = append(buf, r)
			i += sz
		}
	}
	pv.wordScratch = buf
	return buf
}

// wordBounds returns the [start, end) columns of the word (same-class run) under
// col on a line.
func (pv *PrettyView) wordBounds(line int32, col int) (modelPos, modelPos) {
	vl := pv.doc.VisibleLine(line)
	runes := pv.displayRunes(vl)
	n := len(runes)
	if n == 0 {
		return modelPos{line: vl, col: 0}, modelPos{line: vl, col: 0}
	}
	if col >= n {
		col = n - 1
	}
	if col < 0 {
		col = 0
	}
	cls := classOf(runes[col])
	start, end := col, col+1
	for start > 0 && classOf(runes[start-1]) == cls {
		start--
	}
	for end < n && classOf(runes[end]) == cls {
		end++
	}
	return modelPos{line: vl, col: start}, modelPos{line: vl, col: end}
}

// lineBounds returns the full extent of a line.
func (pv *PrettyView) lineBounds(line int32) (modelPos, modelPos) {
	vl := pv.doc.VisibleLine(line)
	return modelPos{line: vl, col: 0}, modelPos{line: vl, col: pv.doc.LineRuneLen(vl)}
}

// stepPos returns the caret position one rune (word false) or one word (word true) before
// or after p, a visible position. A word step skips any whitespace, then the run of one
// class (classOf, the same classes a double-click selects). At a line end a forward step
// lands on the next visible line's start, and at a line start a backward step lands on the
// previous visible line's end, as in widget.Entry; at either end of the document p is
// returned unchanged.
func (pv *PrettyView) stepPos(p modelPos, forward, word bool) modelPos {
	n := pv.doc.LineRuneLen(p.line)
	if forward && p.col >= n {
		row := pv.doc.RowOfLine(p.line) + pv.doc.RowsOfLine(p.line)
		if row >= pv.doc.TotalVisibleRows() {
			return p
		}
		next, _ := pv.doc.LineAndSubRowAtRow(row)
		return modelPos{line: next, col: 0}
	}
	if !forward && p.col <= 0 {
		row := pv.doc.RowOfLine(p.line) - 1
		if row < 0 {
			return p
		}
		prev, _ := pv.doc.LineAndSubRowAtRow(row)
		return modelPos{line: prev, col: pv.doc.LineRuneLen(prev)}
	}
	if !word {
		if forward {
			p.col++
		} else {
			p.col--
		}
		return p
	}
	runes := pv.displayRunes(p.line)
	col := min(p.col, len(runes))
	if forward {
		for col < len(runes) && classOf(runes[col]) == classSpace {
			col++
		}
		if col < len(runes) {
			cls := classOf(runes[col])
			for col < len(runes) && classOf(runes[col]) == cls {
				col++
			}
		}
	} else {
		for col > 0 && classOf(runes[col-1]) == classSpace {
			col--
		}
		if col > 0 {
			cls := classOf(runes[col-1])
			for col > 0 && classOf(runes[col-1]) == cls {
				col--
			}
		}
	}
	p.col = col
	return p
}
