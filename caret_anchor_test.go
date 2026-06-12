package prettyview

import (
	"strings"
	"testing"
)

// TestCaretAnchorSurvivesReformatJSON: editing a deeply nested value and pausing leaves
// the caret on that same value (rune-precise) in the reformatted output. The buffer
// bytes are unchanged by the reformat, so the caret's stable byte offset is the anchor.
func TestCaretAnchorSurvivesReformatJSON(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	const src = `{"a":{"b":{"c":"deepvalue"}}}`
	typeStr(pv, src)
	mid := strings.Index(src, "deepvalue") + 4 // inside "deep|value"; ASCII so col == byte index on the single raw line
	pv.sel.focus = modelPos{line: 0, col: mid}
	pv.sel.anchor = pv.sel.focus
	pv.sel.placed = true

	pv.Reformat()
	lt := pv.doc.LineString(pv.sel.focus.line)
	if !strings.Contains(lt, "deepvalue") {
		t.Fatalf("caret landed on line %q, want the \"deepvalue\" line", lt)
	}
	// The caret was inside "deep|value" (mid = ...+4). It must remap to EXACTLY that
	// rune boundary in the reformatted line — not merely somewhere within the token.
	runes := []rune(lt)
	col := pv.sel.focus.col
	if col < 0 || col > len(runes) {
		t.Fatalf("caret col %d out of range for %q", col, lt)
	}
	if before, after := string(runes[:col]), string(runes[col:]); !strings.HasSuffix(before, "deep") || !strings.HasPrefix(after, "value") {
		t.Errorf("caret col %d is not at the exact 'deep|value' boundary in %q (before=%q after=%q)", col, lt, before, after)
	}
}

// TestCaretAnchorShapeChangeLandsAtNode: the caret's byte offset just inside an array
// lands the caret at the array's first element after pretty-printing — a shape-aware
// edit lands "at the new node", which falls out of byte-offset anchoring.
func TestCaretAnchorShapeChangeLandsAtNode(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	const src = `{"a":[10,20,30]}`
	typeStr(pv, src)
	off := strings.IndexByte(src, '[') + 1 // the '1' of the first element
	pv.sel.focus = modelPos{line: 0, col: off}
	pv.sel.placed = true

	pv.Reformat()
	lt := pv.doc.LineString(pv.sel.focus.line)
	if !strings.Contains(lt, "10") {
		t.Errorf("caret inside the array landed on %q, want the first element (10) line", lt)
	}
}

// TestCaretAnchorFallbackRawAndXML: non-structured input keeps the raw projection with a
// valid caret, and an XML reformat leaves the caret on a valid line — both without panic.
func TestCaretAnchorFallbackRawAndXML(t *testing.T) {
	raw, w1 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer w1.Close()
	typeStr(raw, "plain text line one\nand line two")
	raw.sel.focus = modelPos{line: 1, col: 3}
	raw.sel.placed = true
	raw.Reformat() // stays raw (non-structured input is never rewritten)
	if l := int(raw.sel.focus.line); l < 0 || l >= raw.doc.TotalLines() {
		t.Errorf("caret line %d out of range after raw reformat (lines=%d)", l, raw.doc.TotalLines())
	}

	xml, w2 := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer w2.Close()
	const xsrc = `<root><item>hello</item></root>`
	typeStr(xml, xsrc)
	xml.sel.focus = modelPos{line: 0, col: strings.Index(xsrc, "hello") + 2}
	xml.sel.placed = true
	xml.Reformat() // XML structured or raw fallback — either way a valid caret, no panic
	if l := int(xml.sel.focus.line); l < 0 || l >= xml.doc.TotalLines() {
		t.Errorf("caret line %d out of range after XML reformat (lines=%d)", l, xml.doc.TotalLines())
	}
}
