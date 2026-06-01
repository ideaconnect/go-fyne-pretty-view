> **Status — all 9 findings resolved.** Tracked as phase "7 · Performance Hardening — Response Viewer (prettyview)" in the Helena Asana plan. Measured deltas after the fixes:
>
> | Path | Before | After |
> |---|---|---|
> | Search keystroke (big.json) | 6.6 ms · 190k allocs | 5.0 ms · **10k allocs** (−95%) + debounced (1 scan/word) |
> | SearchNext under a collapsed ancestor | 2.95 ms · 1.84 MB / step | **5 ns · 0 allocs / step** |
> | Row builds per reflow | 2–3× per row | **1× per row** |
> | Parse big.json (transient) | ~270 MB churn | **106 MB** |
> | Fold / projection lookup | 0 allocs | 0 allocs (unchanged) |
>
> Regression guards added: `TestReflowBuildsEachRowOnce`, `TestRevealLineMatchesRebuild` (perf_test.go). The original review follows.

---

# prettyview — Performance Review

**Scope:** virtualized JSON/XML/HTML/raw viewer. Hot paths reviewed against the pinned Fyne v2.7.4. Hardware: AMD Ryzen AI MAX+ 395, Go 1.26.3, linux/amd64. Fixtures: `big.json` ≈ 7.5 MB / 440k display rows, `openapi.json` ≈ 478 KB.

## Executive summary

The widget's core design is sound and genuinely fast: virtualization keeps reflow at ~58 µs and the Fenwick projection/incremental-fold primitives are O(log n) and allocation-free at 440k rows. Scrolling a 7.5 MB document is not the problem. The one thing that actually matters is **incremental search: every keystroke runs a synchronous, full-document scan that materializes two fresh strings per line (6.6 ms / 190k allocs per keystroke on big.json), with no debounce** — so typing a 5-letter word triggers ~5 full scans on the UI goroutine. Secondary: **per-frame rows are rebuilt 2–3× redundantly** (a `setLine`→Refresh build plus an explicit `rowLayer.Refresh()` that re-builds the same rows, plus a third rebuild on programmatic scroll), and **SearchNext does a full O(n) Fenwick rebuild per step when the match is under a collapsed ancestor** (2.95 ms/step). None of the per-frame waste breaches the 16 ms budget today, but the search path is user-visible jank.

## Ranked findings

| Rank | Severity | Hotness | Caveat | Location | Fix (one line) |
|--:|---|---|---|---|---|
| 1 | High | per-keystroke | Full-doc scan with no debounce; fires synchronously on every keystroke | controls.go:128; search.go:92-176 | Honor `cfg.DebounceFor` via `time.AfterFunc` + `fyne.Do` |
| 2 | High | per-keystroke | Per scan: a fresh string + a `strings.ToLower` string for **every** line | search.go:130-146; model.go:206-216 | Scan bytes via reused scratch `[]byte`; lowercase needle once |
| 3 | High | per-frame | Each visible row is built **twice** per reflow (`setLine`→Refresh + `rowLayer.Refresh()`) | renderer.go:129,133; row.go:33-36,79-82 | One driver: republish Objects + `canvas.Refresh(rowLayer)`, drop container Refresh |
| 4 | Medium | per-interaction | Programmatic scroll adds a **third** rebuild via `scroll.Refresh()`→`Content.Refresh()` | renderer.go:158-162; widget_input.go:78-85 | Use `scroll.ScrollToOffset(p)` (updates bars, no Content cascade) |
| 5 | Medium | per-interaction | SearchNext under a collapsed ancestor → full O(n) Fenwick rebuild + realloc per step | search.go:207; foldindex.go:251-266,104-114 | Incremental `unfold` outermost-first; reuse Fenwick backing array |
| 6 | Low | per-frame | `build()` does a `[]rune` round-trip of each whole segment even when fully visible & ASCII | row.go:124,139 | Fast-path `a==segStart && b==segEnd` → `string(segBytes(seg))`, no `[]rune` |
| 7 | Low | per-frame | `build()` calls `t.Resize(t.MinSize())` → whole-string font measure per segment | row.go:147 | Size from the monospace grid: `NewSize(runeCount*charWidth, rowH)` |
| 8 | Low | per-frame | Three fresh `[]fyne.CanvasObject` slices per reflow (`liveObjects` + 2×`applyRects`) | renderer.go:140; highlight.go:127 | Reuse one backing slice per layer; truncate-and-refill |
| 9 | Low | one-time | Parse arenas start nil-cap → ~270 MB transient / 800k allocs for 7.5 MB | builder.go:43-49 | Pre-size from `len(src)`: Lines/Nodes≈src/16, Segs≈src/8 |

