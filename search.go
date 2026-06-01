package prettyview

import (
	"bytes"
	"regexp"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
)

// SearchMode selects plain-substring or regular-expression matching.
type SearchMode uint8

const (
	SearchPlain SearchMode = iota
	SearchRegex
)

// SearchQuery describes an incremental search request.
type SearchQuery struct {
	Text          string
	Mode          SearchMode
	CaseSensitive bool
}

// Match is one search hit, in model coordinates: a stable display-line index and
// a rune column range into that line's expanded text. Keying by line (not visible
// row) makes matches survive folding; the visible row is an O(log n) lookup.
type Match struct {
	Line     int32
	ColStart int
	ColEnd   int
}

type searchState struct {
	query   SearchQuery
	matches []Match
	byLine  map[int32][]int // line -> indices into matches
	active  int             // -1 if none
	capped  bool
	err     error
}

// SetOnSearchRequested registers a callback invoked when the user presses the
// search shortcut (Ctrl/Cmd+F), so a host can focus its search field.
func (pv *PrettyView) SetOnSearchRequested(fn func()) { pv.onSearchRequested = fn }

// Search starts or replaces the active search and reveals the first match. It
// runs synchronously and returns once matches are computed.
func (pv *PrettyView) Search(q SearchQuery) {
	pv.runSearch(q)
}

// searchDebounced coalesces rapid keystrokes: it waits cfg.search.DebounceFor
// before running the (synchronous) scan on the Fyne goroutine, so typing a word
// triggers one scan instead of one per character. A non-positive DebounceFor runs
// immediately.
func (pv *PrettyView) searchDebounced(q SearchQuery) {
	if pv.searchTimer != nil {
		pv.searchTimer.Stop()
	}
	d := pv.cfg.search.DebounceFor
	if d <= 0 {
		pv.Search(q)
		return
	}
	pv.searchTimer = time.AfterFunc(d, func() {
		fyne.Do(func() { pv.Search(q) })
	})
}

// SearchNext moves to the next match (wrapping) and reveals it.
func (pv *PrettyView) SearchNext() { pv.step(+1) }

// SearchPrev moves to the previous match (wrapping) and reveals it.
func (pv *PrettyView) SearchPrev() { pv.step(-1) }

// ClearSearch clears search state and highlights.
func (pv *PrettyView) ClearSearch() {
	pv.search = searchState{active: -1}
	pv.refreshMatchesView()
	pv.notifySearch()
}

func (pv *PrettyView) notifySearch() {
	if pv.onSearchChanged != nil {
		pv.onSearchChanged()
	}
}

// SearchStatus returns the active 1-based index, the total match count, and
// whether the count was capped.
func (pv *PrettyView) SearchStatus() (active, total int, capped bool) {
	if pv.search.active < 0 {
		return 0, len(pv.search.matches), pv.search.capped
	}
	return pv.search.active + 1, len(pv.search.matches), pv.search.capped
}

func (pv *PrettyView) step(dir int) {
	n := len(pv.search.matches)
	if n == 0 {
		return
	}
	pv.search.active = ((pv.search.active+dir)%n + n) % n
	pv.revealActive()
	pv.notifySearch()
}

