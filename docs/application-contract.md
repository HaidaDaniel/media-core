# Shared application and adapter contracts

Target v1. DTO names below freeze behavior for parallel implementation; Go/TypeScript types and fixtures will be created by task 0007, not claimed to exist now. Domain invariants are authoritative in [domain-model.md](domain-model.md).

## Common DTOs and errors

IDs are opaque random UUID strings. Instants are UTC RFC3339; unknown values are JSON null, never zero/empty strings invented as facts. `WorkView` contains work ID/revision, active titles and edition summaries. `EditionView` includes kind/language/revision, titles, consumption/intention/publication with source, progress-by-unit, rating, tags, references and notes; unavailable collections are empty, unknown scalar facts null. Relationships and history are paginated separately. Work cards preserve which edition matched; no mixed-edition filter matches or aggregated ratings.

```json
{"schema_version":"1.0","ok":true,"data":{"items":[],"next_cursor":null,"total":0}}
```

```json
{"schema_version":"1.0","ok":false,"error":{"code":"conflict","message":"Record changed; reload before retrying.","details":{"current_revision":8},"retryable":false}}
```

Errors: `invalid_argument` (400), `not_found` (404), `conflict`, `identity_conflict`, `duplicate_candidate`, `cycle_detected` (409), `unauthorized` (401), `forbidden` (403), `rate_limited` (429), `locked` (503/retryable), `schema_mismatch`, `storage_error`, `config_error`, `internal_error` (500). CLI uses exits 0 success, 1 runtime/config/storage, 2 invalid input or conflict, 3 partial import, 4 locked. Logs go to stderr; stdout contains one JSON document or MCP protocol messages only. Expected domain failures in MCP are typed structured tool results marked isError, not disguised successful results; malformed MCP messages use protocol errors.

Lists use opaque seek cursors tied to query/sort, default 30/max 100, stable ID tie-breaker, total counts before pagination. Reject cursor/query mismatch. Filters: query, kinds, consumption (including unknown), intention, tags, minimum rating and publication. AND between filter dimensions, OR within supplied values; edition-related predicates must match the same edition. Work tags OR that edition's tags can satisfy the tag predicate. Sort: title, updated_at, rating, created_at; missing ratings last. Activity is ordered recorded_at descending then ID, displaying observed_at separately. Initial filters use normalized text, no provider calls.

## Mutations and transactions

Every app mutation accepts `{request_id, actor, expected_revision?, ...payload}`. Required random request_id is an idempotency key scoped to operation; store canonical request hash and result in the **same** transaction as changes. Actor includes principal and asserted origin user/agent/import; transport populates principal rather than trusting an HTTP body. Exact replay returns the stored result, including after a timeout/restart. Same key with different payload returns conflict. Failed validation does not reserve a key. Results and keys are retained in backups for MVP (no premature expiration).

Existing-entity mutations require expected_revision; a same-value no-op succeeds without increment. All child changes bump owning edition/work revisions as applicable. One use case commits once: create work+initial title+optional edition+references; progress/status observation+audit+revision; confirm/revoke relation+cycle check; import event+binding/projection; mutation response/idempotency. Batch import uses one event per transaction and per-item results. Errors roll back the event, never leave a half-created work. Retry prompts must reload revision; a frontend must not silently replay changed intent.

`ResolveAddition` returns one of `ready`, `existing`, `candidates`. Title-only manual creation is always possible. For candidates it returns candidate IDs, evidence, request fingerprint and expiring token. `CreateLibraryEntry` requires choice `use_existing`, `add_edition` or `create_distinct` when candidates were found. Recompute candidates in the write transaction; token/request/candidate changes return conflict. No metadata enrichment is required. An exact identity match with contradictory kind/language/work is an identity_conflict, never an ignored field. New references for a known edition use AddReference.

Suggestions (titles, metadata, origin/relation proposals) include provenance and `state:proposed`; confirmation is explicit in all adapters. A provider-neutral future proposal is `{target?, titles:[], edition_hints:{}, references:[], relations:[], provenance, warnings:[]}`. It cannot write confirmed facts by itself. MVP accepts manual data and relation proposals; it does not implement provider fetching or a proposal workflow engine.

## HTTP route ownership

All paths below are relative to `/api/v1`. JSON-only request bodies, unknown mutation fields rejected, bounded values; same service validation applies after decoding. Authentication applies to every library endpoint.

| Task | Endpoints | App services |
| --- | --- | --- |
| 0024 | POST/GET/DELETE `/session` | Owner login/session/logout; transport only |
| 0025 | GET `/works`, `/works/{id}`, `/editions/{id}` | SearchLibrary, GetWork, GetEdition |
| 0026 | POST `/additions/resolve`, `/works`, `/works/{id}/editions`; PATCH `/works/{id}/titles`, `/editions/{id}` | ResolveAddition, CreateLibraryEntry, AddEdition, CorrectTitles, UpdateEdition metadata |
| 0027 | POST `/editions/{id}/progress`, `/editions/{id}/consumption`; GET `/editions/{id}/history` | RecordProgress, SetConsumption, GetHistory (history evolves as dimensions land) |
| 0028 | POST `/editions/{id}/intention`, `/editions/{id}/ratings`, `/editions/{id}/references`; POST/PATCH `/notes[/{id}]`; POST/DELETE `/tags/assignments` | SetIntention, RecordRating, Add/RetireReference, Add/ReviseNote, SetTag |
| 0029 | GET `/works/{id}/relations`, `/editions/{id}/relations`, `/activity`; POST `/relations`, `/relations/{id}/confirm`, `/relations/{id}/revoke`, `/origins` | ListRelated, Propose/Confirm/RevokeRelation, RecordOrigin, ListActivity |
| 0043 | POST `/imports/releases`, `/imports/{event_id}/bindings`; GET `/imports`, `/editions/{id}/releases` | ImportReleases, SetImportBinding, ListPending, ListReleases |

Reference retirement is POST to `/editions/{id}/references/{reference_id}/retire` in 0028. Observation correction uses the same creation route with supersedes_id; retraction uses POST `/editions/{id}/history/{record_id}/retract` in 0027/0028 as each dimension lands. Titles include retirement/replacement; edition metadata includes explicit archive with reason/replacement reference, never purge. GET work/edition includes work-scoped notes/tags without leaking data from another edition.

## CLI and MCP parity

CLI subcommands are registered by capability files: add/list/show/title/edition, progress/status, intention/tag, note/rate/reference, link/origin/related/activity, import/releases, backup/restore/export/doctor/version/init/migrate. CLI is an OS-trusted local interface; machine JSON uses the common envelope. Human messages may use RU/EN but keys never localize; initial English UI labels do not limit Unicode data.

MCP tool groups are specified in [hermes.md](hermes.md). Web and MCP must expose the same resolved identity, revisions, effective progress and historical observations. Transport tests compare those DTOs and error codes, not just successful status codes. Operational restore/backup remains CLI-only. No SQL, shell execution or arbitrary file tool is exposed.
