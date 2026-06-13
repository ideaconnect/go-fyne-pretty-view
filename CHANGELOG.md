# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The module is on the **`/v2`**
major; the exported surface of `prettyview`/`fonttheme` is **frozen** (pinned by
`TestExportedSurfaceGolden`): additions ship as a minor, and any breaking change ships only
under a new major module path (`.../v3`), called out under **Changed**/**Removed**. Releases
carry a `-alpha` suffix that marks pre-production maturity — not API churn (see
[SECURITY.md](SECURITY.md) for the support model and [WORKFLOWS.md](WORKFLOWS.md) for the
checklist that gates dropping it).

## [Unreleased]

_Nothing pending._

## [v2.1.7-alpha] — 2026-06-13 — markup raw-text fidelity + parse hardening

From the pre-launch production-readiness review. No exported API signatures changed; the **/v2**
surface stays frozen.

### Fixed
- **HTML `<script>`/`<style>` bodies and XML `<![CDATA[…]]>` are now preserved byte-for-byte on
  `Reformat`** instead of being whitespace-collapsed and entity-escaped. Escaping a script body's
  `<` to `&lt;` produced invalid JavaScript (and a `//` line comment collapsed onto the next
  statement); these raw-text spans are re-emitted verbatim from the source. Ordinary element text
  is still escaped/canonicalized as before (#85).
- **A panic in the raw fallback or the model finish step can no longer escape `parse.Parse`.** The
  recover boundary previously wrapped only the structured parsers; it now covers the whole
  function, degrading to the raw fallback (or an empty document) so an unforeseen panic can't crash
  the embedding host (#86).

### Documentation
- README now documents that markup `Reformat` preserves raw-text content verbatim and
  whitespace-canonicalizes ordinary element text (#85).
- Fixed a misattached doc comment (`TypedKey`'s description sat on `KeyDown`) and a stale
  "JSONC is not rewritten" comment in the reformat path (#89).

### Performance
- **The live edit reproject now reuses a single segment-build scratch across all lines** instead
  of allocating one temporary `[]Seg` per physical line (#84, first half of the #80 follow-up).
  Combined with #80's arena pooling, a steady-state keystroke reproject of a large buffer now
  allocates **0 B / 0 allocations** on both minified and pretty-printed shapes (the pretty shape
  was ~2.16 MB / ~67 k allocations). The non-pooled `ParseEditableColored` benefits too (one
  scratch reused across lines instead of one per line). Output is byte-identical to before
  (locked by the equality fuzz). The remaining half of #84 — work strictly proportional to the
  edited region — is still open.

## [v2.1.5-alpha] — 2026-06-13 — production-hardening follow-ups

The remaining gaps from the code- & test-quality reviews (milestone "v2.2.0 — production
hardening"). No exported API signatures changed; the **/v2** surface stays frozen.

### Fixed
- **XML/HTML `Reformat` no longer decodes entities into the buffer.** Reserialization
  re-encodes the reserved characters the parser decoded (`&` → `&amp;`, `<` → `&lt;`, and a
  `"` inside an attribute value → `&quot;`), so a reformat-then-save stays valid markup and
  round-trips `&amp;` rather than emitting an invalid bare `&` (#81). Display/copy are
  unchanged — they still show the decoded text.
- **A JSONC document that begins with a comment now auto-detects as JSONC** instead of
  falling back to raw. A leading `// license` header (or `{ // note`) is an unambiguous
  JSONC signal, so the comment is preserved as a visible node; a `//` inside a string value
  still stays plain JSON (#82).

### Performance
- **The live edit reproject now reuses its Document arenas and the buffer snapshot across
  keystrokes** instead of re-allocating the whole projection every time (#80). A keystroke
  reproject of a large (over-budget, monochrome) buffer drops from ~18.6 MB / 18 allocations to
  ~32 B / 1 allocation, and is ~35 % faster on both minified and pretty-printed multi-MB
  buffers — a GC-pressure/RSS/jitter win. The pooled projection is byte-identical to a fresh
  parse (locked by a randomized equality fuzz), so caret/selection/search behavior is unchanged.
  Per-keystroke wall time is still proportional to the buffer (the snapshot copy and rune-count
  extent pass are inherently whole-buffer); work strictly proportional to the edited region is a
  separate, deferred follow-up (#84).

### Changed
- **Dropped the dead `tabWidth` parameter from the internal `ParseEditableColored`** — every
  grid-hostile byte (a tab included) renders as one placeholder rune, so there was no
  tab-width to honor. Internal package only; the `/v2` public surface is unchanged (#83).

## [v2.1.4-alpha] — 2026-06-13 — reflow allocation + mutation testing

### Performance
- Reuse the soft-wrap break scratch across reflows instead of re-allocating the whole wrapped
  line's break list every scroll tick (review #3).

### Changed
- Make the API-surface golden line-ending-agnostic and pin LF via `.gitattributes`, so the
  cross-OS CI (added in #69) passes on a Windows CRLF checkout.
- Add `make mutation` (gremlins) to measure test *efficacy* beyond coverage; the pure logic
  packages stand at 100 % (review #3, documented in CODE_BIBLE/WORKFLOWS).

## [v2.1.3-alpha] — 2026-06-12 — P2 production hardening

### Fixed
- Close four latent defensive gaps in the parsers/buffer (a trailing backslash pushing past
  EOF, and similar) (#76).

### Performance
- Preallocate the reformat source spans and binary-search the caret remap (millions of spans
  on a multi-MB reformat); memoize the gutter digit width; add regression benches (#77).

### Changed
- Lower the `go` directive to the real floor, unpin `govulncheck`, and document the deps (#79).

### Tests
- Lock the binary-search caret-remap against the linear oracle (#77); convert a self-erasing
  `t.Skip` to `t.Fatal` and lock the structured-classification guard (#78).

## [v2.1.2-alpha] — 2026-06-12 — cross-OS CI + robustness + test breadth

### Fixed
- Clear the undo history when `SetData` re-seeds the buffer, so an undo can't resurrect a
  previous document's bytes (#66).
- Bound the bundled Open dialog's read by `WithMaxInputBytes` (#73).

### Performance
- Stop copying the whole buffer on every delete / arrow keystroke (#68).

### Changed
- CI now runs the test suite on Windows and macOS, not just Linux (#69).

### Tests
- XML/HTML editor round-trip + undo-restores-bytes parity (#70); exercise the real async
  debounce path under `-race` plus the shutdown drop (#71); fuzz the editor undo round-trip,
  caret cells and wrap partition, with an unbounded-undo oracle fix (#72); tighten the
  model-size guard to the documented band (#74); document production limits + accessibility (#75).

## [v2.1.1-alpha] — 2026-06-11 — live-colorizer budget

### Performance
- Cap the live colorizer with a 2 MiB budget so a large editable buffer is rendered monochrome
  rather than re-lexed on every keystroke; color resumes automatically once it drops back under
  budget (#65).

## [v2.1.0-alpha] — 2026-06-11 — live highlighting while typing

### Changed
- **Edit mode: real-time syntax highlighting while typing, and prettify on demand.** The
  editor no longer swaps to a separate structured projection or reflows on a timer. While
  you type, the buffer is shown through a tolerant, layout-preserving syntax colorizer —
  live colors on every keystroke, with the caret kept an exact buffer position — so typing
  never breaks the existing formatting.
  - **`AutoFormat` now defaults to `AutoFormatOff`** (was `AutoFormatOnPause`): nothing
    reflows the text out from under you. Opt into `AutoFormatOnPause` / `AutoFormatOnBlur`
    for the old auto behavior.
  - **`Reformat()` now pretty-prints by rewriting the edit buffer in place** (instead of a
    display-only swap) and remaps the caret to the same token, so the prettified layout
    persists as you keep typing. It rewrites only a structured, *valid* parse; raw or
    invalid input is left exactly as typed. `Source()` therefore returns the indented bytes
    after a `Reformat`, and a reformat is a single undo unit.
  - The settle (debounced) now refreshes live parse validity and fires `OnChanged`/the
    validation callback without reflowing; `SetData` no longer arms a settle.
  - No exported API signatures changed; the **/v2** surface stays frozen.
- The editor demo (`make run-editor`) reflects the new model: live colors as you type, a
  Reformat button that prettifies in place, no surprise auto-formatting.

### Fixed
- Resolve the dead-code / duplication / gap audit (#55–#64) and raise the quality bar; drop
  code orphaned by the live-highlight redesign; guard JSONC reformat; harden tests.

### Added
- An editable-input editor demo, and `make run-viewer` / `make run-editor`.

## [v2.0.0-alpha] — 2026-06-11 — editable input + live formatting

v2 makes the same widget an **opt-in light editor**: a host can let a user type or paste
data and watch it pretty-format live. Read-only hosts migrate by changing only the import
path — see [MIGRATION.md](MIGRATION.md). The exported surface is re-frozen as the **/v2**
surface (`TestExportedSurfaceGolden`); a future breaking change ships under `.../v3`.

### Changed (breaking)
- **Module path** bumped to `github.com/ideaconnect/go-fyne-pretty-view/v2` (Go semantic
  import versioning). This is the only breaking change: **no v1 symbol was renamed or had
  its semantics changed.** Read-only ergonomics (`New`, `NewWithData`, `SetData`/`SetText`,
  `Source`, `Reparse`, `Format`, all `With*`) are preserved with v1 semantics.

### Added
- **Edit mode (opt-in, construction-time):** `WithEditable()` + `Editable() bool`. The
  input-vs-output purpose is fixed at construction — there is no `SetEditable` and no
  chrome toggle (a host-only runtime flip is deferred, see issue #54). A rendered caret,
  character input, Enter/arrows/Home/End, and selection editing.
- **Live formatting:** `InputConfig{ DebounceFor, AutoFormat, MaxEditBytes }` +
  `AutoFormatMode` (`AutoFormatOff`/`AutoFormatOnPause`/`AutoFormatOnBlur`),
  `WithInputConfig`/`SetInputConfig`, and `Reformat()`. On a debounced pause (or an
  explicit `Reformat`) the buffer is re-parsed into the structured, syntax-colored view;
  the caret is anchored across the reformat by its byte offset.
- **`Text() string`** — the document as displayed (pretty), distinct from the raw
  `Source()` bytes; `Source()`/`Reparse` read the live edit buffer in edit mode.
- **Caret API:** `Caret() (line, col int)`, `SetCaret(line, col int) bool`.
- **Undo/redo:** `Undo()`, `Redo()`, `WithUndoLimit(n)` (word-coalesced, bounded).
- **Clipboard:** `Cut()`, `Paste()` (control-byte-safe, one undo unit).
- **Validation feedback:** `ParseStatus{ OK, ErrorLine }`, `ParseStatus()`,
  `SetOnValidationChanged`, and an error-tinted gutter marker.
- **`SetOnChanged(func(string))`** — fired (debounced) after the edited text settles.

## [v1.1.0-alpha] — 2026-06-11

### Added
- The built-in toolbar and search-bar icon controls now carry **hover tooltips** (via
  [fyne-tooltip](https://github.com/dweymouth/fyne-tooltip)) — e.g. *Find next*,
  *Previous match*, *Case-sensitive*, *Regular expression*, *Open file…*, *Expand all*,
  *Collapse all*, *Toggle soft-wrap*. Tooltips render when the host wraps its window in
  `fynetooltip.AddWindowToolTipLayer(content, canvas)` (the demo does this); they are
  absent, not an error, without it. Additive and backward-compatible (the control
  constructors still return `fyne.CanvasObject`).

## [v1.0.0-alpha] — 2026-06-11 — frozen public surface

The exported API of `prettyview` and `fonttheme` is declared stable as of v1.0.0 and
guarded by `TestExportedSurfaceGolden`; see the README **Stability** section. After
1.0 a breaking change ships under a new major module path (`.../v2`).

### Changed (breaking)
- `Match` now uses `int` for all columns (`Line` was `int32`), and
  `PrettyView.Matches() []Match` returns a snapshot of the current hits.
- `WithSearchConfig` merges field-by-field over the defaults: a zero field keeps its
  default, removing the `DebounceFor: 0` zero-value trap. A negative `DebounceFor`
  disables keystroke coalescing.
- `DefaultToolbarConfig(win fyne.Window)` takes the window, so it truly enables every
  control (`nil` omits Open and Ctrl/Cmd+F).

### Added
- A frozen exported-surface golden test (`testdata/api_surface.txt`) and a
  **Stability** section in the README and both package docs.

### Fixed
- Search bar: pressing Enter (find-next) repeatedly now advances through every match
  (1→2→3→…, wrapping) instead of always snapping back to match #2.
- Search bar: the leading magnifier renders at button size and padding (aligned with
  the control row) instead of as a small bare icon.

### Documentation
- Pinned the `Search` (immediate) vs `SearchDebounced` (coalescing) contract and the
  `SearchMode` constants; `Source()` notes that its slice aliases the retained buffer.

## [v0.9.0-alpha] — 2026-06-11 — feature-complete + freeze candidate

### Added
- `ScrollToLine(line) bool`, `ScrollOffset()`, and `SetScrollOffset(pos)` — a
  format-independent programmatic scroll / go-to-line API and scroll-position
  save/restore.
- `CollapseToDepth(d)` / `ExpandToDepth(d)` — runtime fold-to-depth on `PrettyView`
  and `Document`.
- `WithLineNumbers()` — an opt-in line-number gutter (numbers drawn from the model,
  no per-line widgets).
- Right-click **Copy key path** (JSON/JSONC) — the clicked node's JSONPath accessor,
  e.g. `$.users[1].name`.
- Keyboard selection and fold control: `Shift`+arrows / `Shift`+`Home`/`End` extend a
  selection from the caret, `Enter` toggles the caret line's fold, `Left`/`Right`
  scroll horizontally.
- `example_test.go` (rendered on pkg.go.dev), `CHANGELOG.md`, and `SECURITY.md`;
  release archives now ship a `SHA256SUMS` manifest.

### Changed
- `CopySubtree(byteOffset)` and `ExpandTo(byteOffset)` now resolve nodes for **XML and
  HTML** too (real per-node source byte offsets are populated), not JSON/JSONC only.

## [v0.5.0-alpha] — parser hardening + chrome parity

### Added
- `FuzzParse` native fuzz target across all formats (run in CI), plus a govulncheck
  gate in CI and the release workflow.
- Search bar: case-sensitive (`Aa`) and regex (`.*`) toggles, Shift+Enter (find
  previous), Esc (clear); the counter shows `bad regex` for an invalid pattern.
- Public `SearchDebounced(q)` (coalescing scan) and `SearchError() error`.
- Right-click **Copy subtree** for the node under the cursor — works for every format.
- `WithMaxInputBytes(n)` caps `SetData`/`SetText` input.
- JSONC `//` and `/* */` comments now render as nodes (visible, searchable, copyable).

### Fixed
- Control-byte leaks into display lines (XML/HTML text and tag/attribute names) found
  by fuzzing; a `recover()` boundary degrades a parser panic to the raw fallback;
  HTML nesting depth is bounded; the file-open dialog surfaces picker errors instead
  of swallowing them.

### Security
- Bumped `golang.org/x/image` to v0.41.0, clearing the last reachable CVEs;
  `govulncheck ./...` reports no vulnerabilities.

## [v0.4.0-alpha] — honor the invariants under load

### Changed
- Search match-highlight rectangles are culled to the horizontal visible window —
  per-reflow rect count is O(visible columns), not O(matches).
- A byte==column-grid fast path makes a reflow deep into a huge/soft-wrapped line
  O(visible window), not O(scroll position) (a 2 MB line: ~16 ms → ~1.4 ms).
- Regex search gained a literal-prefix prefilter; performance/model-size docs were
  re-scoped to measured numbers (plain scan ~5 ms; model ~4.85×–7.1× source).

### Added
- Renderer-attached perf/alloc regression guards and a benchmark smoke step in CI.

## [v0.3.0-alpha] — stop silently dropping or misrepresenting input

### Fixed
- JSON auto-detect requires a plausible token after a leading bracket (log/markdown
  lines no longer mis-parse as JSON); trailing content after a JSON root falls back to
  raw (NDJSON stays visible); raw control bytes in segments are C-escaped; a search
  match on a collapsed fold-head is now highlighted.

### Changed
- `ExpandTo` and `CopySubtree` return `bool` and are documented JSON/JSONC-only;
  `CopySubtree` copies the pretty-printed rendering.

### Security
- Toolchain bumped to go 1.26.4, clearing two stdlib CVEs.

## [v0.2.0-alpha] — fonts, icons, parser/search hardening

### Added
- Opt-in `fonttheme` subpackage bundling JetBrains Mono + Inter (SIL OFL 1.1).
- Toolbar icons switched to Font Awesome Free (CC BY 4.0).

### Fixed
- Parser robustness across JSON/XML/HTML detection and recovery; a regex-search anchor
  regression (`^`, `\b`, `\B`) where re-slicing re-anchored the pattern.

## [v0.1.0-alpha] — initial alpha

- First public alpha of the virtualized JSON/JSONC/XML/HTML/raw viewer widget:
  syntax highlighting, folding with collapse summaries, char-level selection and copy,
  search with reveal-into-folds, optional soft-wrap, and an opt-in toolbar — built to
  a hard memory budget (only viewport-many rows are ever live widgets).

[Unreleased]: https://github.com/ideaconnect/go-fyne-pretty-view/compare/v1.1.0-alpha...HEAD
[v1.1.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/compare/v1.0.0-alpha...v1.1.0-alpha
[v1.0.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/compare/v0.9.0-alpha...v1.0.0-alpha
[v0.9.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/compare/v0.5.0-alpha...v0.9.0-alpha
[v0.5.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.5.0-alpha
[v0.4.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.4.0-alpha
[v0.3.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.3.0-alpha
[v0.2.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.2.0-alpha
[v0.1.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.1.0-alpha
