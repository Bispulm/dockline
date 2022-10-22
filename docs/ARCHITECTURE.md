# Architecture

Dockline separates record semantics from persistence. The import engine receives
readers and writers; it does not choose paths, reserve destinations, or publish
snapshots. The CLI connects the engine to the filesystem layer.

## Data path

1. `cmd/dockline` installs cooperative SIGINT/SIGTERM handling.
2. `internal/cli` parses options, opens the source, and reserves the destination.
3. `internal/ndjson` frames physical lines with bounded payload retention.
4. `internal/model` validates a JSON object and returns its ID and compact bytes.
5. `internal/engine` applies policies, updates counters, and hashes the streams.
6. `internal/snapshot` syncs completed files and publishes them by rename.
7. `verify` independently checks the persisted snapshot's integrity contract.

The source digest includes every source byte: newline delimiters, whitespace,
invalid records, and skipped duplicates. Output records use LF delimiters.

## Invariants

- A successful snapshot contains at most one accepted record per decoded ID.
- Accepted records retain first-valid-occurrence order under skip policy.
- `lines_read = blank_lines + records_written + duplicate_records + rejected_records`.
- A successful fail-policy import has zero skipped records in that policy's category.
- Declared byte counts and digests match the corresponding streams.
- Unpublished output lives only in staging; the destination is absent.
- Publication exposes all three required files together under the stated
  local-filesystem and cooperating-writer assumptions.
- Cleanup never removes a published snapshot.

Manifest count validation subtracts from the total instead of adding untrusted
counts, avoiding integer overflow in the accounting check.

## Ownership and publication

Each destination has a sibling `.dockline-lock-<hash>` reservation directory.
Atomic directory creation serializes cooperating writers. Output is staged in
a uniquely named `.dockline-stage-*` directory in the same parent.

The writer syncs and closes data files, writes and syncs the manifest, syncs the
staging directory, renames it to the final destination, and syncs the parent.
After rename, the writer forgets its staging path so cleanup cannot delete
published data. Cleanup must also run after successful publication to release
the reservation.

An error after rename can leave a valid snapshot with uncertain durability.
Likewise, stdout can fail after publication. The correct response is verification,
not blind replacement or deletion.

The reservation has no liveness proof and is not automatically reclaimed.
Abruptly terminated jobs therefore require manual investigation. A stronger
ownership lifecycle belongs to the recovery work, not an undocumented shortcut.

## Design tradeoffs

**Standard library only.** No service dependency is needed for imports or
verification. Interfaces around I/O keep failure-path tests independent of a
database or broker.

**In-memory duplicate index.** A map keeps policy handling straightforward and
deterministic, but memory grows with the number of unique IDs. Disk-backed
deduplication is separate work.

**Byte-preserving number handling.** Raw JSON values retain large integer
literals without a floating-point round trip.

**Deterministic manifests.** Source paths and wall-clock timestamps are omitted.
This aids repeatability; it does not prove producer identity.

**Cooperative cancellation.** Context is checked between records, not inside an
arbitrary blocking reader. Signal tests keep input flowing rather than claiming
idle-stdin interruption.

## Testing boundaries

Unit tests cover parsing, line framing, policy decisions, malformed manifests,
write failures, cancellation, destination conflicts, and corrupted output.
Subprocess tests exercise actual Linux signals and cleanup. CLI tests check
source verification and publication surviving a failed summary write.

Local tests do not establish crash recovery, cross-platform atomicity,
distributed-filesystem correctness, or persistence through physical power loss.
Those guarantees require separate implementation and evidence.
