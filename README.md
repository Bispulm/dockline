# Dockline

**Deterministic NDJSON imports. Verifiable local snapshots.**

Dockline is a Go command-line tool that validates line-delimited JSON,
applies duplicate and invalid-record policies, and publishes a snapshot
with SHA-256 integrity checks.

## Quick start

Use Linux and Go 1.23 or newer. There are no third-party Go dependencies.

```sh
go test ./...
make build
./bin/dockline import --input examples/events.ndjson --output out/events
./bin/dockline verify --snapshot out/events --input examples/events.ndjson
```

The output directory must not already exist. It contains accepted records,
rejection reasons, and a manifest. Records stay in NDJSON.

This release implements complete imports. Checkpointed recovery is planned;
there is no resume command or option yet.
