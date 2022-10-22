# Recovery roadmap

The current release implements complete imports and verification. The work
below is planned, not available through the CLI. A checkbox is completed only
when implementation and regression tests land together.

## Durable, resumable imports

- [ ] Define a separate, versioned checkpoint format and job state machine.
- [ ] Record consumed source offsets at fully processed record boundaries.
- [ ] Define input identity and reject incompatible source/options on resume.
- [ ] Persist committed output lengths and validate retained output prefixes.
- [ ] Truncate uncommitted tails before appending new output.
- [ ] Rebuild duplicate-ID state from committed accepted records.
- [ ] Replace stale reservations with a documented, crash-aware ownership scheme.
- [ ] Make publication retries safe across rename and state-cleanup boundaries.
- [ ] Add cancellation, corruption, ownership, and crash-window tests.

## Acceptance criteria

For identical input, options, and writer version, an interrupted/resumed import
must produce the same accepted records, rejection diagnostics, counters, and
digests as an uninterrupted import.

Recovery must fail clearly rather than guess when state is damaged, input is
incompatible, committed files are short, or ownership cannot be established.
A live second process must not write the same job or destination.

Checkpoint accounting must use consumed record boundaries, not a buffered
reader's underlying file position. Oversized lines, blank lines, CRLF,
unterminated final lines, and duplicates across a checkpoint need explicit
tests.

Process-crash tests and real power-loss tests are different evidence. The
documentation must keep those guarantees separate.

## Later directions

Disk-backed duplicate tracking and configurable record schemas are possible
extensions after recovery correctness is established. No performance target
or delivery date is promised here.
