# Release Watcher import contract fixtures

Sanitized representative fixtures for the Release Watcher version 1.0
import envelope. The Release Watcher contract is authoritative:
https://github.com/HaidaDaniel/release-watcher/blob/main/docs/integration-contract.md

These fixtures pin the envelope shape Media Core imports (file/stdin). They
are sanitized: no real publication data, no personal identifiers, and stable
synthetic ids. The importer tests in `internal/importer` and the store tests
in `internal/store` load these fixtures, so a producer contract change that
breaks the v1.0 shape is caught here.

Envelope shape (v1.0):

    {
      "schema_version": "1.0",
      "ok": true,
      "data": {
        "next_cursor": "evt_...",
        "events": [
          {
            "schema_version": "1.0",
            "event_id": "evt_...",
            "event_type": "release.added",
            ...typed, additive fields...
          }
        ]
      }
    }

Idempotency rule: the same `event_id` with a semantically identical payload
is a no-op replay; the same `event_id` with a different payload is an
identity conflict and is never an overwrite. Canonical JSON equality ignores
object key order and whitespace.

Files:

- `envelope_valid.json`: one valid `release.added` event.
- `envelope_replay.json`: the same event with reordered keys and extra
  whitespace; canonical-equal to `envelope_valid.json`.
- `envelope_conflict.json`: the same `event_id` with a different payload.
- `envelope_unknown_type.json`: a valid event of an unknown `event_type`;
  accepted and kept pending, never projected.
- `envelope_unknown_fields.json`: a valid event with additive fields Media
  Core does not know; accepted and preserved in the raw payload.
- `envelope_unsupported_major.json`: `schema_version` major 2; rejected
  before any write.
