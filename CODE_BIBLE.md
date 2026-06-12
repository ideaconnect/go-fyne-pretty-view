# CODE_BIBLE.md

The non-negotiable engineering commandments for `go-fyne-pretty-view`. Everything
here is a rule, not a suggestion. If a change breaks one of these, the change is
wrong — even if it compiles, looks reasonable, and passes a casual review.

This is the short, durable list. The *why* and the detail live in
[AGENTS.md](AGENTS.md) (invariants + conventions), [docs/DESIGN.md](docs/DESIGN.md)
(architecture + risk analysis), and [WORKFLOWS.md](WORKFLOWS.md) (how-to).

## 1. The memory bound is sacred

Only viewport-many rows are ever live canvas objects; per-row text is
horizontally culled; the model is struct-of-arrays, pointer-free, and zero-copies
into the source bytes. Selection, search, and copy operate on the model, never on
widgets. A change that regresses `renderer_test.go` or `memory_test.go` is the
change at fault. Never introduce one-canvas-object-per-line / per-token / per-match.

## 2. Tests ship with the change, and they must have teeth

Every behavioral change ships a test in the **same** commit. A test has *teeth*
only if it fails when the behavior it describes regresses — assert the actual
value, the exact column, the real bytes, not just "no error" or "not nil". Prefer
the headless, pure-model path (parse, fold, selection math, search offsets); use
`test.NewApp()` + `test.NewWindow` only for genuine rendering. Tolerant parsers
must be fuzzed/swept for "never panics".

## 3. Coverage stays **above 95 %**

CI computes cross-package coverage with
`go test ./... -race -covermode=atomic -coverpkg=./...` and **fails the build at or
below 95 %**. Raising coverage is never an excuse for vacuous tests (rule 2) — a
coverage-only poke that asserts nothing is worse than no test, because it hides the
gap. If a line is genuinely untestable (a real `app.New()`/`ShowAndRun` entry
point), isolate it behind a thin `main` and test the extracted body instead of
contorting the test around a driver.

## 4. `make check` is the gate

`gofmt`, `go vet`, and `go test -race ./...` must all be green before a change is
done. `go vet` also forbids `fyne.io/fyne/v2/internal/...` imports — use the public
surface (`sync.Pool`, `container.Scroll`, `fyne.MeasureText`, …). No exceptions, no
"I'll fix the race later".

## 5. One coordinate convention

Content space only; the caller adds the scroll offset once before hit-testing and
nothing subtracts it (see the top of `geometry.go`). Touching the pixel↔model math
means re-running `TestHitTestGoldenRoundTrip`. Off-by-one selection bugs live here.

## 6. The public API is a contract

The exported surface of `prettyview` / `fonttheme` is frozen by
`TestExportedSurfaceGolden` (`testdata/api_surface.txt`). Additions ship as a minor;
a breaking change ships under a new major module path, never as a minor bump.
Document every exported identifier. Keep `CHANGELOG.md` current under Keep-a-Changelog
headings.

**Deprecation:** never remove an exported symbol abruptly inside a major. Mark it with a
Go `// Deprecated:` doc comment naming the replacement, keep it for at least one more minor,
and remove it only at the next major. The golden still tracks it until then.

## 7. Never corrupt the user's bytes

The viewer and editor are lossless by construction: an editor's `Source()` round-trips
through `SetData`. A reformat may only rewrite bytes when it can do so without losing
content (e.g. JSONC reformat recolors in place rather than dropping comments). When in
doubt, preserve the bytes and recolor — do not rewrite.

## 8. Match the surrounding code

Doc comments on exported names; table-driven tests; fixtures in `testdata/`; the
comment density, naming, and idiom of the file you are editing. New code should be
indistinguishable from the code around it.

---

If you are an AI agent, read [AGENTS.md](AGENTS.md) next. If you are a human, read
[HUMANS.md](HUMANS.md). Both point back here for the commandments.
