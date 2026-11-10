# Media Core

A planned personal media library and consumption history system: what you watched or read, where you stopped, what you thought, and what you saved for later. Anime, manga and original novels stay connected through explicit relationships while retaining their own progress and publication histories.

**Documentation and planning only. No application runtime exists yet.** The target MVP includes both a responsive Web UI and Hermes/MCP, with a CLI for local operations. This is not a recommendation or discovery service.

- Local-first Go + SQLite modular monolith; no mandatory AI, cloud, Docker or external metadata service.
- One binary: `media-core <command>`, `media-core mcp`, `media-core serve`.
- React + TypeScript + Vite UI embedded as static assets; Node.js is development tooling only.
- CLI and MCP work without HTTP. Only an enabled `serve` process stays resident.
- Durable edition-specific history, explicit source/adaptation/translation links, safe duplicate handling and consistent backup/restore.
- Release Watcher supplies optional versioned JSON events through a separate database boundary.

Example: complete an anime, rate it, connect it to its unread manga source, save the manga for later with a phone-reading note and a `manga-translation-candidate` tag. Neither entry becomes rejected merely because reading is deferred.

## Documents

- [Product](docs/project.md), [scope](docs/scope.md), [requirements and acceptance scenarios](docs/product/requirements.md)
- [Architecture](docs/architecture.md), [domain model](docs/domain-model.md), [application/API contracts](docs/application-contract.md)
- [Web UI](docs/web-ui.md), [Hermes/MCP](docs/hermes.md), [integration contract](docs/integration-contract.md)
- [Roadmap](docs/roadmap.md), [task inventory, dependencies and coverage](docs/implementation-plan.md), [progress](docs/progress.md)
- [ADRs](docs/decisions.md), [technology](docs/engineering/tech-options.md), [load assumptions](docs/engineering/load-profile.md), [risks](docs/engineering/risk-register.md)

## Planning checks

APK 0.5.2 is pinned for development. These commands check planning, not an implemented application:

```sh
pnpm install --frozen-lockfile
pnpm exec apk status --detail
pnpm exec apk doctor
pnpm exec apk lint
pnpm exec apk sync
python3 scripts/check-planning.py
```

Start implementation with task 0005 after planning task 0004 passes its gate. Future tasks remain unclaimed `todo`. Task 0003 is an existing adoption follow-up; its history and state are preserved.
