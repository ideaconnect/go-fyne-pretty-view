# CLAUDE.md

Guidance for Claude Code (and other AI agents) working in this repo.

**This repo's agent brief lives in [AGENTS.md](AGENTS.md) — read it first.** It
covers the non-negotiable memory invariants, the architecture facts to respect,
conventions, and the gotchas. The notes below are the short version plus a few
Claude-specific tips; they don't replace AGENTS.md.

**The engineering commandments are in [CODE_BIBLE.md](CODE_BIBLE.md)** — the binding
rules every change must obey: the memory bound, teeth-bearing tests, **coverage above
95 %**, a green `make check`, and public-API stability.

## The one thing to remember

This widget exists to view multi-megabyte JSON/XML/HTML **without** the memory
blow-up of one-canvas-object-per-line. So: **only viewport-many rows are ever
widgets; selection/search/copy operate on the model, not on widgets; per-row text
is horizontally culled.** If a change regresses `renderer_test.go` or
`memory_test.go`, it's the change that's wrong.

## Where to look

- [STRUCTURE.md](STRUCTURE.md) — file/package map and the one-paragraph mental model.
- [WORKFLOWS.md](WORKFLOWS.md) — build/run/test/bench, adding a parser or color.
- [docs/DESIGN.md](docs/DESIGN.md) — the full architecture + the adversarial risk
  table (R-1…R-19) the implementation answers. Cite it when changing core logic.

## Working here

- Before finishing a change, run **`make check`** (gofmt + `go vet` + `go test
  -race`). It must pass. `go vet` also enforces "no `internal/` Fyne imports".
- You can see the UI without a display: **`make shots`** renders fixtures to PNGs
  under `/tmp` and `docs/` via Fyne's software painter. Read them to verify
  layout/colors/highlight z-order after rendering changes.
- Pure-model code (parse, fold index, selection math, search offsets) is testable
  headlessly; rendering tests use `test.NewApp()` + `test.NewWindow`.
- Keep changes milestone-sized and ship a test in the same change. The build plan
  in docs/DESIGN.md (M0–M11) shows the intended granularity.

## Things that will trip you up

- `theme.Color(name)` takes one arg; use `themeColor(name, variant)` for a
  specific variant. The Fyne **test** theme's variant is `2` (not Dark/Light).
- `ScrollToOffset` doesn't fire `OnScrolled`; reflow is called explicitly.
- Coordinate math has exactly one convention (top of `geometry.go`). Changing it
  without re-running `TestHitTestGoldenRoundTrip` invites off-by-one selection bugs.
- The "Error parsing user locale C" log line is a harmless WSL warning.

## For humans

If you're a person reading this, see [HUMANS.md](HUMANS.md) and
[README.md](README.md).
