# Architecture

Status: accepted target. No runtime code exists yet.

## Shape

Media Core is a modular monolith in one Go binary, invoked as a CLI, with SQLite as the only datastore. It is not a server and does not require a resident process. MCP stdio, when added, is another adapter over the same application use cases.

```text
cmd/media-core             process entrypoint
internal/cli               argument parsing, human/JSON rendering
internal/app               use cases: library, progress, status, ratings, notes, relations,
                           release import, backup/restore, export
internal/domain            works, editions, progress, events, relations, content concepts
internal/store             SQLite repositories, migrations, transactions
internal/importer          Release Watcher event import and idempotency
internal/buildinfo         version/commit/build identity
migrations                 embedded append-only SQL migrations
```

Dependencies point inward. CLI and future MCP depend on application use cases; the domain does not depend on SQLite, MCP or a content provider.

## Data flow

1. The user or an agent calls a CLI command.
2. The application validates the request, applies one transaction and returns a typed result.
3. Release Watcher events are read from the documented JSON interface and imported idempotently by `event_id`.
4. An imported release is attached to one explicit edition/publication line. Titles are never used to merge editions automatically.
5. Progress, status, rating and notes are ordinary user-owned records with timestamps and provenance.

## Scheduling and resources

There is no scheduler and no daemon. The process runs, does one operation and exits. Zero RAM is consumed while idle. A future optional local index or delivery job must remain on-demand unless a real need justifies a separate documented process.

## Storage

- SQLite, pure-Go driver, WAL with a busy timeout, short transactions.
- State lives outside the source tree (`$XDG_DATA_HOME/media-core` or `MEDIA_CORE_STATE_DIR`).
- Append-only migrations with a schema version.
- Backup is a consistent SQLite copy plus a manifest; restore validates schema/hash and requires exclusivity.
- No shared database file with Release Watcher.

## Agent interfaces

The CLI is the first interface. Machine output is a versioned JSON envelope with stable field names and codes. MCP stdio reuses the same use cases and error codes; it does not duplicate business logic. There is no mandatory HTTP API.

## Content delivery boundary

Content delivery is a future module. The architecture reserves concepts, not implementations:

- `ContentReference` identifies external content.
- `ContentAsset` represents a stored or deliverable object with hash, media type, language, provenance and rights.
- `ContentProvider` can resolve references or acquire assets.
- `DeliveryTarget` describes a device/library destination and its capabilities.
- Translation links tie an original asset to a translated asset and an external harness reference.

None of these perform network calls or content transfer in the MVP. They are documented in [domain-model.md](domain-model.md) and must not become a generic plugin platform.

## Explicit non-architecture

No event sourcing, CQRS, message broker, microservices, graph database, mandatory cloud sync, multi-tenant RBAC, web UI, plugin runtime, or LLM dependency in the core. Those require a real, recorded need.
