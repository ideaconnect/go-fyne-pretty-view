package parse

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// advAssertRunes pins display-runes==buffer-runes per physical line, with the format and
// a label so failures localize.
func advAssertRunes(t *testing.T, label string, src []byte, d *model.Document) {
	t.Helper()
	lines := strings.Split(string(src), "\n")
	if d.TotalLines() != len(lines) {
		t.Fatalf("%s: TotalLines=%d want %d", label, d.TotalLines(), len(lines))
	}
	for i, ln := range lines {
		if got, want := d.LineRuneLen(int32(i)), utf8.RuneCount([]byte(ln)); got != want {
			t.Errorf("%s: line %d display runes %d, want buffer runes %d (%q)", label, i, got, want, ln)
		}
	}
}

// advNoRawControl verifies no display line carries a raw grid-hostile byte (each must be a
// placeholder rune). This is the rendering-grid safety half of the invariant.
func advNoRawControl(t *testing.T, label string, d *model.Document) {
	t.Helper()
	for li := 0; li < d.TotalLines(); li++ {
		s := d.LineString(int32(li))
		for i := 0; i < len(s); i++ {
			if s[i] < 0x20 || s[i] == 0x7f {
				t.Errorf("%s: display line %d carries raw control 0x%02x: %q", label, li, s[i], s)
				break
			}
		}
	}
}

// advNoFold verifies the projection is flat (no folding => no reflow).
func advNoFold(t *testing.T, label string, d *model.Document) {
	t.Helper()
	for i := 0; i < len(d.Lines); i++ {
		if d.Lines[i].Fold != model.NoNode {
			t.Errorf("%s: line %d Fold=%v want NoNode (a fold head can reflow)", label, i, d.Lines[i].Fold)
		}
	}
}

// overBudget returns a buffer guaranteed strictly over the live-color budget.
func overBudget(payload []byte) []byte {
	for len(payload) <= LiveColorBudgetBytes {
		payload = append(payload, payload...)
	}
	return payload
}

// TestAdvOverBudgetHostileBytes is the adversarial core: over the budget (colorize=false)
// the projection must keep display-runes==buffer-runes per line, never emit a raw control
// byte, and never fold/panic — across multi-byte UTF-8, raw control bytes (incl. \r and
// \x7f), tabs, a CRLF, and mixed content. The shipped budget test only uses clean ASCII.
func TestAdvOverBudgetHostileBytes(t *testing.T) {
	// One physical line of every hostile category, repeated; embed real newlines so the
	// document is multi-line, plus a CRLF (\r is grid-hostile, \n is the splitter).
	unit := "héllo\tw\x01rld\x7f é中文 \r mix\n{\"k\":\"v\tx\"}\r\n"
	src := overBudget([]byte(strings.Repeat(unit, 64)))
	if WithinLiveColorBudget(len(src)) {
		t.Fatalf("buffer %d not over budget %d", len(src), LiveColorBudgetBytes)
	}
	for _, f := range []Format{FormatJSON, FormatXML, FormatJSONC, FormatRaw, FormatAuto, FormatHTML} {
		d := ParseEditableColored(src, f, 0, 4)
		label := "over-budget fmt=" + formatName(f)
		advAssertRunes(t, label, src, d)
		advNoRawControl(t, label, d)
		advNoFold(t, label, d)
	}
}

// TestAdvBudgetCliffShapeIdentical proves the HARD claim directly: the *display shape*
// (line count + per-line rune count + each line's exact display string) is identical
// whether the colorizer ran or not. We force the same content through both the colorize
// and no-colorize paths and diff the projections line by line. If the budget switch ever
// shifted a caret column, this fails.
func TestAdvBudgetCliffShapeIdentical(t *testing.T) {
	// Hostile, multi-line content small enough to colorize, then we compare the two paths.
	content := []byte("{\"k\":\"a\tb\x01c中\"}\n[1,2,3]\nhéllo\x7fworld\r\nlast")
	for _, f := range []Format{FormatJSON, FormatJSONC, FormatXML, FormatHTML, FormatRaw} {
		colored := editColorParser{format: f, colorize: true}
		mono := editColorParser{format: f, colorize: false}

		bc := model.NewBuilder(content, f, 0)
		_ = colored.Parse(content, bc)
		dc := bc.Finish()

		bm := model.NewBuilder(content, f, 0)
		_ = mono.Parse(content, bm)
		dm := bm.Finish()

		if dc.TotalLines() != dm.TotalLines() {
			t.Fatalf("fmt=%s line count differs: colored=%d mono=%d", formatName(f), dc.TotalLines(), dm.TotalLines())
		}
		for li := 0; li < dc.TotalLines(); li++ {
			if dc.LineRuneLen(int32(li)) != dm.LineRuneLen(int32(li)) {
				t.Errorf("fmt=%s line %d rune len differs: colored=%d mono=%d", formatName(f), li, dc.LineRuneLen(int32(li)), dm.LineRuneLen(int32(li)))
			}
			if dc.LineString(int32(li)) != dm.LineString(int32(li)) {
				t.Errorf("fmt=%s line %d display string differs:\n colored=%q\n mono   =%q", formatName(f), li, dc.LineString(int32(li)), dm.LineString(int32(li)))
			}
		}
	}
}

