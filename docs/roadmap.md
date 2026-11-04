# Roadmap

The order is dependency-driven. A stage is complete only with implementation and named verification evidence; documentation does not complete a stage.

## Stage 0 — Documentation (task 0002)

- README, project, scope, architecture, domain model, roadmap, integration contract, decisions.
- No runtime code.

## Stage 1 — Local library core

- Go module, build identity, SQLite migrations.
- MediaWork, Edition, ProgressRecord, ConsumptionStatus, Rating, Note, MediaRelation.
- CLI `add`, `list`, `show`, `progress`, `status`, `rate`, `note`, `link`, `remove`, `export`, `backup`, `restore`, `doctor`, `version`.
- Stable JSON envelope; RU/EN human output; on-demand execution only.
- Tests: persistence/restart, progress history, status/rating/note, relations, backup/restore, invalid input, JSON.

Exit: a user can maintain a local library without Internet, and a backup restores it.

## Stage 2 — Release Watcher import

- Read `release-watcher events --json` (or an exported file) using schema version 1.0.
- Store immutable imported events, dedupe by `event_id`, attach to an explicit Edition.
- Show release history and recent events; do not auto-merge editions.
- Handle count-only events without inventing chapter numbers.
- Tests: first import, replay idempotency, event types, sliding-window semantics, unknown publication, schema mismatch.

## Stage 3 — Editions and relations

- Explicit same-story/translation/adaptation links across WN, LN, manga, anime.
- Provenance and user correction.
- No automatic similarity merge.

## Stage 4 — MCP stdio

- Same application use cases as the CLI; no separate business logic.
- Stable tool/result JSON; local only in the first version.

## Stage 5 — Content references and assets (read-only)

- ContentReference, ContentAsset, ContentProvider records.
- Register/verify an asset by hash; link to edition and provenance.
- No delivery or scraping by default.

## Stage 6 — Delivery and translation (future)

- DeliveryTarget capabilities and delivery state.
- Original/translation asset links with external harness provenance.
- Format conversion/delivery only when a real device scenario justifies it.

## Explicitly deferred

Hosted sync, web UI, social features, recommendations, streaming, DRM, automatic scraping, mandatory LLM, plugin marketplace, microservices, Docker/PostgreSQL/Redis as runtime requirements, and a shared SQLite file with Release Watcher.
