# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Pre-1.0 the public
API may change between minor versions; breaking changes are called out under
**Changed**/**Removed**.

## [Unreleased]

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

### Fixed
- Search bar: pressing Enter (find-next) repeatedly now advances through every match
  (1→2→3→…, wrapping) instead of always snapping back to match #2.

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

[Unreleased]: https://github.com/ideaconnect/go-fyne-pretty-view/compare/v0.5.0-alpha...HEAD
[v0.5.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.5.0-alpha
[v0.4.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.4.0-alpha
[v0.3.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.3.0-alpha
[v0.2.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.2.0-alpha
[v0.1.0-alpha]: https://github.com/ideaconnect/go-fyne-pretty-view/releases/tag/v0.1.0-alpha
