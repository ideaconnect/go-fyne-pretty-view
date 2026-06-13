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
make build                 # compile the library + both demo binaries into ./bin
make run-viewer            # read-only viewer demo on testdata/openapi.json
make run-viewer FILE=path  # viewer on any file  (make run is an alias of run-viewer)
make run-editor            # editable-input demo (type/paste -> live format)
make run-editor EDITOR_FILE=path  # editor seeded from a file
make test                  # full suite
make test RUN=TestSearch   # filter by test-name regex
make bench                 # all benchmarks
make bench BENCH=BenchmarkParse
make check                 # CI gate: gofmt check + vet + -race
```

`make cover` writes `coverage.out` and prints total coverage. `make clean`
removes `./bin` and coverage artifacts.

```sh
make mutation              # mutation-test the pure logic packages (gremlins); efficacy gate
make mutation MUT_PKGS=internal/geometry MUT_EFFICACY=100
```

`make mutation` runs [gremlins](https://github.com/go-gremlins/gremlins) over
`internal/{geometry,model,parse}` (installing it on first use), mutating the source and
re-running each package's tests — a **surviving** mutant is covered-but-not-actually-tested
code. It gates on test efficacy (`MUT_EFFICACY`, default 95 %; the pure packages are at
100 %). It's a manual/periodic check, not part of `make check` (it runs the suite once per
mutant — minutes, not seconds). The widget (root) package needs a Fyne app per run, so
mutation testing is scoped to the headless logic packages. Many byte-scanner mutations
(`i++`→`i--`) cause infinite loops that gremlins records as "timed out" = killed; that's
expected, not a failure.

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

The code is three packages (`internal/model` ← `internal/parse` ← root
`prettyview`); see [STRUCTURE.md](STRUCTURE.md).

### Add or change a format parser
1. In `internal/parse/`, implement the `Parser` interface (`Format`, `Detect`,
   `Parse`) in a new `parse_<fmt>.go` — drive `model.Builder`
   (`Open`/`Leaf`/`Close`/`AppendComma`), keeping data tokens as zero-copy
   `model.SrcSeg` ranges where source offsets exist.
2. Register it in `parsers()` and `parserFor()` in `internal/parse/parse.go`.
3. Give `Detect` a confidence score that doesn't collide with siblings (see how
   XML vs HTML and JSON vs JSONC are disambiguated).
4. Add a fixture under `testdata/` and a dump/assert test in
   `internal/parse/parse_test.go` (fixtures load via `../../testdata/`).

### Add a syntax color role
1. Add the `ColorRole` constant in `internal/model/model.go` (before
   `NumColorRoles`).
2. Map it in the `palette()` method (root `theme.go`) for dark and light, and in
   `SyntaxColors` if it should be user-overridable.
3. Emit it from the relevant parser via `model.LitSeg`/`model.SrcSeg`.

### Touch the rendering / hit-testing math
All pixel↔model mapping lives in `internal/geometry` and obeys **one** coordinate
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

Pushing a `vX.Y.Z` tag also triggers the **release** workflow
([.github/workflows/release.yml](.github/workflows/release.yml)): it builds the
demo for Linux/Windows/macOS, zips each binary together with the `testdata/`
fixtures, and attaches the zips to a GitHub Release with auto-generated notes. The
zip filenames embed the tag (e.g. `prettyview-demo-linux-amd64-v2.0.0-alpha.zip`),
so v1.x and v2.x release assets never collide. A tag with a pre-release suffix (e.g.
`v2.0.0-alpha`) is marked as a pre-release; you can also re-run it manually from the
Actions tab (workflow_dispatch) against an existing tag.

### v1 / v2 branch & tag policy

The module is on the **`/v2` major** (`module …/go-fyne-pretty-view/v2`). Features land
on the v2 line (`main`, via the `feature-v2.0.0` branch pre-merge) and tag `v2.x.y`. v1
is frozen: critical/security fixes land on the **`v1-maintenance`** branch and tag
`v1.x.y`. Both branches run the **same CI** (gofmt + vet + `-race` + cross-package
coverage + govulncheck) — they are in `ci.yml`'s `branches:` list, and PRs from any
branch are covered by the `pull_request` trigger. `TestModulePathIsV2` guards the `/v2`
path in CI.

Follow semver; the public API is everything exported from package `prettyview`
(see [README.md](README.md) for the surface), frozen as the `/v2` surface by
`TestExportedSurfaceGolden`.

### Dropping -alpha

Every release so far is tagged `vX.Y.Z-alpha`. The suffix signals **pre-production
maturity**, not API churn — the `/v2` surface is already frozen. The API being frozen and
the release being `-alpha` are independent: the suffix is dropped only when the project is
ready to make a production-readiness promise, gated by this checklist. Until then, docs must
not claim a non-alpha / "1.0" status (issue #67 fixed the contradictory copy).

To cut the first non-alpha release (`v2.Y.0`, no suffix), all of the following must hold and
be verifiable:

- [ ] **Green CI on all three OSes** (Linux test + Windows/macOS build & test — the matrix
      added in #69) on the exact commit being tagged.
- [ ] **`govulncheck` clean** (the CI gate passes; no unaddressed advisories).
- [ ] **Surface golden unchanged** — `TestExportedSurfaceGolden` passes and
      `testdata/api_surface.txt` has no pending diff (the frozen `/v2` contract held).
- [ ] **`make check` green** and **cross-package coverage > 95 %** (the CI gate).
- [ ] **`make mutation` ≥ the efficacy gate** on the pure logic packages (no new surviving
      mutants since the last run).
- [ ] **CHANGELOG cut** — the `[Unreleased]` section is moved under the new version heading,
      dated, with no stale "pre-1.0 / v0.x / v1.0.0-frozen" copy anywhere
      (`grep -rn "pre-1.0\|v0\.x" *.md *.go` returns only intentional history).
- [ ] **This milestone's P0/P1 issues closed** (the "production hardening" gaps resolved).
- [ ] **Maturity soak** — a deliberate decision that the editor + viewer have enough
      real-world mileage to back a stability promise (this is the judgment call the suffix
      exists to defer; it is not something CI can assert).

When dropping the suffix, bump the **minor** (e.g. `v2.2.0`) to mark the production line,
reconcile README / SECURITY.md / package docs / CHANGELOG to the non-alpha story in the same
commit, and tag without `-alpha`.
