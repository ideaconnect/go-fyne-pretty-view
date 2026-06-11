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
	// SearchPlain matches the query as a literal substring (the default).
	SearchPlain SearchMode = iota
	// SearchRegex matches the query as a Go regular expression (regexp/RE2). RE2
	// matches in time linear in the input, so a host-supplied pattern cannot cause
	// catastrophic backtracking (no ReDoS); an invalid pattern is reported via
	// SearchError. A pathological pattern can still be expensive to compile, so treat
	// the pattern as you would any host input.
	SearchRegex
)

// SearchQuery describes an incremental search request.
type SearchQuery struct {
	Text          string
	Mode          SearchMode
	CaseSensitive bool
}

// Match is one search hit, in model coordinates: a stable display-line index and a
// rune column range [ColStart,ColEnd) into that line's expanded text. All three are
// int. Keying by line (not visible row) makes a match survive folding; the visible
// row is an O(log n) lookup. Retrieve the current hits with PrettyView.Matches.
type Match struct {
	Line     int
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

// Search starts or replaces the active search and reveals the first match. It is the
// IMMEDIATE path: it scans synchronously and returns once matches are computed, and it
// never debounces. For per-keystroke input from a host search field, prefer
// SearchDebounced (which coalesces a burst into one scan); use Search when you want
// the scan to happen now. q.Mode selects SearchPlain or SearchRegex. Call it on the
// Fyne goroutine.
func (pv *PrettyView) Search(q SearchQuery) {
	pv.runSearch(q)
}

// SearchDebounced runs Search(q) after the configured SearchConfig.DebounceFor delay,
// coalescing a burst of rapid calls (e.g. one per keystroke from a host's own search
// field) so only the last query in the burst scans. With DebounceFor <= 0 it is
// equivalent to Search (immediate); since WithSearchConfig treats a zero DebounceFor as
// "keep the 150ms default", pass a NEGATIVE DebounceFor (or call Search) to disable
// debouncing. Last call wins: a newer SearchDebounced / Search / ClearSearch / SetData
// supersedes a still-pending scan. Call it on the Fyne goroutine.
func (pv *PrettyView) SearchDebounced(q SearchQuery) { pv.searchDebounced(q) }

// searchDebounced coalesces rapid keystrokes: it waits cfg.search.DebounceFor before
// running the (synchronous) scan on the Fyne goroutine, so typing a word triggers one
// scan instead of one per character. A non-positive DebounceFor runs immediately.
func (pv *PrettyView) searchDebounced(q SearchQuery) {
	pv.stopSearchTimer()
	// Bump the generation so that any earlier debounced scan that has ALREADY fired
	// and queued its fyne.Do closure (which Stop can no longer cancel) is recognized
	// as stale and skips itself.
	pv.searchGen++
	d := pv.cfg.search.DebounceFor
	if d <= 0 {
		pv.Search(q)
		return
	}
	gen := pv.searchGen
	pv.searchTimer = time.AfterFunc(d, func() {
		// The AfterFunc runs on its own goroutine; marshal the scan onto the Fyne
		// thread via fyne.Do. Correctness here is load-bearing on fyne.Do enqueuing
		// onto the main event loop (glfw): that serializes pv.Search's mutation of
		// pv.search.* with reflow/rebuildMatches, which also run on the Fyne
		// goroutine. The destroyed flag (atomic, the only field this goroutine reads)
		// drops a callback that fired after teardown — Stop cannot cancel one already
		// fired. Inside fyne.Do we re-check destroyed and the generation, so a scan
		// superseded by a newer keystroke / ClearSearch / SetData does not run.
		if pv.destroyed.Load() {
			return
		}
		fyne.Do(func() {
			if pv.destroyed.Load() || gen != pv.searchGen {
				return
			}
			pv.Search(q)
		})
	})
}

// stopSearchTimer cancels and clears any pending debounced scan. Best-effort:
// Timer.Stop cannot cancel a callback that has already fired (the destroyed guard
// covers that window). Must be called on the Fyne goroutine; pv.searchTimer is
// only ever touched there (searchDebounced / ClearSearch / Destroy).
func (pv *PrettyView) stopSearchTimer() {
	if pv.searchTimer != nil {
		pv.searchTimer.Stop()
		pv.searchTimer = nil
	}
}

// SearchNext moves to the next match (wrapping) and reveals it.
func (pv *PrettyView) SearchNext() { pv.step(+1) }

// SearchPrev moves to the previous match (wrapping) and reveals it.
func (pv *PrettyView) SearchPrev() { pv.step(-1) }

// ClearSearch clears search state and highlights. It also cancels any pending
// debounced scan so a stale query can't repopulate matches after a clear (this is
// the path SetData uses, so loading new data drops the old document's pending
// search too).
func (pv *PrettyView) ClearSearch() {
	pv.searchGen++ // invalidate any in-flight debounced scan (pending OR already queued)
	pv.stopSearchTimer()
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

// Matches returns a snapshot of the current search hits in document order (nil if
// there is no active search), letting a host build its own match list or minimap.
// The returned slice is a copy — mutating it does not affect the viewer, and it is
// not updated as the document or search changes; call Matches again after a Search.
func (pv *PrettyView) Matches() []Match {
	if len(pv.search.matches) == 0 {
		return nil
	}
	out := make([]Match, len(pv.search.matches))
	copy(out, pv.search.matches)
	return out
}

// SearchError reports the error from the most recent Search / SearchDebounced, or nil.
// The only error is an invalid regular expression (a SearchRegex query whose pattern
// does not compile); it lets a caller distinguish "the pattern is bad" from "the
// pattern is valid but matched nothing". It is cleared by the next Search/ClearSearch.
func (pv *PrettyView) SearchError() error { return pv.search.err }

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
//
// It runs on the Fyne goroutine, not a worker: the cost is O(total bytes) with a
// single forward byte->rune pass per line (see colCursor) and a hard MaxMatches
// cap, so it stays well under a frame even on large input — a full PLAIN scan of
// the 7.5 MB / 440k-line fixture is single-digit milliseconds (~5 ms, hardware
// dependent). Regex (SearchRegex) is heavier per line; a literal-prefix prefilter
// (re.LiteralPrefix) skips the RE2 engine on non-candidate lines, but a pattern with
// no literal head still runs the engine on every line. Combined with keystroke
// debouncing (searchDebounced), a cooperative chunked/off-thread scan is unnecessary
// (and would reintroduce the Search-vs-reflow data race that staying on the Fyne thread
// avoids). The ceiling is a single pathological multi-gigabyte document; such input
// is out of scope for an in-memory viewer.
func (pv *PrettyView) runSearch(q SearchQuery) {
	// A synchronous scan is the authoritative supersede point. Cancel any pending
	// debounce timer and bump the generation so an already-fired-but-queued
	// debounced scan (which Timer.Stop can no longer cancel) recognizes itself as
	// stale (gen != pv.searchGen) and skips itself. Without this, the
	// Enter-applies-immediately path (controls.go OnSubmitted -> Search) leaves a
	// keystroke timer armed; it then re-runs the scan, resets the active match to 0
	// and re-centers, yanking the viewport back to match #1 after the user navigated.
	pv.stopSearchTimer()
	pv.searchGen++

	pv.search.query = q
	pv.search.matches = pv.search.matches[:0]
	clear(pv.search.byLine) // reuse the map across scans (nil-safe); indexMatches refills it
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
	// Literal-prefix prefilter: LiteralPrefix returns a byte sequence that EVERY match
	// of re must begin with (Go's documented contract), so a line not containing it
	// cannot match and the (far costlier) RE2 engine is skipped for it via bytes.Index
	// — turning an O(lines) engine sweep into a cheap scan for the common "regex with a
	// literal head" case (e.g. `item\d+`, or a pattern that is effectively a literal).
	// This is correct regardless of case sensitivity: for a case-insensitive (?i)
	// pattern the prefix is the run of caseless leading characters (empty for a cased
	// head like `foo`, but e.g. "0x" for `(?i)0xFF`), and that run is still a literal
	// every match must contain. An empty prefix simply disables the filter.
	var rePrefix []byte
	if re != nil {
		if lp, _ := re.LiteralPrefix(); lp != "" {
			rePrefix = []byte(lp)
		}
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
			if rePrefix != nil && bytes.Index(scratch, rePrefix) < 0 {
				continue // line cannot contain the required literal prefix — skip RE2
			}
			// FindAllIndex evaluates the pattern against the whole line, so anchors
			// (^ $) and word boundaries (\b \B) see full context. A from-offset
			// FindIndex(scratch[from:]) loop is wrong here: re-slicing the suffix
			// gives the engine a fresh start-of-text, which re-anchors ^ at every
			// resume point and hides the byte to the left of \b/\B. The n argument
			// bounds the per-line slice to the remaining match budget (so a single
			// pathological line can't allocate past the cap); zero-width matches
			// (e.g. \b, or o* at a non-o) carry nothing to highlight and are skipped.
			cur := colCursor{hay: scratch}
			for _, loc := range re.FindAllIndex(scratch, limit-len(pv.search.matches)) {
				if loc[0] == loc[1] {
					continue
				}
				pv.addMatchB(li, &cur, loc[0], loc[1])
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
	cur := colCursor{hay: hay}
	from := 0
	for {
		idx := bytes.Index(hay[from:], needle)
		if idx < 0 {
			return
		}
		bs := from + idx
		pv.addMatchB(li, &cur, bs, bs+len(needle))
		from = bs + len(needle)
		if len(pv.search.matches) >= limit {
			pv.search.capped = true
			return
		}
	}
}

// colCursor converts byte offsets within one line's bytes to rune columns. Matches
// are found in increasing byte order, so the cursor advances forward and counts
// only the runes between successive offsets — making a whole line O(line length)
// instead of O(matches * line length) (the old per-match utf8.RuneCount-from-zero).
type colCursor struct {
	hay    []byte
	byteAt int
	runeAt int
}

// col returns the rune column at byte offset off, advancing the cursor to it. ASCII
// bytes (the common case) advance byte and column in lockstep without decoding.
func (c *colCursor) col(off int) int {
	if off < c.byteAt { // non-monotonic (shouldn't happen): recount safely
		c.byteAt, c.runeAt = 0, 0
	}
	for c.byteAt < off && c.byteAt < len(c.hay) {
		if c.hay[c.byteAt] < utf8.RuneSelf {
			c.byteAt++
		} else {
			_, sz := utf8.DecodeRune(c.hay[c.byteAt:])
			c.byteAt += sz
		}
		c.runeAt++
	}
	return c.runeAt
}

// addMatchB records a match at byte range [bs,be) of the cursor's line, converting
// to rune columns via the forward cursor.
func (pv *PrettyView) addMatchB(li int32, cur *colCursor, bs, be int) {
	cs := cur.col(bs)
	ce := cur.col(be)
	pv.search.matches = append(pv.search.matches, Match{Line: int(li), ColStart: cs, ColEnd: ce})
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
	// runSearch already cleared byLine for reuse; only allocate on the first scan.
	if pv.search.byLine == nil {
		pv.search.byLine = make(map[int32][]int, len(pv.search.matches))
	}
	for i, m := range pv.search.matches {
		pv.search.byLine[int32(m.Line)] = append(pv.search.byLine[int32(m.Line)], i)
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
	pv.doc.RevealLine(int32(m.Line))

	if pv.r == nil {
		return
	}
	pv.r.scroll.Content.Resize(pv.contentSize())
	pv.centerOnLine(int32(m.Line), m.ColStart) // scrollToOffset -> reflow redraws rows + highlights
}

func (pv *PrettyView) refreshMatchesView() {
	if pv.r == nil {
		return
	}
	pv.r.rebuildMatches(pv.r.firstRow, pv.r.lastRow)
}
