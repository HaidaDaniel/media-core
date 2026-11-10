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
3. A replay with the same event_id but a semantically different JSON payload is an identity conflict, not an overwrite (compare canonical JSON, ignoring object-key order and whitespace). Store the raw payload plus the typed fields needed for queries: event type, publication id, external work reference, release ids/numbers/titles/URLs, counts, source/detection times, work/edition hints.
4. Attach to an Edition explicitly. Import does not create a new Edition by fuzzy title match. If no edition is linked, keep the event as a pending `ImportRecord` without an active binding; the user/agent selects the edition. Binding/correction records are separate from the immutable event payload A `ReleaseRecord` is projected only after linking.
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

Field names and error codes are English; human text may be RU/EN. Exit codes: 0 success, 1 runtime failure, 2 usage/input, 3 partial import with per-item results, 4 lock held. Shared services, HTTP mappings and CLI behavior are defined in [application-contract.md](application-contract.md); the incremental milestones are in [roadmap.md](roadmap.md).

## Content concepts (future contract, not a network API)

`ContentReference`, `ContentAsset`, `ContentProvider`, `DeliveryTarget` and translation links are documented in [domain-model.md](domain-model.md). They are local records/providers, not a public protocol. Any future provider adapter must be explicit, optional, time-bounded and provenance-recording. No delivery endpoint is part of the MVP.

## Versioning

- Consumers must ignore unknown additive fields. Unknown event types are stored as pending ImportRecords but not projected into release/progress state; the importer reports them.
- Additive fields are allowed within a major event/CLI version.
- Removing/renaming a field or changing its meaning requires a new major version.
- Media Core records the original payload and schema version so a later migration can re-project events without asking Release Watcher to resend them.
- A consumer must stop on an unsupported major version rather than silently dropping data.

## Non-goals

- A shared database or a second source of truth for Release Watcher watches.
- Automatic title-based merging of WN/LN/translations.
- Treating Telegram or any notifier as the event source. Events are the source; notifications are presentation.


## Reviewed compatibility and application boundary (2026-11-10)

Checked the upstream [version 1.0 contract](https://github.com/HaidaDaniel/release-watcher/blob/main/docs/integration-contract.md), blob `2ba629f7cb0a374cbd275fd2d9aab54b5fc3632c`. The producer may evolve; task 0042 must pin representative fixtures and recheck compatibility. No producer repository changes are needed here.

- Validate the entire envelope/major version before processing; validate each event before its transaction. A mixed valid/invalid batch reports per-item outcomes (exit 3); malformed envelope or unsupported major stops before writes. Bounds apply to file/stdin input too.
- Retain unknown additive fields and unknown event types. Unknown types remain pending/unprojected with a warning. Event IDs are dedupe keys; release IDs are scoped to publication. `dedupe_key` is producer provenance, not a replacement local identity rule.
- Import binding explicitly associates a producer namespace + publication_id with an edition. Once confirmed, later events for that exact source identity can bind deterministically. Conflicting maps stop that item; title/URL hints never create an authoritative map. The producer namespace is a configured stable name (default `release-watcher`), not an arbitrary network location.
- Rebinding revokes the previous binding/projections and creates replacements while preserving raw payloads and history. Two known publication lines can point to the same edition only after explicit confirmation; their counts remain labeled separately, never summed.
- Count-only and RSS sliding-window semantics stay as above. Publication status observations carry event provenance. Metadata changes remain sourced observations/proposals; they do not silently rename user titles or invent completion.
- Imported releases, publication counts and personal consumption/progress are independent. A release import never changes intention, rating or reading position. No source inactivity implies abandonment.
- HTTP/MCP accept bounded JSON batches through shared import services (task 0043); only OS-trusted CLI accepts a file path. Browsers/MCP cannot ask the server to open arbitrary paths or execute Release Watcher. Recent activity includes explicitly labeled imported events; the library still works if every external integration is unavailable.

## Translation and metadata boundaries

Translator Agent and future manga translators can be referenced using edition language, publication/content/translation_project references, tags and translation_of links. They own translation execution and assets; Media Core owns personal history. No OCR, image editing, acquisition, rendering or models are implemented. Optional metadata providers return provenance-bearing proposals, never silently accepted identities; manual entry remains reliable. See [domain-model.md](domain-model.md) and [application-contract.md](application-contract.md).
