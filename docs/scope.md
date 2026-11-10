# Scope

## MVP

- One personal library, offline and local-first, with works, multilingual titles and edition lines for web/light/traditional novels, manga/manhwa/manhua, anime, movies, TV and `other`.
- Distinct personal consumption, intention and sourced publication status; edition progress/history, notes, 1–10 ratings, tags and external references.
- Explicit provenance-bearing adaptation/translation/version/story relationships, origin claims and corrections; no fuzzy merges.
- CLI, MCP stdio and HTTP over common application services. Web UI and Hermes/MCP are equally required before MVP completion.
- Library, quick add, work/edition details, related media and recent history screens. Manual title-only entry always works.
- Secure loopback HTTP by default, browser session protection, explicit authenticated TLS configuration for non-loopback access. No multi-user RBAC.
- Embedded React/TypeScript/Vite assets in one Go binary; no Node production dependency.
- SQLite migrations, restart durability, readable JSON export, consistent backup/restore, cross-process access tests.
- Idempotent Release Watcher file/stdin imports, explicit edition binding, pending imports and release history. Imported publication information never advances personal progress.
- Hermes same-host stdio and documented SSH stdio bridge for different hosts.

## Deferred

Native remote Streamable HTTP MCP (SSH suffices for initial NAS use), metadata/cover fetching, provider-specific title normalization, vector search, full graph visualization, content acquisition/rendering/delivery, translation execution, OCR/inpainting, hosted sync, social features and recommendation engines.

`ContentReference`, `ContentAsset`, `ContentProvider` and `DeliveryTarget` remain documented extension concepts. No tables or plugin framework are required for unused future concepts. A source or translation-project reference can already be an ExternalReference.

No microservices, event sourcing, CQRS, graph database, Redis, PostgreSQL, broker, Kubernetes or mandatory Docker. No shared SQLite file with Release Watcher or Translator Agent. No deletion of history as an incidental effect of pausing, dropping or removing an external watch. Destructive purge and automatic merge are outside the MVP.

## Scope changes from the previous plan

Web UI and an optional resident HTTP mode are now in scope; MCP is a release requirement rather than a later optional stage. ADR-0009 explicitly supersedes the contrary parts of ADR-0002/0003. The practical milestones are [roadmap](roadmap.md); the complete MVP includes M1–M5, not just the first CLI or web slice.
