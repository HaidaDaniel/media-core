# Media Core

Media Core is a planned personal media library for reading and watching history: books, web novels, light novels, manga, films, series and anime, with progress, statuses, ratings, notes and explicit links between editions and adaptations.

Status: documentation only. This repository currently contains the accepted product, scope, architecture, domain, roadmap and integration-contract documents. No runtime code or business logic exists yet.

## Principles

- Local-first and offline-first. The library works without Internet or a hosted account.
- Agent-first. A stable CLI and JSON contract are the primary interface; MCP stdio arrives after the CLI.
- Go + SQLite, one binary, no daemon, no server, no Docker, no PostgreSQL, no Redis.
- Zero idle resource use. Operations run on demand and exit.
- Reading history is durable and exportable. Backup/restore is a core capability.
- Explicit relations, not automatic title matching. WN, LN, manga, anime and translations are distinct editions unless the user links them.
- No LLM dependency. Optional external enrichment or translation is a provider integration, never a core runtime requirement.

## Relationship to Release Watcher

[Release Watcher](https://github.com/HaidaDaniel/release-watcher) detects new publications and emits immutable, versioned events. Media Core consumes those events through a documented JSON contract; the projects do not share a SQLite file or a deployment.

See [docs/integration-contract.md](docs/integration-contract.md).

## Documents

- [Project](docs/project.md) and [scope/non-goals](docs/scope.md)
- [Architecture](docs/architecture.md)
- [Domain model](docs/domain-model.md)
- [Roadmap](docs/roadmap.md) and [progress](docs/progress.md)
- [Integration contract](docs/integration-contract.md)
- [Decisions](docs/decisions.md)

## Development workflow

APK v0.5.2 is pinned as a development dependency for task contracts, verification and gates. Node.js and pnpm are development tooling only; the product will be a Go binary.

```sh
pnpm install --frozen-lockfile
pnpm exec apkit status
pnpm exec apkit lint
```
