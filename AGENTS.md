# AGENTS.md

Guidance for AI coding agents working in this repository. (Claude Code reads
[CLAUDE.md](CLAUDE.md), which points here.) This file is the canonical,
tool-agnostic brief.

## What this project is

A reusable Fyne v2 widget (`package prettyview`) that views JSON / JSONC / XML /
HTML / raw text like Bruno's response panel — syntax highlighting, folding,
char-level selection, copy, and search — built to a **hard memory budget**.

Read these before making non-trivial changes:
- [CODE_BIBLE.md](CODE_BIBLE.md) — the binding engineering commandments (memory
  bound, teeth-bearing tests, **coverage > 95 %**, `make check`, API stability).
  Read it first; the rest of this file is the detail behind it.
- [STRUCTURE.md](STRUCTURE.md) — where everything lives.
- [WORKFLOWS.md](WORKFLOWS.md) — how to build/test/bench/extend.
- [docs/DESIGN.md](docs/DESIGN.md) — the architecture and the adversarial risk
  analysis the code is built from.

## Non-negotiable invariants

These are the reason the widget exists. A change that breaks one is wrong even
if it compiles and passes a casual look. The tests in `renderer_test.go` and
`memory_test.go` enforce them — keep those green.

1. **Only visible rows are widgets.** At most ~viewport-many `rowWidget`s exist
   at once (recycled via a pool). Never create a canvas object per line, per
   token, or per match for the whole document. RichText/Label-per-line is banned.
2. **Per-row horizontal culling is mandatory.** A row emits only the text in the
   visible column window (`row.go`). Fyne rasterizes each `canvas.Text` to its
   full width, so an un-culled 2 MB line would allocate a ~1 GB bitmap.
3. **The model is struct-of-arrays and pointer-free.** `Node` (32 B), `Line`
   (24 B), `Segment` (12 B) in flat arenas; data tokens are zero-copy byte ranges
   into `Document.Src`. Don't add per-node pointers or per-line `[]rune`/strings;
   materialize runes only for the rows on screen. Target ≈ 5× source size.
4. **Selection / search / copy are model-based.** They operate on `(line, col)`
   positions and the byte arenas, never by reading a `CanvasObject`. Highlight
   rectangles are intersected with the visible row range before drawing.

## Architecture facts to respect

- **One coordinate convention** (top of `geometry.go`): content-space only;
  callers add the scroll offset once before hit-testing; nothing here subtracts
  it. If you touch this, run `TestHitTestGoldenRoundTrip`.
- **Projection is line-granular** with a Fenwick tree (`foldindex.go`). Folding
  is O(visible descendants); lookups are O(log n). The correctness of incremental
  fold/unfold depends on the rule that **you only ever fold/unfold a currently
  visible node** (hidden triangles can't be clicked; bulk ops rebuild).
- **Positions are stable line indices, not node ids.** Line indices never change
  after parse (only visibility does); hidden lines snap to the nearest visible
  ancestor at use sites.
- Parsers never build a tree; they drive `Builder`. The collapsed rendering of a
  fold head is built in `Builder.finish` after trailing commas are placed.

## Conventions

- Run `make check` (gofmt + `go vet` + `go test -race`) before finishing. It must
  be green.
- **Coverage must stay above 95 %.** CI computes cross-package coverage
  (`go test ./... -race -covermode=atomic -coverpkg=./...`) and fails the build at
  or below 95 %. Ship a teeth-bearing regression test with every change; never pad
  the number with assertion-free "coverage" tests (see [CODE_BIBLE.md](CODE_BIBLE.md)
  rules 2–3). Genuinely untestable entry points (a real `app.New()`/`ShowAndRun`)
  are isolated behind a thin `main` so the extracted body is tested instead.
- **No `fyne.io/fyne/v2/internal/...` imports** — `go vet` rejects them. Use
  `sync.Pool`, `container.Scroll`, `fyne.MeasureText`, etc.
- Match the surrounding style: doc comments on exported identifiers, table-driven
  tests, fixtures in `testdata/`.
- UI mutations happen on the Fyne goroutine. Parsing is synchronous today; if you
  move it off-thread, swap the model back via `fyne.Do` and keep workers reading
  only immutable arenas.
- Prefer adding a focused test in the same change. Pure-model logic (parse, fold,
  selection math, search offsets) is testable without a window; rendering needs
  `test.NewApp()` + `test.NewWindow`.

## Quick commands

```sh
make build         # library + demo
make test          # full suite     (RUN=Name to filter)
make bench         # benchmarks     (BENCH=Name to filter)
make check         # gofmt + vet + -race  (the gate)
make shots         # headless screenshots to /tmp and docs/
```

## Gotchas

- `theme.Color(name)` is single-arg (current variant); use the helper
  `themeColor(name, variant)` for a specific variant.
- The Fyne test theme reports `ThemeVariant() == 2`, not Dark/Light — variant-keyed
  tests should read the live variant.
- `container.Scroll.ScrollToOffset` does **not** fire `OnScrolled`; the code calls
  `reflow()` explicitly after programmatic scrolls.
- The "Fyne error: Error parsing user locale C" line in test/run output is a
  harmless WSL locale warning, not a failure.