---

## 1. Search runs synchronously on every keystroke with no debounce (High, per-keystroke)

**Trigger:** any character typed in the search box on a large document.

**Cost:** each keystroke is a full O(n) scan — **6.6 ms / 2.78 MB / 190,012 allocs** on big.json (measured, `SearchKeystrokeBig`). With default `MinQueryLen=1` (search.go:100, options.go:22) the *first* character — the one that matches the most lines — already triggers the most expensive scan; a 5-char word ≈ 5 full scans on the UI goroutine.

**Evidence:** `controls.go:128` — `entry.OnChanged = func(s string){ pv.Search(SearchQuery{Text:s}) }` calls `runSearch` directly, no timer/goroutine/cancellation. `grep` confirms `DebounceFor`/`ChunkBytes` (options.go:12-13, 20-21) are read **nowhere** — the documented 150 ms debounce does not exist.

**Fix:** coalesce keystrokes; marshal the actual scan back onto the UI goroutine.

```go
// controls.go
entry.OnChanged = func(s string) {
    if pv.searchTimer != nil { pv.searchTimer.Stop() }
    pv.searchTimer = time.AfterFunc(pv.cfg.DebounceFor, func() {
        fyne.Do(func() { pv.Search(SearchQuery{Text: s}) })
    })
}
```

Collapses N scans/word into 1. Independent of (and complementary to) finding 2.

## 2. Each scan allocates a per-line string + a `strings.ToLower` string for every line (High, per-keystroke)

**Trigger:** every `runSearch` (so every debounced keystroke).

**Cost:** the loop bound is `len(pv.doc.Lines)` (search.go:130) — the **whole** line arena, not the viewport. Per line: `lineString`→`segsText` does `make([]byte,0,n)` + `string(buf)` (model.go:211-215) = 1 alloc; then the default case-insensitive path does `hay = strings.ToLower(text)` (search.go:146) = a 2nd alloc. CPU profile: ~37% `lineString`/`segsText`, ~17% `slicebytetostring`, ~12% `strings.ToLower`.

**Evidence:** search.go:131, search.go:144-146; model.go:206-216.

**Fix:** match over bytes with one reused scratch buffer; lowercase the needle once. Search deliberately scans **expanded** text (`lineSegs`, not `displaySegs`) so hits inside folds are findable — preserve that. `addMatch` (search.go:185-188) converts byte offsets → rune columns via `utf8.RuneCountInString`, so keep feeding it byte offsets into the assembled line.

```go
needleLower := bytes.ToLower([]byte(needle))
var scratch []byte
for li := ...; li++ {
    scratch = scratch[:0]
    for _, seg := range pv.doc.lineSegs(li) {
        scratch = append(scratch, pv.doc.segBytes(seg)...)
    }
    // ASCII-fold compare against needleLower, or bytes.Index after lowercasing scratch in place
}
```

Removes essentially all 190k allocs and most of the 6.6 ms.

## 3. Every visible row is built twice per reflow (High, per-frame)

**Trigger:** every `OnScrolled` / Layout / Refresh / programmatic scroll.

**Cost:** ~40 visible rows do ~80 `build()` calls + ~80 `canvas.Refresh(row)` enqueues per frame instead of ~40. The doubled cost is the `[]rune` allocs (finding 6), the object-list rebuild, and the Refresh enqueues; text *shaping* is not doubled because the 2nd `MinSize` is a font-metrics cache hit. Within budget today (~58 µs/reflow) but pure waste.

