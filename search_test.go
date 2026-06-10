package prettyview

import (
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

func lineContaining(d *model.Document, sub string) int32 {
	for li := range d.Lines {
		if strings.Contains(d.LineString(int32(li)), sub) {
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
	runes := []rune(pv.doc.DisplayString(m.Line))
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

// TestSearchRegexAnchors pins that the regex scan evaluates each pattern against
// the whole line, not against a re-sliced suffix. A from-offset
// FindIndex(scratch[from:]) loop re-anchors ^ at every resume point and invents a
// start-of-text word boundary, producing spurious matches.
func TestSearchRegexAnchors(t *testing.T) {
	// ^. anchors to the line start: exactly one match on a single line, not one
	// per byte. (The old suffix-reslicing loop reported one per byte.)
	pv := docPV("abcabc", FormatRaw)
	pv.Search(SearchQuery{Text: `^.`, Mode: SearchRegex})
	if _, total, _ := pv.SearchStatus(); total != 1 {
		t.Errorf(`^. total = %d, want 1`, total)
	}

	// \ba matches an "a" at a word boundary. In "aab" only the leading a qualifies;
	// the second a is preceded by a word char. Re-slicing after the first match made
	// the resumed suffix "ab" start-anchored, inventing a boundary before its 'a'.
	pv = docPV("aab", FormatRaw)
	pv.Search(SearchQuery{Text: `\ba`, Mode: SearchRegex})
	if _, total, _ := pv.SearchStatus(); total != 1 {
		t.Errorf(`\ba total = %d, want 1 (not a spurious boundary match)`, total)
	}
}

// TestSearchRegexZeroWidth pins that a zero-width-capable pattern terminates and
// records only the non-empty runs, never an empty highlight at every position.
func TestSearchRegexZeroWidth(t *testing.T) {
	pv := docPV("foo bar boo", FormatRaw)
	pv.Search(SearchQuery{Text: `o*`, Mode: SearchRegex})
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf(`o* total = %d, want 2 (the two "oo" runs; empty matches skipped)`, total)
	}
}

// TestSearchRegexMaxMatchesCap mirrors TestSearchMaxMatchesCap for the regex path:
// many matches on one line must honor the cap (and report capped) rather than the
// per-line FindAllIndex over-running the global budget.
func TestSearchRegexMaxMatchesCap(t *testing.T) {
	const cap = 50
	const occ = cap + 25
	src := strings.Repeat("x ", occ)

	capped := NewWithData([]byte(src), FormatRaw,
		WithSearchConfig(SearchConfig{MaxMatches: cap, MinQueryLen: 1}))
	capped.Search(SearchQuery{Text: `x`, Mode: SearchRegex})
	if _, total, isCapped := capped.SearchStatus(); total != cap || !isCapped {
		t.Errorf("regex capped: total=%d capped=%v, want total=%d capped=true", total, isCapped, cap)
	}
	if got := len(capped.search.matches); got != cap {
		t.Errorf("regex stored %d matches, want exactly the cap %d", got, cap)
	}

	uncapped := NewWithData([]byte(src), FormatRaw,
		WithSearchConfig(SearchConfig{MaxMatches: 1000, MinQueryLen: 1}))
	uncapped.Search(SearchQuery{Text: `x`, Mode: SearchRegex})
	if _, total, isCapped := uncapped.SearchStatus(); isCapped || total != occ {
		t.Errorf("regex uncapped: total=%d capped=%v, want total=%d capped=false", total, isCapped, occ)
	}
}

// TestSearchRegexLiteralPrefixPrefilter guards the literal-prefix prefilter, which
// skips the RE2 engine on lines lacking the pattern's required literal head: it must
// never drop a real match nor invent one. A line WITH the prefix but no full match
// (e.g. "item-ish") must still be scanned and yield nothing; a line WITHOUT the
// prefix must be correctly absent; an anchored pattern must still anchor.
func TestSearchRegexLiteralPrefixPrefilter(t *testing.T) {
	src := `{"item1":"x","note":"item-ish","item22":"y","other":"z"}`
	pv := docPV(src, FormatJSON)

	// `item\d+` has literal prefix "item"; only the item1 and item22 keys match
	// ("item-ish" has the prefix but no digit; "note"/"other" lack the prefix).
	pv.Search(SearchQuery{Text: `item\d+`, Mode: SearchRegex, CaseSensitive: true})
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf(`item\d+ total = %d, want 2 (item1, item22)`, total)
	}

	// Case-insensitive with a CASED head ("item") has no literal prefix (the (?i)
	// compile reports none), so the prefilter does not fire and must not drop the
	// upper-case-only hit.
	ci := docPV(`{"a":"ITEM5","b":"plain"}`, FormatJSON)
	ci.Search(SearchQuery{Text: `item\d`, Mode: SearchRegex, CaseSensitive: false})
	if _, total, _ := ci.SearchStatus(); total != 1 {
		t.Errorf(`(?i)item\d total = %d, want 1 (ITEM5)`, total)
	}

	// Case-insensitive with a CASELESS head ("0x") DOES have a non-empty literal
	// prefix, so the prefilter fires on "0x" — it must still match every casing of the
	// cased tail and must not drop them, while a line lacking the caseless head is
	// correctly excluded.
	cl := docPV(`{"a":"0xff","b":"0XFF","c":"0xFf","d":"1xff"}`, FormatJSON)
	cl.Search(SearchQuery{Text: `0x[0-9a-f]{2}`, Mode: SearchRegex, CaseSensitive: false})
	if _, total, _ := cl.SearchStatus(); total != 3 {
		t.Errorf(`(?i)0x[0-9a-f]{2} total = %d, want 3 (0xff, 0XFF, 0xFf; not 1xff)`, total)
	}
}

// TestSearchDebouncedExported guards the public SearchDebounced wrapper: with
// DebounceFor <= 0 it is immediate (equivalent to Search), so the matches are present
// synchronously.
func TestSearchDebouncedExported(t *testing.T) {
	// With DebounceFor 0 the debounced path is immediate (equivalent to Search), so the
	// matches are present synchronously without pumping the event loop.
	pv := NewWithData([]byte(`{"a":"x","b":"x"}`), FormatJSON,
		WithSearchConfig(SearchConfig{DebounceFor: 0, MinQueryLen: 1}))
	pv.SearchDebounced(SearchQuery{Text: "x"})
	if _, total, _ := pv.SearchStatus(); total != 2 {
		t.Errorf("SearchDebounced (DebounceFor 0) total = %d, want 2 (immediate)", total)
	}
}

// TestSearchError exposes a bad regex distinctly from "no matches".
func TestSearchError(t *testing.T) {
	pv := docPV(`{"a":1}`, FormatJSON)
	pv.Search(SearchQuery{Text: "(", Mode: SearchRegex})
	if pv.SearchError() == nil {
		t.Error("SearchError() = nil for an invalid regex, want an error")
	}
	// A valid pattern with no matches must clear the error.
	pv.Search(SearchQuery{Text: "zzz", Mode: SearchRegex})
	if err := pv.SearchError(); err != nil {
		t.Errorf("SearchError() = %v after a valid pattern, want nil", err)
	}
	if _, total, _ := pv.SearchStatus(); total != 0 {
		t.Errorf("valid no-match total = %d, want 0", total)
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
	pv.doc.Fold(o)

	deep := lineContaining(pv.doc, "needle")
	if deep < 0 {
		t.Fatal("could not find the deep line")
	}
	if pv.doc.Visible(deep) {
		t.Fatal("precondition: deep line should be hidden before search")
	}

	pv.Search(SearchQuery{Text: "needle"})
	if !pv.doc.Visible(deep) {
		t.Error("search did not reveal the match inside the folded node")
	}
}

// TestSearchHighlightsCollapsedFoldHead is the regression for matches on a
// collapsed fold-head being suppressed. A collapsed head still shows its head text
// (its collapsed rendering is head ++ summary ++ close) and the match columns are
// computed against that same head text, so the highlight must be drawn rather than
// skipped until the node is expanded.
func TestSearchHighlightsCollapsedFoldHead(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"needle":{"a":1,"b":2}}`), FormatJSON, 800, 600)
	defer win.Close()

	head := findFoldHead(pv.doc, `"needle"`)
	if head == model.NoNode {
		t.Fatal(`could not find the fold head whose line begins with "needle"`)
	}
	headLine := pv.doc.Nodes[head].HeadLine
	pv.doc.Fold(head)
	if !pv.doc.IsCollapsed(headLine) {
		t.Fatal("precondition: the fold head should be collapsed")
	}

	pv.Search(SearchQuery{Text: "needle"})
	if _, total, _ := pv.SearchStatus(); total == 0 {
		t.Fatal("search found no match on the head line")
	}
	if !pv.doc.IsCollapsed(headLine) {
		t.Fatal("the fold head should still be collapsed after searching its own head line")
	}
	pv.r.reflow()
	if got := len(pv.r.matchLayer.Objects); got == 0 {
		t.Error("no match highlight drawn on the collapsed fold-head (its head text is still on screen)")
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
	runes := []rune(pv.doc.DisplayString(m.Line))
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

// TestSearchHighlightBoundedHorizontal pins invariant M-1 on the horizontal axis:
// thousands of matches on ONE visible line must not each emit a highlight rect when
// most are scrolled off-screen. (TestSearchHighlightBounded covers only the
// vertical / many-lines axis.) Before the horizontal cull this drew ~K rects.
func TestSearchHighlightBoundedHorizontal(t *testing.T) {
	const k = 5000
	line := strings.Repeat("needle ", k) // one raw line, k matches
	pv, win := renderInWindow(t, []byte(line), FormatRaw, 400, 300)
	defer win.Close()

	pv.Search(SearchQuery{Text: "needle"})
	if _, total, _ := pv.SearchStatus(); total != k {
		t.Fatalf("total matches = %d, want %d", total, k)
	}
	// Scroll horizontally into the middle of the line, onto a matched region.
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(0, 1000), 0))

	rects := len(pv.r.matchLayer.Objects)
	visCols := int(pv.r.scroll.Size().Width/pv.met.CharWidth) + 2 // 1 rect/visible col is the ceiling
	if rects == 0 {
		t.Error("no match rects drawn after scrolling onto a matched region (over-culled)")
	}
	if rects > visCols {
		t.Errorf("match rects = %d exceeds the visible-column bound %d (total matches %d) — O(matches), not O(visible columns)", rects, visCols, k)
	}
}

// TestMatchRectCullsOffscreenColumns proves the horizontal cull is correct in both
// directions, not merely a count cap: a single match draws a rect only while it lies
// inside the horizontal visible window.
func TestMatchRectCullsOffscreenColumns(t *testing.T) {
	const pad = 2000
	line := strings.Repeat("x", pad) + "needle" + strings.Repeat("x", pad) // one match at col pad
	pv, win := renderInWindow(t, []byte(line), FormatRaw, 400, 300)
	defer win.Close()
	pv.Search(SearchQuery{Text: "needle"})
	if _, total, _ := pv.SearchStatus(); total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}

	// Left edge: the match (col pad) is off-screen to the right -> no rect.
	pv.r.scrollToOffset(fyne.NewPos(0, 0))
	if got := len(pv.r.matchLayer.Objects); got != 0 {
		t.Errorf("match far to the right drew %d rect(s) at scroll 0; want 0", got)
	}
	// Scroll the match into view -> exactly one rect.
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(0, pad-5), 0))
	if got := len(pv.r.matchLayer.Objects); got != 1 {
		t.Errorf("match in view drew %d rect(s); want 1", got)
	}
	// Scroll past it so it is off-screen to the left -> no rect.
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(0, pad+50), 0))
	if got := len(pv.r.matchLayer.Objects); got != 0 {
		t.Errorf("match to the left drew %d rect(s); want 0", got)
	}
}

// TestClearSearchStopsPendingTimer is the regression for the debounce-timer
// lifecycle: a pending debounced scan must be cancelled by ClearSearch (the path
// SetData uses), so a stale query can't repopulate matches after a clear/reload.
func TestClearSearchStopsPendingTimer(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON,
		WithSearchConfig(SearchConfig{DebounceFor: time.Second, MinQueryLen: 1}))
	win := test.NewWindow(pv)
	defer win.Close()

	pv.searchDebounced(SearchQuery{Text: "a"})
	if pv.searchTimer == nil {
		t.Fatal("debounce should arm a timer")
	}
	pv.ClearSearch()
	if pv.searchTimer != nil {
		t.Error("ClearSearch must stop and clear the pending debounce timer")
	}
}

// TestDestroyStopsPendingTimer checks teardown cancels the debounce timer and sets
// the guard flag, so the AfterFunc can't fire a scan after the widget is gone.
func TestDestroyStopsPendingTimer(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON,
		WithSearchConfig(SearchConfig{DebounceFor: time.Second, MinQueryLen: 1}))
	win := test.NewWindow(pv)
	defer win.Close()

	pv.searchDebounced(SearchQuery{Text: "a"})
	if pv.searchTimer == nil {
		t.Fatal("debounce should arm a timer")
	}
	pv.r.Destroy()
	if !pv.destroyed.Load() {
		t.Error("Destroy must set the destroyed guard flag")
	}
	if pv.searchTimer != nil {
		t.Error("Destroy must stop the pending debounce timer")
	}
}

// TestSearchGenerationInvalidatesStaleScan checks the generation counter that
// makes an already-fired-but-queued debounce callback recognize it has been
// superseded: both a newer debounce and ClearSearch/SetData bump the generation,
// so the queued closure's captured gen no longer matches and it skips itself.
func TestSearchGenerationInvalidatesStaleScan(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON,
		WithSearchConfig(SearchConfig{DebounceFor: time.Second, MinQueryLen: 1}))
	win := test.NewWindow(pv)
	defer win.Close()

	g0 := pv.searchGen
	pv.searchDebounced(SearchQuery{Text: "a"})
	if pv.searchGen == g0 {
		t.Error("searchDebounced must bump the search generation to supersede earlier scans")
	}
	g1 := pv.searchGen
	pv.ClearSearch()
	if pv.searchGen == g1 {
		t.Error("ClearSearch must bump the search generation to invalidate a queued scan")
	}
	// SetData goes through ClearSearch, so it bumps too.
	g2 := pv.searchGen
	pv.SetData([]byte(`{"b":2}`), FormatJSON)
	if pv.searchGen == g2 {
		t.Error("SetData (via ClearSearch) must bump the search generation")
	}
}

// TestSearchSupersedesPendingDebounce is the regression for the Enter-applies-
// immediately path: a synchronous Search() (the search bar's OnSubmitted and every
// public host call) must cancel any pending debounce timer and bump the generation,
// so a keystroke timer armed just before Enter can't re-run the scan and snap the
// viewport back to match #1.
func TestSearchSupersedesPendingDebounce(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":"alpha","b":"alpha"}`), FormatJSON,
		WithSearchConfig(SearchConfig{DebounceFor: time.Second, MinQueryLen: 1}))
	win := test.NewWindow(pv)
	defer win.Close()

	pv.searchDebounced(SearchQuery{Text: "alpha"})
	if pv.searchTimer == nil {
		t.Fatal("debounce should arm a timer")
	}
	gen := pv.searchGen
	pv.Search(SearchQuery{Text: "alpha"})
	if pv.searchTimer != nil {
		t.Error("a synchronous Search must stop the pending debounce timer")
	}
	if pv.searchGen <= gen {
		t.Errorf("a synchronous Search must bump searchGen (was %d, now %d) to invalidate a queued scan",
			gen, pv.searchGen)
	}
}

