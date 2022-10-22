# Contributing

Keep changes focused on one behavior or invariant. Include a regression test
for a bug fix and document changes to CLI options, data formats, or persistence.

## Local checks

```sh
go version
make check
make demo
```

The supported runtime contract is Linux with a local filesystem. The Go
language baseline is 1.23. CI also targets the current stable Go release.

For parsing changes, run `make fuzz`. For import-engine changes, run
`make bench`, record the environment, and compare like-for-like input.
Do not generalize a local benchmark to production workloads.

## Persistence changes

Read `docs/ARCHITECTURE.md`, `docs/FORMAT.md`, and `docs/ROADMAP.md`.
Keep incomplete recovery state separate from completed snapshots. Tests must
make ownership, cancellation, write ordering, and post-publication cleanup
observable. Do not add a stale-lock deletion shortcut without proving ownership.

## Commit scope

Use descriptive messages such as:

```text
feat: add checkpoint state validation
fix: preserve output after a failed summary write
test: cover duplicates across a recovery boundary
docs: clarify source fingerprint verification
```

A commit should contain a coherent, buildable change. Do not split lines or
files into empty activity-only commits.

## Data and licensing

Use synthetic examples. Do not include customer data, employer code,
credentials, private infrastructure names, or captured production logs.

No open-source license has been selected for this repository yet.