**Evidence:** `reflow` calls `rw.setLine(...)` for every idx (renderer.go:129), and `setLine` (row.go:33-36) unconditionally calls `r.Refresh()`→`rowRenderer.Refresh`→`build()` (build #1). Then renderer.go:133 calls `r.rowLayer.Refresh()`; Fyne `Container.Refresh` (container.go:108) does `for _, child := range c.Objects { child.Refresh() }`, routing each `rowWidget` back into `rowRenderer.Refresh`→`build()` (build #2).

**Fix:** pick one driver. Keep `setLine` doing the build and replace `rowLayer.Refresh()` with publishing `Objects` + a single `canvas.Refresh(r.rowLayer)` (not container Refresh, so children aren't re-built); or strip `Refresh()` out of `setLine` and let one `rowLayer.Refresh()` drive exactly one build per row. Either yields identical pixels with one build/row/frame.

## 4. Programmatic scroll triggers a third full row rebuild (Medium, per-interaction)

**Trigger:** any fold/unfold, SearchNext/Prev reveal, or `ExpandTo` (anything that scrolls programmatically).

**Cost:** on top of finding 3's double build, `scroll.Refresh()` cascades into a third rebuild of all ~40 rows **plus** redundant `selLayer`/`matchLayer` refreshes.

**Evidence:** `scrollToOffset` sets `r.scroll.Offset = p; r.scroll.Refresh()` then `reflow()` (renderer.go:158-162); `refreshContent` does the same (widget_input.go:78-85). Fyne `Scroll.Refresh` (scroller.go:551) calls `s.Content.Refresh()`; the content is the 3-layer container (renderer.go:48), so its `Container.Refresh` rebuilds `selLayer`, `matchLayer`, **and** `rowLayer`. The following `reflow()` already republishes everything.

**Caveat:** `scroll.Refresh()` there also updates the scrollbar thumbs after mutating `Offset`, so it cannot simply be deleted.

**Fix:** replace `r.scroll.Offset = p; r.scroll.Refresh()` with `r.scroll.ScrollToOffset(p)` — verified in Fyne (scroller.go:572 sets Offset and updates bars **without** the Content.Refresh cascade), then keep `reflow()`. The content cascade is fully redundant with reflow.

## 5. SearchNext does a full O(n) Fenwick rebuild per step under a collapsed ancestor (Medium, per-interaction)

**Trigger:** Next/Prev (or `ExpandTo`) when the active match's line is hidden under a collapsed ancestor.

**Cost:** expanded doc — **210 µs, no rebuild** (the common case is fine). Collapsed doc — **2.95 ms / 1.84 MB per step** (`SearchNextCollapsedBig`); stepping through N folded matches is N × 2.95 ms.

**Evidence:** `revealLine` early-returns when `vis[line]==1` (foldindex.go:252). Otherwise it clears ancestor collapsed bits (foldindex.go:256-261) and calls `rebuild(d)` (foldindex.go:263), which re-walks all 440k lines (foldindex.go:118-149) and reallocates the whole Fenwick via `newFenwick` (foldindex.go:106).

**Fix — two independent wins:**
1. In `buildFenwick` (foldindex.go:104-114) zero+refill the existing `fi.bit.tree` instead of `newFenwick` allocating fresh — kills the 1.84 MB/step alloc for **all** bulk-rebuild paths.
2. `revealLine` already enumerates the exact ancestors it un-collapses. Replace the global rebuild with incremental `fi.unfold(d, a)` per ancestor (`FoldToggle` = 138 ns / 0 allocs). **Critical ordering:** `unfold`'s precondition is "node currently visible" and it only restores lines with `hiddenBy==node`; the walk runs innermost→outermost (foldindex.go:256), so you must collect the collapsed ancestors and unfold **outermost-first** (reverse), or you corrupt the projection. Apply the same change to `expandAncestors` (foldindex.go:268-282), which is currently uncalled but identical. Ship (1) first; gate (2) behind a test asserting identical `vis[]` after both paths.

## 6. `build()` does a `[]rune` round-trip of each whole segment per row per frame (Low, per-frame)

**Trigger:** every visible segment of every visible row, every reflow.

**Cost:** `RowBuildWide` = 4 segs → 4 allocs (~1 alloc/segment); the bulk of the ~199 allocs/reflow. Absolute cost tiny (~58–105 µs/reflow), squarely on the per-frame path, trivially avoidable.

**Evidence:** row.go:124 `runes := []rune(string(pv.doc.segBytes(seg)))` allocates a rune slice for the whole segment even when `[a,b)` covers it entirely (no horizontal cull — the common case) and the bytes are ASCII; row.go:139 slices it.

**Fix:** fast-path the no-cull case — if `a==segStart && b==segEnd`, set `t.Text = string(d.segBytes(seg))` with no `[]rune`. Only when the segment straddles the column window walk `utf8.DecodeRune` to the cut offsets (for ASCII, byte offset == rune offset, so slice the bytes directly). Column math stays rune-based, so culling is unchanged.

## 7. `build()` asks Fyne to measure each segment via `t.Resize(t.MinSize())` (Low, per-frame)

**Trigger:** every visible segment of every visible row, every reflow.

**Cost:** `canvas.Text.MinSize()` (Fyne text.go:42) → `RenderedTextSize` → `cache.GetFontMetrics` keyed by a `fontSizeEntry{Text: fullString}` (cache/text.go:19-21) — i.e. it hashes the **entire** segment string on every call, and on a miss runs full text shaping. After a horizontal scroll the culled substring changes every frame → guaranteed cache misses (full shaping) **and** unbounded cache growth. Today this is mostly cache hits (text rarely changes culling), but it is needless and the horizontal-scroll case is a real cliff.

**Evidence:** row.go:147 `t.Resize(t.MinSize())`.

**Fix:** the view is a strict monospace grid with integral `m.charWidth`/`m.rowH` (geometry.go). Size directly from the rune count already computed during culling:

```go
t.Resize(fyne.NewSize(float32(b-a)*m.charWidth, m.rowH))
```

Pixel-exact (the whole coordinate system is built on integral `charWidth`), removes the whole-string hash, the shaping-on-miss, and the unbounded cache churn.

## 8. Three fresh `[]fyne.CanvasObject` slices per reflow (Low, per-frame)

**Trigger:** every reflow (and one extra `applyRects` per drag-move).

**Cost:** part of the ~199 allocs / 2.1 KB per reflow; O(viewport), not O(doc). Pure per-frame churn, well inside budget.

**Evidence:** `liveObjects` does `make([]fyne.CanvasObject, 0, len(r.live))` (renderer.go:140); `applyRects` does `make([]fyne.CanvasObject, n)` (highlight.go:127) for the selection layer and again for the match layer, each followed by an unconditional `layer.Refresh()`.

**Fix:** keep three reusable backing slices on the renderer (one **per layer** — Fyne holds `layer.Objects` by reference, so sharing would alias the layers). `buf = buf[:0]; append(...); layer.Objects = buf`. Optionally skip the assignment + Refresh when `n` and the row range are unchanged.

## 9. Parse arenas start at nil capacity (Low, one-time)

**Trigger:** one-time `SetData`/parse.

**Cost:** **155 ms / 270 MB transient / 800k allocs** for 7.5 MB (final model ≈ 38–48 MB; the rest is doubling churn). `alloc_space` profile: `Builder.Leaf` 38%, `Open` 20%, `appendSegs` 19%, `buildCollapsedRenderings` 15%, `Close` 6% — all arena `append`s. Acceptable one-shot UX; only the churn is worth trimming.

**Evidence:** builder.go:43-49 — `Nodes`/`Lines`/`Segs`/`Aux` all start nil; `buildCollapsedRenderings` (builder.go:200-219) appends ~3 more segs per foldable node.

**Fix:** pre-size in `newBuilder` from `len(src)`. Measured divisors that don't under-allocate: `Lines ≈ src/16`, `Nodes ≈ src/16`, `Segs ≈ src/5`. Do **not** pre-size `Aux` (interning keeps it at 47 B–5 KB; summary dedup is 94–100% hit) — pre-sizing it is pointless. Pre-size `litCache` similarly. Pure capacity hints, no behavior change; removes most of the 800k allocs and roughly halves peak transient memory.

---

## Non-issues / already-fine (verified, not assumed)

- **Projection** `lineAtRow`/`rowOfLine` — pure O(log n) Fenwick rank/select, **104.6 ns / 0 allocs** at 440k rows (foldindex.go:156-164). Do not touch.
- **Incremental fold/unfold/toggle** — O(visible descendants) + O(log n) Fenwick adds, **137.8 ns / 0 allocs**; `toggle` does *not* call `rebuild` (foldindex.go:167-198). This is the template for fixing finding 5.
- **Reflow virtualization bound holds** — `ReflowOnlyBig` = 58 µs regardless of 440k total rows; `contentLayout.MinSize` returns `contentSize()` arithmetic and never walks children (renderer.go:186, 197-205). `recomputeMetrics` (and `MeasureText`) is confined to Refresh/CreateRenderer, never per scroll frame (renderer.go:209-229).
- **Highlight rect building** is clamped to `[first,last]` (highlight.go:32, 80-86) — a document-spanning selection or 10k-match set still draws only ~viewport-many rects, pooled via `poolRect` (highlight.go:115-119).
- **SearchNext on an expanded doc** — 210 µs, no rebuild, via the `vis[line]==1` early-out (foldindex.go:252).
- **Zero-copy model** — segments are byte ranges into `Src`/`Aux` (model.go:156-160), `computeExtent` is allocation-free (builder.go:236-261). Stable retained memory, not churn. *Contract to document:* the `src` slice is retained and must not be mutated after `SetData`.
- **Copy / double-click** (`selectedText` selection.go:335/341, `wordBounds` selection_words.go:30) allocate `[]rune` per line but are rare one-shot actions — leave as-is.
- **`addMatch` rune recount** (search.go:186) is O(K·L) per line but globally capped at `MaxMatches=10000`; only pathological for a 1-char needle in a huge single minified line. Optional: carry a running rune cursor across the per-line loop.
- **XML/HTML parse** (`xml.CopyToken`, `z.Token()`) inherit allocations from `encoding/xml` / `x/net/html`, but fixtures are 16 KB / 2.6 KB — trivial. Only revisit if those inputs get large.

## Recommended action order

1. **Finding 1 (debounce) — do first.** Smallest change, biggest user-visible win: collapses N keystroke-scans into 1. Pure addition, no risk to existing behavior.
2. **Finding 2 (byte-scan search).** Cuts the per-scan cost from 6.6 ms → sub-millisecond and removes ~190k allocs. Together 1+2 eliminate search jank entirely. Preserve `lineSegs` (expanded-text) semantics and `addMatch` rune columns.
3. **Finding 3 + 4 (kill double/triple row rebuild).** One small renderer change each; halves (then thirds) per-frame and per-interaction row work with identical pixels. Verify with a "build count per reflow" assertion.
4. **Finding 5 (incremental reveal).** Ship the Fenwick-reuse half first (trivial, kills the 1.84 MB/step alloc); gate the incremental-unfold half behind a nested-fold test because the outermost-first ordering is subtle.
5. **Findings 6–8 (per-frame alloc polish).** Cheap, low-risk, do together while in `build()`/reflow.
6. **Finding 9 (arena pre-size).** Cold-path cleanup; lowest priority.

**Leave alone:** the Fenwick projection, incremental fold path, content-size arithmetic, highlight-window clamping, the rect pool, and the zero-copy model. They are measured fast and allocation-free; "optimizing" them only adds risk.