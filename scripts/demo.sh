#!/bin/sh
set -eu

root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$root"
work=$(mktemp -d "${TMPDIR:-/tmp}/dockline-demo.XXXXXX")
trap 'rm -rf "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

go build -trimpath -buildvcs=false -o "$work/dockline" ./cmd/dockline

printf '%s\n' '--- Import mixed records (skip invalid entries and duplicate IDs) ---'
"$work/dockline" import \
  --input examples/mixed.ndjson \
  --output "$work/snapshot" \
  --invalid skip \
  --duplicates skip

printf '\n%s\n' '--- Accepted records ---'
cat "$work/snapshot/records.ndjson"
printf '\n%s\n' '--- Rejected record diagnostics ---'
cat "$work/snapshot/rejected.ndjson"
printf '\n%s\n' '--- Verify the snapshot and original source ---'
"$work/dockline" verify \
  --snapshot "$work/snapshot" \
  --input examples/mixed.ndjson