// runSearch performs a synchronous, capped scan of the document's expanded line
// text. (Matches are found in expanded text so a hit inside a folded node can be
// revealed; the highlight is only drawn once the line is visible.)
func (pv *PrettyView) runSearch(q SearchQuery) {
	pv.search.query = q
	pv.search.matches = pv.search.matches[:0]
	pv.search.byLine = nil
	pv.search.active = -1
	pv.search.capped = false
	pv.search.err = nil

	if pv.doc == nil || len(q.Text) < pv.minQueryLen() {
		pv.refreshMatchesView()
		pv.notifySearch()
		return
	}

	var re *regexp.Regexp
	if q.Mode == SearchRegex {
		pat := q.Text
		if !q.CaseSensitive {
			pat = "(?i)" + pat
		}
		r, err := regexp.Compile(pat)
		if err != nil {
			pv.search.err = err
			pv.refreshMatchesView()
			pv.notifySearch()
			return
		}
		re = r
	}
	needleBytes := []byte(q.Text)
	var needleLower []byte
	if re == nil && !q.CaseSensitive {
		needleLower = bytes.ToLower(needleBytes)
	}
	limit := pv.cfg.search.MaxMatches
	if limit <= 0 {
		limit = 10000
	}

	// The scan walks the whole line arena (so hits inside folded nodes are
	// findable) using a single reused scratch buffer — no per-line string or
	// lowercase allocation on the common ASCII path.
	var scratch []byte
	for li := int32(0); li < int32(len(pv.doc.Lines)); li++ {
		scratch = pv.doc.AssembleLine(li, scratch[:0])
		switch {
		case re != nil:
			for _, loc := range re.FindAllIndex(scratch, -1) {
				if loc[1] == loc[0] {
					continue // skip zero-width matches
				}
				pv.addMatchB(li, scratch, loc[0], loc[1])
				if len(pv.search.matches) >= limit {
					pv.search.capped = true
					break
				}
			}
		case q.CaseSensitive:
			pv.scanPlain(li, scratch, needleBytes, limit)
		case isASCII(scratch):
			asciiLowerInPlace(scratch) // byte positions unchanged → columns stay correct
			pv.scanPlain(li, scratch, needleLower, limit)
		default:
			// Non-ASCII, case-insensitive: Unicode-correct fold (rare; allocates).
			pv.scanPlain(li, bytes.ToLower(scratch), needleLower, limit)
		}
		if pv.search.capped {
			break
		}
	}

	pv.indexMatches()
	if len(pv.search.matches) > 0 {
		pv.search.active = 0
		pv.revealActive()
	} else {
		pv.refreshMatchesView()
	}
	pv.notifySearch()
}

func (pv *PrettyView) minQueryLen() int {
	if pv.cfg.search.MinQueryLen > 0 {
		return pv.cfg.search.MinQueryLen
	}
	return 1
}

// scanPlain finds every occurrence of needle in hay (byte offsets) and records
// them as matches on line li, stopping at the match cap.
func (pv *PrettyView) scanPlain(li int32, hay, needle []byte, limit int) {
	if len(needle) == 0 {
		return
	}
	from := 0
	for {
		idx := bytes.Index(hay[from:], needle)
		if idx < 0 {
			return
		}
		bs := from + idx
		pv.addMatchB(li, hay, bs, bs+len(needle))
		from = bs + len(needle)
		if len(pv.search.matches) >= limit {
			pv.search.capped = true
			return
		}
	}
}

// addMatchB records a match at byte range [bs,be) of hay, converting to rune
// columns into the line's displayed text.
func (pv *PrettyView) addMatchB(li int32, hay []byte, bs, be int) {
	cs := utf8.RuneCount(hay[:bs])
	ce := cs + utf8.RuneCount(hay[bs:be])
	pv.search.matches = append(pv.search.matches, Match{Line: li, ColStart: cs, ColEnd: ce})
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

func asciiLowerInPlace(b []byte) {
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
}

func (pv *PrettyView) indexMatches() {
	pv.search.byLine = make(map[int32][]int, len(pv.search.matches))
	for i, m := range pv.search.matches {
		pv.search.byLine[m.Line] = append(pv.search.byLine[m.Line], i)
	}
}

// revealActive expands any collapsed ancestors of the active match, then scrolls
// it to the center of the viewport. Order is load-bearing: expand, recompute the
// projection, resolve the row, then scroll.
func (pv *PrettyView) revealActive() {
	if pv.search.active < 0 || pv.search.active >= len(pv.search.matches) {
		pv.refreshMatchesView()
		return
	}
	m := pv.search.matches[pv.search.active]
	pv.doc.RevealLine(m.Line)

	if pv.r == nil {
		return
	}
	pv.r.scroll.Content.Resize(pv.contentSize())
	pv.centerOnLine(m.Line, m.ColStart) // scrollToOffset -> reflow redraws rows + highlights
}

func (pv *PrettyView) refreshMatchesView() {
	if pv.r == nil {
		return
	}
	pv.r.rebuildMatches(pv.r.firstRow, pv.r.lastRow)
}
