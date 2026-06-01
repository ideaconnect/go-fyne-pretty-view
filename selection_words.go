package prettyview

import "unicode"

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

// wordBounds returns the [start, end) columns of the word (same-class run) under
// col on a line.
func (pv *PrettyView) wordBounds(line int32, col int) (modelPos, modelPos) {
	vl := pv.doc.VisibleLine(line)
	runes := []rune(pv.doc.DisplayString(vl))
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
