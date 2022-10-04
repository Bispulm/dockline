# Dockline

A Go command-line project for converting NDJSON input into verifiable local snapshots.

## Implementation plan

Start with the record contract and bounded line framing, then add the import
engine, snapshot publication, verification, and command-line interface.
Keep parsing and policy decisions separate from filesystem ownership.

The first milestone is complete imports with deterministic manifests.
Checkpointed recovery is a separate follow-on milestone.
