package prettyview

import (
	"os"
	"strings"
	"testing"
)

func lineContaining(d *Document, sub string) int32 {
	for li := range d.Lines {
		if strings.Contains(d.lineString(int32(li)), sub) {
			return int32(li)
		}
	}
	return -1
}

func TestSearchPlain(t *testing.T) {
	pv := docPV(`{"name":"alpha","other":"alphabet"}`, FormatJSON)
	pv.Search(SearchQuery{Text: "alpha"})
	active, total, _ := pv.SearchStatus()
	if total != 2 {
		t.Fatalf("plain search total = %d, want 2", total)
	}
	if active != 1 {
		t.Errorf("active = %d, want 1", active)
	}
	// Verify the first match slices back to the needle.
	m := pv.search.matches[0]
	runes := []rune(pv.doc.displayString(m.Line))
	if got := string(runes[m.ColStart:m.ColEnd]); got != "alpha" {
		t.Errorf("match text = %q, want %q", got, "alpha")
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	pv := docPV(`{"a":"Alpha","b":"ALPHA"}`, FormatJSON)
	pv.Search(SearchQuery{Text: "alpha", CaseSensitive: false})
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf("case-insensitive total = %d, want 2", total)
	}
	pv.Search(SearchQuery{Text: "alpha", CaseSensitive: true})
	if _, total, _ := pv.SearchStatus(); total != 0 {
		t.Errorf("case-sensitive total = %d, want 0", total)
	}
}

func TestSearchRegex(t *testing.T) {
	pv := docPV(`{"a":"alpha","b":"alphabet","c":"beta"}`, FormatJSON)
	pv.Search(SearchQuery{Text: `alpha\w*`, Mode: SearchRegex})
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf("regex total = %d, want 2", total)
	}
}

func TestSearchBadRegex(t *testing.T) {
	pv := docPV(`{"a":1}`, FormatJSON)
	pv.Search(SearchQuery{Text: "[", Mode: SearchRegex})
	if pv.search.err == nil {
		t.Error("expected an error for a bad regex")
	}
}

func TestSearchRevealsFolded(t *testing.T) {
	pv := docPV(`{"outer":{"deep":"needle"}}`, FormatJSON)
	o := findFoldHead(pv.doc, `"outer"`)
	pv.doc.fold.fold(pv.doc, o)

	deep := lineContaining(pv.doc, "needle")
	if deep < 0 {
		t.Fatal("could not find the deep line")
	}
	if pv.doc.fold.vis[deep] != 0 {
		t.Fatal("precondition: deep line should be hidden before search")
	}

	pv.Search(SearchQuery{Text: "needle"})
	if pv.doc.fold.vis[deep] != 1 {
		t.Error("search did not reveal the match inside the folded node")
	}
}

func TestSearchNavWrap(t *testing.T) {
	pv := docPV(`{"a":"x","b":"x","c":"x"}`, FormatJSON)
	pv.Search(SearchQuery{Text: `"x"`})
	if _, total, _ := pv.SearchStatus(); total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if a, _, _ := pv.SearchStatus(); a != 1 {
		t.Fatalf("initial active = %d, want 1", a)
	}
	pv.SearchNext()
	pv.SearchNext()
	if a, _, _ := pv.SearchStatus(); a != 3 {
		t.Fatalf("after 2x next, active = %d, want 3", a)
	}
	pv.SearchNext() // wrap
	if a, _, _ := pv.SearchStatus(); a != 1 {
		t.Fatalf("after wrap, active = %d, want 1", a)
	}
	pv.SearchPrev() // wrap backward
	if a, _, _ := pv.SearchStatus(); a != 3 {
		t.Fatalf("after prev wrap, active = %d, want 3", a)
	}
}

func TestSearchMultibyteColumns(t *testing.T) {
	pv := docPV(`{"k":"héllo wörld"}`, FormatJSON)
	pv.Search(SearchQuery{Text: "wörld"})
	if len(pv.search.matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(pv.search.matches))
	}
	m := pv.search.matches[0]
	runes := []rune(pv.doc.displayString(m.Line))
	if got := string(runes[m.ColStart:m.ColEnd]); got != "wörld" {
		t.Errorf("multibyte match = %q, want %q (rune columns wrong)", got, "wörld")
	}
}

func TestSearchHighlightBounded(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	pv.Search(SearchQuery{Text: "item"}) // common token across the whole doc
	_, total, capped := pv.SearchStatus()
	pv.r.reflow()
	rects := len(pv.r.matchLayer.Objects)
	visRows := pv.r.lastRow - pv.r.firstRow + 1
	t.Logf("matches=%d capped=%v, match rects on screen=%d, visible rows=%d", total, capped, rects, visRows)
	if rects > visRows*8 {
		t.Errorf("match rect count %d exceeds visible bound (%d rows)", rects, visRows)
	}
}
