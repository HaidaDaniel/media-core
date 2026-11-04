# Scope

## MVP scope

A usable local library for one person:

- Add and edit media works and editions: WN, LN, book, manga, film, series, anime, other.
- Track progress by chapter, volume, episode, page or percentage.
- Set status: planned, in progress, paused, completed, dropped, on hold, unknown.
- Record rating, notes, quotes and dates started/finished.
- Keep release history imported from Release Watcher.
- Link editions explicitly: same story, translation, adaptation, side story, related.
- CLI with human and stable JSON output; local SQLite; backup/restore; readable export.
- Optional MCP stdio over the same application use cases after the CLI is proven.

## Future modules (not MVP)

- ContentReference/ContentAsset/ContentProvider read-only resolution.
- DeliveryTarget and content delivery to devices (e.g. e-readers).
- Translation harness integration, bilingual assets and provenance.
- Optional external metadata/cover providers.
- Import/export adapters beyond the documented event and backup contracts.

## Non-goals

- Streaming, DRM circumvention, scraping behind authorization/paywalls, or a content marketplace.
- Social features, public profiles, recommendations, hosted accounts or mandatory cloud sync.
- Automatic merging of WN/LN/manga/anime by similar titles.
- A mandatory LLM, mandatory translation, or per-scan model calls.
- A continuously running server, Docker/PostgreSQL/Redis/Kafka, microservices or a plugin platform as runtime requirements.
- Sharing Release Watcher's SQLite file. Integration is through the versioned event contract and backup/export.
- A third deployment-framework repository. Deployment will be documented as an operational concern, not a new product.

## Boundary with Release Watcher

Release Watcher owns watching external sources, detecting releases and emitting events. Media Core owns the user's library, progress, opinions, relationships and future content assets. Either can run without the other; together they form a local media workflow.

## Quality bar

The smallest correct local feature wins. A feature belongs in the MVP only when the personal library scenario needs it. Unknown source semantics stay unknown; an edition is never silently merged; imported events remain traceable to their origin.
