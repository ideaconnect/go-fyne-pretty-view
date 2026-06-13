# Security Policy

## Supported versions

The module is on the **`/v2`** major and ships `vX.Y.Z-alpha` releases (the `-alpha`
suffix marks pre-production maturity, not API churn — the exported surface is frozen).
Security fixes are made on the **latest `v2.x.y-alpha` tag** and `main`; the frozen **v1**
line receives critical/security fixes only, on the **`v1-maintenance`** branch (tagged
`v1.x.y`). Older alphas are not maintained — pin a tag and upgrade to pick up fixes.

## Reporting a vulnerability

Please report suspected vulnerabilities **privately**, not as a public issue:

- Use GitHub's **[private vulnerability reporting](https://github.com/ideaconnect/go-fyne-pretty-view/security/advisories/new)**
  (Security → Report a vulnerability), or
- email **security@idct.tech**.

Include a description, affected version/commit, and a minimal reproducer (ideally an
input that triggers the issue). We aim to acknowledge within a few business days and
to coordinate a fix and disclosure timeline with you.

## Threat model & posture

This is a **viewer**: it parses and renders **untrusted input** (JSON / JSONC / XML /
HTML / raw bytes) but never executes it, makes network calls, or writes files. The
relevant attack surface is the parsers and the render/selection math. Hardening in
place:

- **Tolerant parsers** that never trust structure; a `recover()` boundary degrades any
  unforeseen parser panic to the raw fallback rather than crashing the host.
- **Bounded work**: recursion/nesting caps, a 4 GiB source ceiling, an optional
  `WithMaxInputBytes` cap, and an `O(visible window)` render budget independent of
  document size.
- **No code execution and no ReDoS**: search regular expressions use Go's RE2
  (`regexp`), which matches in time linear in the input.
- **Continuous checks**: `FuzzParse` across every format runs in CI, and a
  `govulncheck` gate fails the build (and the release) on any reachable known
  vulnerability in the toolchain or dependencies.

Consumers that embed the widget inherit these properties; see the README "Credits and
third-party licenses" for the assets compiled into a linking binary.
