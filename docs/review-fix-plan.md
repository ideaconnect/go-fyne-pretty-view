# Codebase review — fix plan

Tracking doc for the deep review (code quality · improvements · evident issues ·
performance). Findings were adversarially verified; severities below are the
post-verification values. No blockers / high-severity defects were found — the
list is a tail of low/nit items. Check items off as they land. This file can be
deleted once the plan is complete.

## Batch P1 — correctness (high ROI)

- [x] **P1.1 — JSON whitespace mismatch → silent data loss.**
  `internal/parse/parse_json.go` `skipSpace`. The detector treats `\f`/`\v`/NBSP
  /Unicode spaces as whitespace (`unicode.IsSpace`) but the scanner only skips
  ASCII ` \t\n\r`, so a `\f` between members silently drops the rest of the
  container. Fix: make `skipSpace` recognise the same whitespace set as the
  detector (ASCII fast path + `unicode.IsSpace` for multi-byte runes) so near-JSON
  parses fully instead of truncating. Tests: leading `\f{…}`, NBSP-led, mid-member
  `{"a":1,\f"b":2}` all parse as JSON with every member present.

- [x] **P1.2 — Partial raw-line copy expands tabs to spaces** (breaks DESIGN §4.3 /
  R-9 "copy round-trips the original bytes"). `selection.go` `selectedText`. The
  whole-line copy path restores `\t`; a mid-line (partial-endpoint) selection slices
  `DisplayString` (expanded spaces). Fix: make the partial branch tab-aware — walk
  `DisplaySegs` tracking the display-column cursor and emit `\t` for an Aux/RolePlain
  pad fully inside `[start,end)`. Test: a *partial* selection across a raw tab
  round-trips `\t`. Reconcile the R-9 "Resolved" wording in `docs/DESIGN.md`.

- [x] **P1.3 — `colorToHex` reads premultiplied RGBA, drops alpha.** `icons.go`. The
  exact footgun `withAlpha` (`theme.go`) was written to avoid; a non-opaque themed
  foreground bakes a darkened icon. Fix: convert via `color.NRGBAModel` before
  reading channels, mirroring `withAlpha`.

- [x] **P1.4 — Search bar `Enter` skips match #1** within the debounce window.
  `controls.go` `NewSearchBar` `OnSubmitted`. Fix: only `SearchNext()` when the
  query is unchanged — `advance := pv.search.query.Text == entry.Text` before
  `Search`, then conditional advance. Test: fresh-query Enter lands on match #1.

- [x] **P1.5 — `lineAndSubRow` returns a negative sub-row when `total()==0`.**
  `internal/model/foldindex.go`. Unreachable today but contradicts the
  "clamps match lineAtRow" contract. Fix: early guard
  `if fi.bit.total() == 0 { return n - 1, 0 }`. Test: the guarded state.

- [x] **P1.6 — Source > 4 GiB silently wraps uint32 offsets.** `internal/parse`
  `Parse`. Guard at the parse entry so an out-of-scope giant input degrades
  gracefully (truncate to `MaxUint32`, preventing offset corruption) instead of
  silently mis-slicing; document the ceiling next to the uint32 fields in
  `internal/model/model.go`. (Note: a raw fallback does *not* help — same casts.)

## Batch P2 — parser robustness & hot-path performance

- [x] **P2.1 — XML `Detect` unbounded `bytes.Contains`** (~20-40× the bounded HTML
  detector on multi-MB files). `internal/parse/parse_xml.go`. Fix: bound the scan to
  a `sniffLimit` head window like the HTML detector.
- [x] **P2.2 — HTML structural-tag sniff misclassifies legitimate XML as HTML**
  (and lowercases its tag names). `internal/parse/parse_html.go`. Fix: require a
  tag-name boundary after each prefix and give XML's positive signal precedence.
- [x] **P2.3 — `WrapBreaks` recomputed up to 3× per visible line per reflow** (only
  when wrap + selection + search are all active). `renderer.go` / `highlight.go`.
  Fix: hoist the two `[]int32(nil)` highlight caches to reusable renderer fields
  (kills the per-reflow allocation); optionally share one breaks list across passes.
- [x] **P2.4 — `indexMatches` reallocates its map every scan.** `search.go`. Fix:
  reuse the map with `clear()` instead of `make` per scan. (Do *not* do the
  binary-search rewrite — it moves cost onto the render hot path.)
- [x] **P2.5 — Regex search `FindAllIndex` allocates per matching line + the regex
  path is unbenchmarked.** `search.go`. Fix: from-offset `FindIndex` loop mirroring
  `scanPlain`; add `BenchmarkSearchRegex`.
- [x] **P2.6 — JSON literals/numbers lack a delimiter check** → trailing garbage
  (`[trueX]`, `[123abc]`) silently dropped. `internal/parse/parse_json.go`. Fix:
  require a value terminator after a literal/number; emit a `KindError` marker for
  the unexpected tail (mirroring the truncated-key recovery).
- [ ] **P2.7 — `CellOrigin`/`wrapRowSpan` allocate a fresh `WrapBreaks` per
  hit-test/search-nav under wrap.** `internal/geometry/geometry.go`. **Deferred:** the
  only fix is to thread a `dst []int32` through the `HitTest`/`CellOrigin` signatures
  (the coordinate-convention boundary guarded by `TestHitTestGoldenRoundTrip`) and
  every caller — disproportionate churn for a tiny per-gesture (not per-frame)
  allocation the verifier rated lowest priority. Revisit only if a profile shows it.

## Batch P3 — polish & docs

- [ ] **P3.1 — Toolbar icons don't recolor on a runtime light/dark switch** (baked
  `StaticResource`). `icons.go`. Fix or reconcile the DESIGN/comment wording.
- [ ] **P3.2 — Code-quality nits:** stale container head/close comment
  (`internal/model/model.go`); `collapseAll` mixed `NodeID(id)`/raw `id`
  (`internal/model/foldindex.go`); `HitTest` below-content comment over-promises
  under wrap (`internal/geometry/geometry.go`); `MouseDown` missing the `pv.r`
  nil-guard its siblings use (`selection.go`); `total==0` reflow doesn't truncate
  `rowObjs` (`renderer.go`).
- [ ] **P3.3 — DESIGN.md reconciliations:** R-9 scope (whole-line vs partial copy,
  resolved by P1.2), icon-recolor wording, format-detection spec vs implementation.
- [ ] **P3.4 — (WONTFIX candidate) search counter colour stale on theme switch**
  (`controls.go`). Verifier: the proposed fix doesn't work and it's consistent with
  the project's baked-chrome convention on an opt-in helper. Leave as-is unless the
  whole toolbar gets a theme-change listener.

## Rejected (false positives — no action)

- `CellOrigin` "missing clamp" — unreachable; columns are wrap/fold-invariant.
- Pooled `text()`/`guide()` "white flash" — hidden *and* excluded from `Objects()`.
- Case-insensitive non-ASCII search "misalignment" — `bytes.ToLower` preserves rune
  count (the `U+0130→2 runes` premise is false).

## Housekeeping

- [ ] Stray untracked `internal/parse/zz_xmldetect_bench_test.go` (left by a review
  agent) — remove, or fold into the P2.1 benchmark. Confirm with maintainer first.
