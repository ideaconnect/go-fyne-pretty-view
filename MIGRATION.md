# Migrating from v1 to v2

v2 turns the read-only viewer into an **opt-in light editor** — a host can let a user
type or paste data and watch it pretty-format live — while keeping every v1 behavior
intact. It ships under a new major module path because new exported symbols cannot be
added to the frozen v1 surface (Go [semantic import versioning](https://go.dev/ref/mod#major-version-suffixes)).

## TL;DR

- **Read-only hosts:** change the import path. That is the only required change.
- **Editing is opt-in and additive:** nothing changes unless you pass `WithEditable()`.
- **No v1 symbol was renamed or changed semantics.** v2 is additive over v1, except the
  module path.

## The one required change (read-only hosts)

```sh
go get github.com/ideaconnect/go-fyne-pretty-view/v2@v2.0.0-alpha
```

```diff
-import prettyview "github.com/ideaconnect/go-fyne-pretty-view"
+import prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
```

Everything else is unchanged: `New`, `NewWithData`, `SetData`/`SetText`, `Source`,
`Reparse`, `Format`, the fold/scroll/search/selection API, all `With*` options, and the
`fonttheme` package keep their v1 names and semantics.

## Opt-in editing (new in v2)

Construct the widget with `WithEditable()`. The input-vs-output purpose is **fixed at
construction** — there is no `SetEditable` and the widget renders no view/edit toggle
(see [docs/DESIGN.md §12](docs/DESIGN.md)).

```go
ed := prettyview.New(
    prettyview.WithEditable(),
    prettyview.WithInputConfig(prettyview.InputConfig{AutoFormat: prettyview.AutoFormatOnPause}),
)
ed.SetOnChanged(func(text string) { /* mirror the edited text into your host */ })
ed.SetOnValidationChanged(func(s prettyview.ParseStatus) { /* show valid / error line */ })
```

A complete, compiled example lives in [examples/migrate/](examples/migrate/main.go).

### New surface (all additive)

| Area | Symbols |
|------|---------|
| Mode | `WithEditable()`, `Editable() bool` |
| Live formatting | `InputConfig{ DebounceFor, AutoFormat, MaxEditBytes }`, `AutoFormatMode` (`AutoFormatOff`/`AutoFormatOnPause`/`AutoFormatOnBlur`), `WithInputConfig`, `SetInputConfig`, `Reformat()` |
| Text | `Text() string` (the displayed/pretty text) — see below |
| Caret | `Caret() (line, col int)`, `SetCaret(line, col int) bool` |
| History | `Undo()`, `Redo()`, `WithUndoLimit(n)` |
| Clipboard | `Cut()`, `Paste()` (Copy already existed) |
| Validation | `ParseStatus{ OK, ErrorLine }`, `ParseStatus()`, `SetOnValidationChanged` |
| Callback | `SetOnChanged(func(string))` |

### `Text()` vs `Source()`

- **`Source() []byte`** — the raw bytes the user typed/loaded. In edit mode it is the
  live edit buffer (a fresh, owned copy); read-only, it is the v1 retained input.
- **`Text() string`** — the document *as displayed*: the pretty, depth-indented form
  after a reformat, or the raw text while typing.

Round-trip: `SetData(pv.Source(), pv.Format())` reproduces an equivalent document, so an
editable widget's edits reload into a read-only viewer losslessly.

### Not in v2.0

Runtime mode-flipping (`SetEditable`) is intentionally **out of scope** — the mode is
construction-time per the design decision above. A host-only runtime flip is parked in
the **Future Features** milestone ([#54](https://github.com/ideaconnect/go-fyne-pretty-view/issues/54))
and would land as a v2.x minor, never as a widget-rendered toggle.

## Renamed / changed-semantics table

| v1 symbol | v2 | Note |
|-----------|----|------|
| *(all)* | *unchanged* | No v1 symbol was renamed or had its semantics changed. The only breaking change is the module-path suffix `/v2`. |

The exported surface is pinned by `TestExportedSurfaceGolden`
(`testdata/api_surface.txt`), now frozen as the **/v2** surface; a future breaking change
ships under `.../v3`.

## Maintenance policy

- **v2** is where features land.
- **v1** is frozen: it receives **critical and security fixes only**, on a
  `v1-maintenance` branch. New features are not back-ported.
