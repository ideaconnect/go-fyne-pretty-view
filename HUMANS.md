# HUMANS.md

For the people who build, use, and contribute to `go-fyne-pretty-view`. If you're
an AI agent, see [AGENTS.md](AGENTS.md) / [CLAUDE.md](CLAUDE.md) instead.

## What it is

A drop-in [Fyne](https://fyne.io) widget that renders JSON / JSONC / XML / HTML /
raw text the way Bruno's response viewer does — colors, fold/expand with
summaries, copy-a-section, free-text selection, and search — and stays small in
memory no matter how big the document is. The user-facing overview and API table
are in [README.md](README.md).

## Get it running (5 minutes)

You need Go 1.26+, a C compiler, and the Fyne GUI dependencies (CGO). On
Debian/Ubuntu:

```sh
sudo apt install gcc libgl1-mesa-dev xorg-dev
make build      # compiles the library and ./bin/prettyview-demo
make run        # opens the demo on testdata/openapi.json
```

In the demo: pick a file/format, expand/collapse, click a fold triangle, drag to
select text (Ctrl+C copies), and use the Find box (or Ctrl+F) to search.

## Where things are

Start with [STRUCTURE.md](STRUCTURE.md) — it maps every file and gives a
one-paragraph mental model. The short version:

- **Parsers** (`internal/parse/parse_*.go`) turn bytes into a compact model.
- **Model** (`internal/model/`: `model.go`, `foldindex.go`, `builder.go`,
  `wrap.go`) is a flat, pointer-free representation plus the fold/visibility and
  soft-wrap indices.
- **View** (root: `renderer.go`, `row.go`, `selection*.go`, `search.go`,
  `highlight.go`, with the pixel↔model math in `internal/geometry/`) draws only
  the rows on screen and handles input.

The *why* behind the design — and the trade-offs considered — is in
[docs/DESIGN.md](docs/DESIGN.md).

## Contributing

1. Read [CODE_BIBLE.md](CODE_BIBLE.md) — the binding commandments — and
   [AGENTS.md](AGENTS.md)'s "Non-negotiable invariants". The whole point of the
   widget is bounded memory; please don't regress it. The tests in
   `renderer_test.go` and `memory_test.go` will catch you, and that's by design.
2. Make your change with a teeth-bearing test in the same commit. See
   [WORKFLOWS.md](WORKFLOWS.md) for how to add a parser or a color, and how to
   eyeball the UI headlessly (`make shots`).
3. Run `make check` (gofmt + `go vet` + `go test -race`) — it must be green, and CI
   enforces **> 95 % coverage**, so keep the suite above the line.
4. Keep commits focused and the public API stable (semver). Open a PR with a
   short description of the behavior change and any new fixtures.

## Conventions humans care about

- Go formatting via `gofmt` (run `make fmt`); doc comments on exported names.
- Fixtures live in `testdata/`; tests are table-driven where it helps.
- Issues/PRs: describe what the user sees change, not just the diff.

## License

Licensed under the [BSD 3-Clause License](LICENSE) (© 2026 IDCT, Bartosz Pachołek).
