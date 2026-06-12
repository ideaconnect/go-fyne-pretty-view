package prettyview

import (
	"strings"
	"testing"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// assertCaretExact pins the HARD invariant at the widget level: the projected display line
// the caret sits on has exactly as many runes as the buffer line, and the caret round-trips
// (caretOff -> setCaretOff -> caretOff is a fixpoint at the same byte offset). This is the
// property #65's fix must not break when a large paste flips the buffer over the budget.
func assertCaretExact(t *testing.T, pv *PrettyView, label string) {
	t.Helper()
	src := pv.Source()
	lines := strings.Split(string(src), "\n")
	if pv.doc.TotalLines() != len(lines) {
		t.Fatalf("%s: projection TotalLines=%d, buffer lines=%d", label, pv.doc.TotalLines(), len(lines))
	}
	for i, ln := range lines {
		// A huge grid-hostile-dense line can saturate SegCount (a pre-existing limit), so
		// only assert exact rune-equality on lines short enough to never saturate. Every
		// realistic edit line is far below that, and the budget gate never changes it.
		if len(ln) > 60000 {
			continue
		}
		if got, want := pv.doc.LineRuneLen(int32(i)), utf8.RuneCount([]byte(ln)); got != want {
			t.Errorf("%s: line %d display runes %d, want buffer runes %d", label, i, got, want)
		}
	}
	// Caret offset round-trips through the buffer (line,col) mapping.
	off := pv.caretOff()
	pv.setCaretOff(off)
	if off2 := pv.caretOff(); off2 != off {
		t.Errorf("%s: caret offset did not round-trip: %d -> %d", label, off, off2)
	}
}

// TestAdvPasteAcrossBudgetAuto pastes a payload that pushes a FormatAuto editor from below
// to above the live-color budget, then keeps typing. It exercises the edit.go change (skip
// the whole-buffer AutoDetect above budget) end to end and checks: the buffer round-trips
// losslessly (CODE_BIBLE rule 7), no display line carries a raw control byte, the caret lands
// at the exact end of the paste, and the caret stays exact while typing past the cliff.
func TestAdvPasteAcrossBudgetAuto(t *testing.T) {
	test.NewApp()
	// Default InputConfig: MaxEditBytes == 0 (unlimited), AutoFormat default — the exact #65
	// configuration (live colorizer ungated by any cap). FormatAuto via no explicit format.
	pv := New(WithEditable())
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))
	pv.Refresh()
	pv.FocusGained()

	// Seed with a small, colorizable JSON object (below budget => colorized live).
	typeStr(pv, `{"a":1,`)
	if !parse.WithinLiveColorBudget(pv.buf.Len()) {
		t.Fatalf("seed buffer should be below budget")
	}
	assertCaretExact(t, pv, "below-budget seed")

	// Build an over-budget paste payload with hostile bytes (multibyte, tab, control, CRLF).
	var sb strings.Builder
	unit := "\"x中\":\"v\tw\x01\",\r\n"
	for sb.Len() <= parse.LiveColorBudgetBytes+(1<<20) {
		sb.WriteString(unit)
	}
	payload := sb.String()
	setClipboard(payload)
	// Paste normalizes CRLF/CR to LF (documented), so the inserted bytes are the normalized
	// payload; the caret/round-trip assertions compare against that, not the raw clipboard.
	normPayload := strings.ReplaceAll(strings.ReplaceAll(payload, "\r\n", "\n"), "\r", "\n")

	caretBefore := pv.caretOff()
	pv.Paste()
	if parse.WithinLiveColorBudget(pv.buf.Len()) {
		t.Fatalf("after paste buffer (%d) should be over budget (%d)", pv.buf.Len(), parse.LiveColorBudgetBytes)
	}
	// Caret must sit exactly len(normPayload) bytes past where it was.
	if got, want := pv.caretOff(), caretBefore+len(normPayload); got != want {
		t.Errorf("caret after paste = %d, want %d (caretBefore %d + normalized paste %d)", got, want, caretBefore, len(normPayload))
	}
	assertCaretExact(t, pv, "after over-budget paste")

	// No display line may contain a raw control byte even over budget (placeholders only).
	for li := 0; li < pv.doc.TotalLines(); li++ {
		if hasRawControl(pv.doc.LineString(int32(li))) {
			t.Fatalf("over-budget projection line %d carries a raw control byte", li)
		}
	}

	// Keep typing past the cliff: still over budget, caret still exact, buffer still lossless.
	typeStr(pv, "tail中\t")
	assertCaretExact(t, pv, "typing past cliff")

	// Lossless round-trip: Source() is exactly seed + normalized payload + tail. (Paste's
	// CRLF->LF is the one documented transform; everything else round-trips, CODE_BIBLE r7.)
	want := `{"a":1,` + normPayload + "tail中\t"
	if got := string(pv.Source()); got != want {
		t.Errorf("Source() did not round-trip across the budget cliff (len got=%d want=%d)", len(got), len(want))
	}
}

// TestAdvDeleteBackBelowBudgetRecolors verifies color RESUMES when a delete shrinks the
// buffer back under the budget (the fix's "color resumes automatically" claim). It also
// guards the symmetric direction of the cliff that the paste test crosses upward.
func TestAdvDeleteBackBelowBudgetRecolors(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable(), WithFormat(FormatJSON))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))
	pv.Refresh()
	pv.FocusGained()

	// Paste an over-budget colorizable JSON-ish blob (single line), then it is monochrome.
	var sb strings.Builder
	sb.WriteString("[")
	for sb.Len() <= parse.LiveColorBudgetBytes+(1<<20) {
		sb.WriteString(`"abc",`)
	}
	sb.WriteString(`"z"]`)
	setClipboard(sb.String())
	pv.Paste()
	if parse.WithinLiveColorBudget(pv.buf.Len()) {
		t.Fatalf("blob should be over budget")
	}
	overColored := projectionHasColor(pv)
	if overColored {
		t.Errorf("over-budget projection should be monochrome, but has colored segments")
	}

	// Select most of the buffer (a single physical line) and delete it, dropping back under
	// budget. Endpoints are set as buffer (line,col) via the buffer's own offset mapping.
	loLine, loCol := pv.buf.LineColAt(20)
	hiLine, hiCol := pv.buf.LineColAt(pv.buf.Len())
	pv.sel.anchor = modelPos{line: int32(loLine), col: loCol}
	pv.sel.focus = modelPos{line: int32(hiLine), col: hiCol}
	pv.sel.active = true
	pv.sel.placed = true
	pv.editDelete(true)
	if !parse.WithinLiveColorBudget(pv.buf.Len()) {
		t.Fatalf("after delete buffer (%d) should be back under budget", pv.buf.Len())
	}
	if !projectionHasColor(pv) {
		t.Errorf("color did not resume after shrinking back under budget")
	}
	assertCaretExact(t, pv, "after delete below budget")
}

func projectionHasColor(pv *PrettyView) bool {
	for li := 0; li < pv.doc.TotalLines(); li++ {
		for _, s := range pv.doc.LineSegs(int32(li)) {
			if s.Role != 0 { // 0 == RolePlain
				return true
			}
		}
	}
	return false
}
