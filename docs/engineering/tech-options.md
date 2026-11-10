# Technology choices

Target choices; no runtime dependencies installed by this documentation phase. Every implementation dependency must be pinned and recorded in [decisions](../decisions.md) by its owning task.

| Concern | Choice | Reason / rejected alternative |
| --- | --- | --- |
| Runtime | Go modular monolith, one binary | Simple Linux NAS deployment; no split backend services |
| Persistence | SQLite, recommended modernc.org/sqlite, local WAL | Pure Go and cross compilation; no server DB; task 0008 verifies supported version/snapshot behavior |
| Titles | golang.org/x/text normalization/case folding | Unicode semantics beyond SQLite ASCII lower; task 0010 records/pins dependency; no vectors/LLM |
| HTTP | Go net/http routing/middleware | Small surface; no framework requirement |
| UI | React, TypeScript, Vite, semantic HTML/CSS, typed fetch | User-required stack, static build; no SSR/Node production service |
| MCP | Maintained Go SDK selected/pinned in 0037 | Protocol implementation reuse; handwritten framing is unnecessary risk; transport stdio initially |
| Sessions | Single owner secret, opaque in-memory cookie sessions + CSRF | No hosted auth or multi-user RBAC; restarts intentionally require login |
| Relationships | SQLite edge tables and transactional validation | Small explicit graph; graph database unnecessary |
| Histories | Ordinary relational observation/revision rows | Recoverability without event sourcing/CQRS |
| Tests | Go testing/httptest and temporary DBs; lightweight frontend component tests; Playwright browser acceptance recommended | Tools added only in owning tasks; no existing application checks implied |
| Build | Frontend build staged into Go embed; CGO_ENABLED=0 Linux amd64/arm64 | One distributable; Node/pnpm only build/development tooling |

Use native services or manual execution for NAS documentation; Docker is optional outside this MVP plan. No provider is chosen before the manual library is useful. Native remote MCP requires a later transport/auth decision; SSH solves the initial remote-host topology.
