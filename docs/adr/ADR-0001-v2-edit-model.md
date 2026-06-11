# ADR-0001 — v2 edit model

- **Status:** Accepted (2026-06-11)
- **Milestone:** v2.0.0 — editable input + live formatting
- **Issue:** [#36](https://github.com/ideaconnect/go-fyne-pretty-view/issues/36)
- **Full design:** [docs/DESIGN.md §12](../DESIGN.md) — this ADR is the one-page summary.

## Context

v1 is a read-only viewer built to a hard memory bound (only viewport-many row
widgets; a pointer-free struct-of-arrays model with zero-copy byte segments;
model-based selection/search/copy). v2 must let a host accept typed/pasted input and
pretty-format it live, **without** discarding those properties — and the v1 public
surface is frozen (`TestExportedSurfaceGolden`), so any new API forces a new major.

## Decision

1. **Rebuild from bytes, never mutate the model.** Editing mutates a separate, owned
   **gap buffer over `[]byte`**. On a debounced pause the buffer is reparsed
   wholesale into a fresh, internally-immutable `Document` that atomically replaces
   the previous one (the existing `SetData` path). In-place model edits are rejected —
   they would shift every `SrcStart/SrcEnd`, re-thread `SegFirst`, and re-balance the
   fold index, O(document) per keystroke.
2. **Gap buffer**, not a piece table or rope: edits are caret-local, `Src` is already
   contiguous, and `buf.Bytes()` collapses to one `copy` for the parser. Between
   keystrokes the buffer renders through the cheap raw-text projection; structured
   reparse runs only on the debounced pause.
3. **Mode is construction-time and immutable.** `WithEditable()` (default = read-only)
   + a read-only `Editable() bool` accessor. **No `SetEditable`, no
   `OnEditModeChanged`, no chrome toggle** — "its purpose, input or output, should be
   defined, not user changeable." A host-only runtime flip is deferred to the Future
   Features milestone ([#54](https://github.com/ideaconnect/go-fyne-pretty-view/issues/54)).
4. **Ship as a `/v2` module path** ([#37](https://github.com/ideaconnect/go-fyne-pretty-view/issues/37)),
   re-baselining the surface golden. Read-only stays the default, so a v1 host
   migrates by changing only the import path.
5. **Size-cap fallback.** Above `MaxEditBytes`, disable auto-format-on-pause and keep
   raw-highlight-only editing; structured reformat then runs only on explicit
   `Format()`.

## Consequences

- **Held (5 of 7 invariants):** only-visible-rows-are-widgets, per-row culling,
  SoA/pointer-free/zero-copy model (per snapshot), model-based selection/search/copy,
  one coordinate convention. A read-only v2 widget is byte-for-byte equivalent to v1.
- **Traded (2):** *immutability-after-build* (a live mutable buffer now sits beside
  the model, replaced on each reformat) and *memory* (gap buffer + one transient
  reparse snapshot — a documented, `MaxEditBytes`-bounded delta).
- Caret is a model position re-resolved against each rebuilt model; a semantic anchor
  across reformat is specified in [#41](https://github.com/ideaconnect/go-fyne-pretty-view/issues/41).
