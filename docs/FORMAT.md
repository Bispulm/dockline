# Snapshot format v1

The format identifier is `dockline.snapshot.v1`. A snapshot contains:

| File | Contract |
| --- | --- |
| `records.ndjson` | Compacted accepted JSON objects, LF-delimited, in source order |
| `rejected.ndjson` | Objects with integer `line` and nonempty string `reason` |
| `manifest.json` | Format/version, options, counters, byte lengths, SHA-256 digests |

Additional files are not part of the current verifier's integrity guarantee.

## Manifest fields

`tool_version` identifies the writer. `options` contains `invalid_policy`,
`duplicate_policy`, and `max_record_bytes`.

`stats` contains nonnegative signed 64-bit counters:

```text
lines_read
blank_lines
records_written
duplicate_records
rejected_records
source_bytes
records_bytes
rejected_bytes
```

A terminal newline does not create an additional empty line. A delimiter-only
line is a blank line. Empty input has zero lines.

`digests` contains `source_sha256`, `records_sha256`, and `rejected_sha256`.
Writers emit lowercase hexadecimal SHA-256. Empty streams have the SHA-256
digest of zero bytes. Source hashing preserves the exact input line endings;
accepted output always uses LF.

## Validation

Verification rejects unsupported formats, unknown manifest fields, malformed
JSON, invalid options, negative or inconsistent counts, malformed hashes,
duplicate accepted IDs, malformed accepted records, and incorrect byte counts
or digests. Rejection line numbers must increase strictly and cannot exceed the
declared number of input lines.

Optional source verification compares the full source's digest and byte count.
It does not reconstruct duplicate decisions or replay policies.

## Recovery boundary

This format describes only complete snapshots. A partial import must never be
represented as a completed v1 manifest. Future checkpoints need a separate,
versioned internal state format and explicit recovery validation.

The tool is scoped to accidental-corruption detection, not hostile modification
of both the data and its manifest.
