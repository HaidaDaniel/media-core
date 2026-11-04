# Integration contract

Status: documented target. Media Core does not implement an importer yet. The Release Watcher contract is authoritative: https://github.com/HaidaDaniel/release-watcher/blob/main/docs/integration-contract.md.

## Principles

- Separate repositories and separate SQLite databases. Media Core never opens Release Watcher's database file.
- The interface is immutable, versioned JSON events, not shared tables.
- Import is idempotent by `event_id`; repeated reads/imports are no-ops.
- Release Watcher events describe publication lines (WN, LN, translation, mirror), not a merged title. Media Core must not merge editions while importing.
- Media Core may run without Release Watcher; missing events do not block the library.

## Release Watcher event input

Release Watcher exposes events as:

```json
{"schema_version":"1.0","ok":true,"data":{"next_cursor":"evt_...","events":[{"schema_version":"1.0","event_id":"evt_...","event_type":"release.added"}]}}
```

Media Core import rules for every event:

1. Require `schema_version` major `1`. An unsupported major stops the import with a clear error; it is not guessed.
2. Deduplicate by `event_id` before applying anything. A repeated event is a no-op.
3. Store the raw payload plus the typed fields needed for queries: event type, publication id, external work reference, release ids/numbers/titles/URLs, counts, source/detection times, work/edition hints.
4. Attach to an Edition explicitly. Import does not create a new Edition by fuzzy title match. If no edition is linked, keep the event in a pending import queue; the user/agent selects the edition.
5. Never sum release counts across editions. `release.added` count-only events keep `current_count`/`count_delta` and no invented numbers.
6. An event is not updated in place. Corrections arrive as new events or user annotations.
7. Deleting/pausing a Release Watcher watch never deletes Media Core history.

## Event types

| Release Watcher event | Media Core effect |
| --- | --- |
| `release.added` | Append release records; update known last release. |
| `release.updated` | Append a revision/observation; do not delete the old release. |
| `release.removed` | Mark the linked release removed only with authoritative evidence; keep history. |
| `publication.metadata_changed` | Record title/metadata provenance; do not silently rename a user edition. |
| `publication.status_changed` | Store source publication status; never overwrite the user's consumption status. |
| `release.count_changed` | Store count change; no invented chapter numbers. |

RSS/Atom events use opaque entry ids; they are scoped to a publication. Media Core must not assume numbering is contiguous or globally unique.

## Import interface

Planned local commands:

```sh
media-core import releases --from <release-watcher-json-or-export>
media-core import releases --from -            # stdin
media-core releases <edition-id> --json
media-core pending-imports --json
```

The import is a local file/stdin operation. There is no mandatory network connection between the two projects. An optional remote fetch is a future adapter, not a core dependency.

## Media Core CLI JSON envelope

Planned stable envelope, analogous to Release Watcher:

```json
{"schema_version":"1.0","ok":true,"data":{}}
{"schema_version":"1.0","ok":false,"error":{"code":"not_found","message":"..."}}
```

Field names and error codes are English; human text may be RU/EN. Exit codes: 0 success, 1 runtime failure, 2 usage/input, 3 partial import with per-item results, 4 lock held. The exact command set is defined by the Stage 1 task, not by this document.

## Content concepts (future contract, not a network API)

`ContentReference`, `ContentAsset`, `ContentProvider`, `DeliveryTarget` and translation links are documented in [domain-model.md](domain-model.md). They are local records/providers, not a public protocol. Any future provider adapter must be explicit, optional, time-bounded and provenance-recording. No delivery endpoint is part of the MVP.

## Versioning

- Additive fields are allowed within a major event/CLI version.
- Removing/renaming a field or changing its meaning requires a new major version.
- Media Core records the original payload and schema version so a later migration can re-project events without asking Release Watcher to resend them.
- A consumer must stop on an unsupported major version rather than silently dropping data.

## Non-goals

- A shared database or a second source of truth for Release Watcher watches.
- Automatic title-based merging of WN/LN/translations.
- Treating Telegram or any notifier as the event source. Events are the source; notifications are presentation.
