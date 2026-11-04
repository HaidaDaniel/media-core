# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- Go module, build identity and SQLite migrations.
- MediaWork, Edition, progress, status, rating, note and relation storage.
- CLI commands with stable JSON, RU/EN human output and backup/restore.
- Release Watcher event import with idempotent `event_id` handling.
- MCP stdio over the same use cases.
- ContentReference/ContentAsset/ContentProvider and DeliveryTarget as future read-only concepts.
- Translation harness provenance and content delivery modules.

## Implemented

- Documentation-only foundation: README, project, scope, architecture, domain model, roadmap, integration contract and ADRs.
- APK v0.5.2 pinned as a development-only dependency.
- No runtime code, no SQLite schema, no CLI, no MCP.

## Tested

- APK contract lint and documentation consistency (task 0002 evidence). No application tests exist.

## Deployed

- Not deployed. There is no runtime artifact.

## Blocked

- Runtime work is not blocked; it is intentionally deferred to separate implementation tasks.
- Release Watcher import cannot be tested until both projects have matching event fixtures and a Media Core importer.

## Next step

Create the Stage 1 local-library task: Go bootstrap, SQLite schema and the first CLI vertical slice.
