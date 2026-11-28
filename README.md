# Media Core

A personal, local-first library of encountered, consumed and saved media: what you watched or read, where you stopped, what you thought, and what you saved for later. Anime, manga and original novels stay connected through explicit relationships while retaining their own progress and publication histories. This is not a recommendation or discovery service.

- Local-first Go + SQLite modular monolith; no mandatory AI, cloud, Docker or external metadata service.
- One binary: `media-core <command>`, `media-core mcp` (stdio or opt-in authenticated Streamable HTTP) and `media-core serve`.
- React + TypeScript + Vite UI embedded as static assets; Node.js is build tooling only.
- CLI and MCP work without HTTP. Only an enabled `serve`/`mcp --http` process stays resident.
- Dark-first cover grid and compact list library with URL-persisted filters, sidebar navigation and shared accessible UI components.
- Explicit, provider-based cover discovery (Kitsu, Shikimori, MangaDex, Open Library) with user confirmation, provenance and a bounded local cache; no background fetching and no third-party request while rendering saved data. See [artwork providers](docs/engineering/artwork-providers.md).
- Optional explicit Trusted LAN serves `http://media.home.arpa:8787` directly from the Go binary on a private interface, with no reverse proxy or embedded DNS; see [deployment](docs/engineering/deployment.md#trusted-lan-deployment).
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
go test -race ./...
pnpm --dir web exec tsc --noEmit
pnpm --dir web exec vitest run
pnpm --dir web exec playwright install --with-deps chromium
pnpm --dir web exec playwright test
bash -n scripts/build-release.sh scripts/backup-smoke.sh
go test ./tests/packaging -run TestPackageReleaseArtifacts -count=1
```

The same checks run in `.github/workflows/ci.yml`. `.github/workflows/release-gate.yml` builds both Linux binaries and `SHA256SUMS` into an Actions artifact on every run; on a pushed `v*` tag only, after the gate succeeds, it publishes them as public GitHub Release assets using the built-in `GITHUB_TOKEN` with `contents: write` and verifies anonymous HTTPS access. `workflow_dispatch` never publishes. Download, checksum verification and NAS install commands are in [deployment](docs/engineering/deployment.md#downloading-a-published-release). `python3 scripts/check-planning.py` validates the current task graph and documentation links; it does not replace application tests. `bash scripts/backup-smoke.sh <state-dir> <backup-dir>` proves that a backup can be restored into a scratch state directory without touching live data.
