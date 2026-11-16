# Media Core

A personal, local-first library of encountered, consumed and saved media: what you watched or read, where you stopped, what you thought, and what you saved for later. Anime, manga and original novels stay connected through explicit relationships while retaining their own progress and publication histories. This is not a recommendation or discovery service.

- Local-first Go + SQLite modular monolith; no mandatory AI, cloud, Docker or external metadata service.
- One binary: `media-core <command>`, `media-core mcp` (stdio or opt-in authenticated Streamable HTTP) and `media-core serve`.
- React + TypeScript + Vite UI embedded as static assets; Node.js is build tooling only.
- CLI and MCP work without HTTP. Only an enabled `serve`/`mcp --http` process stays resident.
- Durable edition-specific history, explicit source/adaptation/translation links, safe duplicate handling and consistent backup/restore.
- Release Watcher supplies optional versioned JSON events through a separate database boundary.

Example: complete an anime, rate it, connect it to its unread manga source, save the manga for later with a phone-reading note and a `manga-translation-candidate` tag. Neither entry becomes rejected merely because reading is deferred.

## Documents

- [Product](docs/project.md), [scope](docs/scope.md), [requirements and acceptance scenarios](docs/product/requirements.md)
- [Architecture](docs/architecture.md), [domain model](docs/domain-model.md), [application/API contracts](docs/application-contract.md)
- [Web UI](docs/web-ui.md), [Hermes/MCP](docs/hermes.md), [integration contract](docs/integration-contract.md)
- [Roadmap](docs/roadmap.md), [task inventory, dependencies and coverage](docs/implementation-plan.md), [progress](docs/progress.md)
- [ADRs](docs/decisions.md), [technology](docs/engineering/tech-options.md), [load assumptions](docs/engineering/load-profile.md), [risks](docs/engineering/risk-register.md)

## Verification

The implementation tasks 0005–0049 and post-implementation fixes 0050–0063 are recorded in [progress](docs/progress.md), each with its own candidate-bound evidence. No deployment or publication is claimed.

```sh
pnpm install --frozen-lockfile
pnpm exec apk status --detail
pnpm exec apk doctor
pnpm exec apk lint
pnpm exec apk sync
python3 scripts/check-planning.py
go build ./...
go vet ./...
go test ./...
pnpm --dir web exec tsc --noEmit
pnpm --dir web exec vitest run
bash -n scripts/build-release.sh && go test ./tests/packaging -run TestPackageReleaseArtifacts
```

`python3 scripts/check-planning.py` validates the current task graph and documentation links; it does not replace application tests. Browser end-to-end scenarios run separately with `pnpm --dir web exec playwright test`.
