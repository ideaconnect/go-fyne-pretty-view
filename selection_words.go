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
