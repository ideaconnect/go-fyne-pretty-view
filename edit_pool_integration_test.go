package prettyview

import (
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// The live reproject reuses pooled arenas and a reused buffer snapshot (#80). This drives the
// REAL edit/reformat/undo/redo code paths and, at each step, asserts the live (pooled) document
// equals a fresh, un-pooled ParseEditableColored of the current buffer — proving the snapshot
// aliasing is safe and the pooled doc never diverges from the proven full-rebuild. It runs under
// -race in `make check`, which also guards the single-goroutine no-retained-doc contract.
func TestReprojectPooledMatchesFreshIntegration(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	assertPooledDocMatchesFresh := func(when string) {
		t.Helper()
		src := pv.buf.Bytes() // a fresh, independent snapshot (not the pooled one)
		fresh := parse.ParseEditableColored(src, pv.resolveFormat(src), pv.cfg.collapseDepth)
		got := pv.doc
		if got.TotalLines() != fresh.TotalLines() {
			t.Fatalf("%s: pooled TotalLines %d != fresh %d (buf=%q)", when, got.TotalLines(), fresh.TotalLines(), src)
		}
		// LineString is wrap-independent, so it is a valid pooled-vs-fresh check even after the
		// renderer has applied soft-wrap weights to the live doc's fold index.
		for i := 0; i < got.TotalLines(); i++ {
			if got.LineString(int32(i)) != fresh.LineString(int32(i)) {
				t.Fatalf("%s: line %d pooled %q != fresh %q", when, i, got.LineString(int32(i)), fresh.LineString(int32(i)))
			}
		}
	}

	typeStr(pv, "alpha\nbeta\tgamma\n中文 line\n[1, 2, 3]")
	assertPooledDocMatchesFresh("after typing")

	pv.Reformat() // rewrites the buffer in place (JSON-ish content) — a whole-buffer edit
	assertPooledDocMatchesFresh("after reformat")

	for i := 0; i < 12; i++ { // backspaces across line joins
		pv.editDelete(false)
	}
	assertPooledDocMatchesFresh("after backspaces")

	pv.Undo()
	assertPooledDocMatchesFresh("after undo")
	pv.Redo()
	assertPooledDocMatchesFresh("after redo")

	// A grow→shrink cycle through the pool: paste a chunk, then delete most of it.
	typeStr(pv, "\nXYZ\nlonger line of text here\nmore")
	assertPooledDocMatchesFresh("after grow")
	for i := 0; i < 25; i++ {
		pv.editDelete(false)
	}
	assertPooledDocMatchesFresh("after shrink")
}