// TestAdvOverBudgetSingleHugeLine is the single-huge-line worst case (#65's minified blob):
// one physical line far over budget, no newline at all. The budget fix's guarantee is that
// the OVER-budget (colorize=false) projection is display-shape-identical to the colorize=true
// projection of the SAME bytes. It does NOT promise universal rune-equality on a huge line
// dense with grid-hostile bytes: each such byte becomes its own LitSeg and Line.SegCount is a
// uint16, so beyond ~65535 segments the tail is dropped on BOTH paths (a pre-existing
// maxColorLineBytes / SegCount-saturation limit, not introduced by #65 — see the clean-line
// case below, which keeps full rune-equality because clean runs collapse to one SrcSeg).
func TestAdvOverBudgetSingleHugeLine(t *testing.T) {
	// (a) A CLEAN huge line (one giant JSON string) keeps full display-runes==buffer-runes
	// over budget: clean bytes collapse into a single zero-copy SrcSeg, no saturation.
	clean := []byte(`["` + strings.Repeat("x", LiveColorBudgetBytes+(1<<20)) + `"]`)
	if WithinLiveColorBudget(len(clean)) {
		t.Fatalf("clean huge line %d not over budget", len(clean))
	}
	dc := ParseEditableColored(clean, FormatJSON, 0, 4)
	if dc.TotalLines() != 1 {
		t.Fatalf("clean huge: TotalLines=%d want 1", dc.TotalLines())
	}
	advAssertRunes(t, "huge-clean-over-budget", clean, dc)
	advNoFold(t, "huge-clean-over-budget", dc)

	// (b) A grid-hostile-DENSE huge line: the over-budget (colorize=false) projection must be
	// byte-for-byte display-identical to the colorize=true projection of the same bytes. The
	// budget switch never changes the shape — even where SegCount saturates, it saturates the
	// same way on both sides, so the caret column is consistent and never panics.
	var sb strings.Builder
	for sb.Len() <= LiveColorBudgetBytes+(1<<20) {
		sb.WriteString("中x\tyé")
	}
	src := []byte(sb.String())
	if strings.ContainsRune(sb.String(), '\n') {
		t.Fatalf("test setup: huge line must have no newline")
	}
	colored := model.NewBuilder(src, FormatJSON, 0)
	_ = editColorParser{format: FormatJSON, colorize: true}.Parse(src, colored)
	dColor := colored.Finish()
	dMono := ParseEditableColored(src, FormatJSON, 0, 4) // over budget => colorize=false
	if dColor.TotalLines() != 1 || dMono.TotalLines() != 1 {
		t.Fatalf("huge single line: colored=%d mono=%d want 1 each", dColor.TotalLines(), dMono.TotalLines())
	}
	if dColor.LineRuneLen(0) != dMono.LineRuneLen(0) {
		t.Errorf("budget cliff shifted huge-line shape: colored runes %d != mono runes %d", dColor.LineRuneLen(0), dMono.LineRuneLen(0))
	}
	if dColor.LineString(0) != dMono.LineString(0) {
		t.Errorf("budget cliff changed huge-line display string")
	}
	advNoRawControl(t, "huge-hostile-over-budget", dMono)
	advNoFold(t, "huge-hostile-over-budget", dMono)
}

// TestAdvBudgetEmptyAndTinyAtCliff sweeps the exact boundary at/around the cap with hostile
// trailing bytes, and the empty buffer, on both sides. n==cap colorizes; n==cap+1 does not.
func TestAdvBudgetEmptyAndTinyAtCliff(t *testing.T) {
	for _, src := range [][]byte{
		nil,
		[]byte(""),
		[]byte("\x01"),
		[]byte("\t"),
		[]byte("中"),
	} {
		d := ParseEditableColored(src, FormatJSON, 0, 4)
		advAssertRunes(t, "tiny", src, d)
		advNoRawControl(t, "tiny", d)
	}

	// Build a buffer of exactly the cap and exactly cap+1, both ending in hostile bytes,
	// so the boundary line itself contains a tab + control + multibyte rune.
	tail := []byte("\t\x01中") // 3 hostile/multibyte chars, len 5 bytes
	atCap := make([]byte, LiveColorBudgetBytes)
	for i := range atCap {
		atCap[i] = 'a'
	}
	copy(atCap[len(atCap)-len(tail):], tail)
	if !WithinLiveColorBudget(len(atCap)) {
		t.Fatalf("atCap len %d should be within budget", len(atCap))
	}
	overCap := append(append([]byte{}, atCap...), 'z') // cap+1: over budget
	if WithinLiveColorBudget(len(overCap)) {
		t.Fatalf("overCap len %d should be over budget", len(overCap))
	}
	dAt := ParseEditableColored(atCap, FormatJSON, 0, 4)
	dOver := ParseEditableColored(overCap, FormatJSON, 0, 4)
	advAssertRunes(t, "atCap", atCap, dAt)
	advNoRawControl(t, "atCap", dAt)
	advAssertRunes(t, "overCap", overCap, dOver)
	advNoRawControl(t, "overCap", dOver)
}

func formatName(f Format) string {
	switch f {
	case FormatJSON:
		return "JSON"
	case FormatJSONC:
		return "JSONC"
	case FormatXML:
		return "XML"
	case FormatHTML:
		return "HTML"
	case FormatRaw:
		return "Raw"
	case FormatAuto:
		return "Auto"
	}
	return "?"
}
