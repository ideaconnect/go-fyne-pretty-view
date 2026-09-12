#!/usr/bin/env bash
# release-notes.sh TAG [CHANGELOG] > body.md
#
# Extracts the CHANGELOG section for TAG (the block from "## [TAG]" up to the next
# "## [" heading) as the GitHub release body, and prints the heading's title after the
# date, if any, to stderr-free stdout of `--title`. Fails when the section is missing or
# empty, so a tag cannot be released without a written account of what it contains.
#
#   release-notes.sh v2.7.0            # the section body (markdown)
#   release-notes.sh --title v2.7.0    # "v2.7.0: <heading title>" for the release name
set -euo pipefail

mode=body
if [ "${1:-}" = "--title" ]; then
  mode=title
  shift
fi
tag="${1:?usage: release-notes.sh [--title] TAG [CHANGELOG]}"
changelog="${2:-CHANGELOG.md}"

heading=$(grep -m1 -E "^## \[${tag//./\\.}\]" "$changelog" || true)
if [ -z "$heading" ]; then
  echo "release-notes: no '## [$tag]' section in $changelog; add one before tagging" >&2
  exit 1
fi

if [ "$mode" = title ]; then
  # "## [v2.7.0] - 2026-09-12 - title" -> "v2.7.0: title"; older headings use an em dash.
  title=$(printf '%s\n' "$heading" | sed -E 's/^## \[[^]]+\][[:space:]]*[-—][[:space:]]*[0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]]*([-—][[:space:]]*)?//')
  if [ -n "$title" ] && [ "$title" != "$heading" ]; then
    printf '%s: %s\n' "$tag" "$title"
  else
    printf '%s\n' "$tag"
  fi
  exit 0
fi

body=$(awk -v tag="$tag" '
  /^## \[/ { if (found) exit; if (index($0, "## [" tag "]") == 1) { found = 1; next } }
  found { print }
' "$changelog")
# Strip leading/trailing blank lines.
body=$(printf '%s\n' "$body" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')
if [ -z "$(printf '%s' "$body" | tr -d '[:space:]')" ]; then
  echo "release-notes: the '## [$tag]' section in $changelog is empty" >&2
  exit 1
fi
printf '%s\n' "$body"
