# Development workflows

How to build, run, test, benchmark, and extend `go-fyne-pretty-view`. Everything
routes through the [Makefile](Makefile); run `make help` for the full list.

## Prerequisites

- Go 1.26+
- A C toolchain and the Fyne GUI build dependencies (CGO). On Debian/Ubuntu:
  `sudo apt install gcc libgl1-mesa-dev xorg-dev`
- A display for `make run` (X11 or Wayland). Tests and screenshots need **no**
  display — Fyne's software renderer is used.

## Everyday loop

```sh
make build                 # compile the library + ./bin/prettyview-demo
make run                   # launch the demo on testdata/openapi.json
make run FILE=path/to/file # launch on any file
make test                  # full suite
make test RUN=TestSearch   # filter by test-name regex
make bench                 # all benchmarks
make bench BENCH=BenchmarkParse
make check                 # CI gate: gofmt check + vet + -race
```

`make cover` writes `coverage.out` and prints total coverage. `make clean`
removes `./bin` and coverage artifacts.

## Verifying visually without a display

The widget renders through Fyne's software painter in tests, so you can produce
real PNGs headlessly:

```sh
make shots          # writes /tmp/pv_*.png and docs/shot-json.png
```

`screenshot_test.go` (gated by `PV_SHOTS=1`) drives JSON/XML/HTML fixtures,
applies a selection and a search, and captures the canvas. Use this to eyeball
layout, colors, and highlight z-order after a rendering change.

## The invariant tests are the safety net

Two test groups guard the project's whole reason for existing — keep them green:

- `renderer_test.go` — live `rowWidget` count stays ~viewport-sized while
  scrolling the 7.5 MB fixture.
- `memory_test.go` — model size ≈ 5× source; heap stays well under 1 GB after
  scrolling the whole file; a single 2 MB line emits no over-wide texture;
  select-all produces ≤ viewport-many rectangles.

If a change makes these regress, the change is wrong, not the test. See the
invariants in [AGENTS.md](AGENTS.md).

## Extending the widget

### Add or change a format parser
1. Implement the `Parser` interface (`Format`, `Detect`, `Parse`) in a new
   `parse_<fmt>.go` — drive the `Builder` (`Open`/`Leaf`/`Close`/`AppendComma`),
   keeping data tokens as zero-copy `srcSeg` ranges where source offsets exist.
2. Register it in `parsers()` and `parserFor()` in `parse.go`.
3. Give `Detect` a confidence score that doesn't collide with siblings (see how
   XML vs HTML and JSON vs JSONC are disambiguated).
4. Add a fixture under `testdata/` and a dump/assert test in `parse_test.go`.

### Add a syntax color role
1. Add the `ColorRole` constant in `model.go` (before `numColorRoles`).
2. Map it in `buildPalette` (`theme.go`) for dark and light, and in
   `SyntaxColors` if it should be user-overridable.
3. Emit it from the relevant parser via `litSeg`/`srcSeg`.

### Touch the rendering / hit-testing math
All pixel↔model mapping lives in `geometry.go` and obeys **one** coordinate
convention (documented at the top of that file). If you change it, run the
golden round-trip test (`TestHitTestGoldenRoundTrip`) — it catches origin and
offset mistakes that otherwise show up as off-by-one selection drift.

## Milestone discipline

The project was built in testable milestones (M0–M11; see the build plan in
[docs/DESIGN.md](docs/DESIGN.md)). Keep that style: each change should leave
`make check` green and, where it affects behavior, ship a test in the same
commit.

## Releasing

This is a library; consumers pin a tag.

```sh
make check                      # must be green
git tag vX.Y.Z && git push --tags
```

Follow semver; the public API is everything exported from package `prettyview`
(see [README.md](README.md) for the surface).