// TestSearchMaxMatchesCap guards the load-bearing match cap: a scan must stop at
// MaxMatches, store no more than the cap, and report capped; a scan below the cap
// must report capped=false. (Previously only logged, never asserted.)
func TestSearchMaxMatchesCap(t *testing.T) {
	const cap = 50
	const occ = cap + 25
	src := strings.Repeat("x ", occ) // one raw line, occ occurrences of "x"

	capped := NewWithData([]byte(src), FormatRaw,
		WithSearchConfig(SearchConfig{MaxMatches: cap, MinQueryLen: 1}))
	capped.Search(SearchQuery{Text: "x"})
	if _, total, isCapped := capped.SearchStatus(); total != cap || !isCapped {
		t.Errorf("capped scan: total=%d capped=%v, want total=%d capped=true", total, isCapped, cap)
	}
	if got := len(capped.search.matches); got != cap {
		t.Errorf("stored %d matches, want exactly the cap %d", got, cap)
	}

	uncapped := NewWithData([]byte(src), FormatRaw,
		WithSearchConfig(SearchConfig{MaxMatches: 1000, MinQueryLen: 1}))
	uncapped.Search(SearchQuery{Text: "x"})
	if _, total, isCapped := uncapped.SearchStatus(); isCapped || total != occ {
		t.Errorf("uncapped scan: total=%d capped=%v, want total=%d capped=false", total, isCapped, occ)
	}
}
