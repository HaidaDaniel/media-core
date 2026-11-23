# Task 0077 - Trusted LAN, local DNS access and deployment readiness

State: done
Owner: opencode-lan-20261123
Mode: mvp
Lane: implementation
Type: feature
Scope: network,security,web,systemd,docs,tests
Risk: high
Parallel: false
Depends on: none
Tags: feature

## Goal

Enable explicit Trusted LAN HTTP access on a configured private interface and local home.arpa hostname without a reverse proxy, while preserving all existing security invariants and public HTTPS behavior.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/scope.md
- docs/decisions.md
- docs/engineering/deployment.md
- docs/engineering/configuration.md
- docs/web-ui.md
- internal/config/config.go
- internal/runtime/runtime.go
- internal/httpapi/security.go
- internal/httpapi/session.go
- internal/httpapi/server.go
- web/src/api/client.ts

## Files allowed to edit

- internal/cli/cli.go
- internal/config/config.go
- internal/config/config_test.go
- internal/runtime/runtime.go
- internal/runtime/runtime_test.go
- internal/httpapi/security.go
- internal/httpapi/security_test.go
- internal/httpapi/session.go
- internal/httpapi/session_test.go
- internal/httpapi/server.go
- internal/httpapi/server_test.go
- internal/httpapi/routes.go
- internal/httpapi/routes_test.go
- web/src/api/client.ts
- web/src/api/client.test.ts
- web/e2e/library.spec.ts
- web/e2e/lan.spec.ts
- deploy/media-core.service.d/trusted-lan.conf
- tests/packaging/deployment_test.go
- tests/acceptance/trusted_lan_test.go
- docs/decisions.md
- docs/architecture.md
- docs/scope.md
- docs/web-ui.md
- docs/engineering/deployment.md
- docs/engineering/configuration.md
- docs/progress.md
- docs/implementation-plan.md
- README.md
- .tasks/0077-trusted-lan-local-dns-access-and-deployment-readiness.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- go.mod
- go.sum
- internal/store/migrations/*
- .github/workflows/*

## Steps

1. Explore current config/serve/listener, Host/Origin validation, cookies, frontend UUID usage, systemd units and release packaging
2. Add explicit Trusted LAN mode to config with private-IP/listen validation and lan-host validation; keep loopback default and public HTTPS trio intact
3. Wire --trusted-lan, --listen and --lan-host through runtime/serve; fail clearly on missing or invalid input
4. Allow exact configured IP and LAN hostname in Host and Origin checks; keep CSRF, auth, headers, rate limits and body limits
5. Make session Secure cookie conditional on HTTPS; keep it mandatory for public TLS
6. Replace crypto.randomUUID in the frontend with a getRandomValues-based UUID v4 generator; preserve request_id reuse on retry
7. Add a systemd drop-in example for Trusted LAN with no secrets and preserved hardening; extend packaging checks
8. Write Trusted LAN deployment/local DNS documentation and an ADR
9. Add unit, integration and browser regression tests; run full verification and independent review

## Acceptance criteria

- Default serve remains loopback-only and safe without flags
- --trusted-lan requires an explicit RFC1918 (or validated IPv6 ULA) interface IP, never wildcard/public/multicast/unspecified, and one explicit hostname without scheme or port
- HTTP LAN accepts only the configured IP Host and configured LAN hostname Origin; rejects unknown Host, foreign/missing Origin; CSRF and owner auth remain mandatory
- Public HTTPS remote mode behavior is unchanged; no permissive CORS, no wildcard Host/Origin, security headers retained
- Frontend works on insecure HTTP LAN origin with no crypto.randomUUID and no uncaught insecure-context errors; Quick Add retry reuses request_id
- Systemd drop-in example is verifiable, preserves hardening and contains no secrets; backup unit unchanged
- Full Go and web verification plus release build pass with recorded evidence; NAS deployment marked unverified until real smoke

## Invariants

- Trusted LAN is an explicit mode, never inferred from a private IP.
- Owner secret, sessions, CSRF, rate limiting, CSP, body limits and safe errors remain mandatory in LAN mode.
- HTTP never sets Secure cookies; HTTPS always does.
- No reverse proxy, embedded DNS, container, data-schema or business-logic changes.

## Review questions

- Can a private-IP listener or a `home.arpa` name accidentally enable Trusted LAN without the explicit flag, or combine LAN HTTP with the remote HTTPS trio?
- Does any path let an unexpected Host, foreign or missing Origin, missing CSRF token or missing session reach a mutation on the LAN origin?
- Does the insecure-context UUID generator keep version/variant bits and request_id reuse on retry, with no secure-context API or Math.random dependency?
- Is the systemd example only an optional drop-in, with no secrets and with the backup timer and default hardening unchanged?

## Counterexample searches

- Corrupt the drop-in (unknown section/key, removed ExecStart reset, secret value) and verify packaging checks fail.
- Try wildcards, public/loopback/link-local/multicast/mapped addresses, port 0, hostnames as listeners, scheme/port/path/wildcard/.local LAN hosts.
- Send hostile Host and Origin spellings, case variants, missing Origin/CSRF/session to both IP and hostname spellings.
- Remove `crypto.randomUUID` and stub `crypto.getRandomValues` in a browser; retry a lost mutation and compare request ids.
- Confirm no proxy, embedded DNS, container, migration or unrelated adapter change is present in the diff.

## Verification

- `{"id":"go-build","type":"automated","required":true,"environment":"local","profile":"report","command":"go build ./..."}`
- `{"id":"go-vet","type":"automated","required":true,"environment":"local","profile":"report","command":"go vet ./..."}`
- `{"id":"go-tests","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./..."}`
- `{"id":"go-race","type":"automated","required":true,"environment":"local","profile":"report","command":"go test -race ./..."}`
- `{"id":"web-tsc","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit"}`
- `{"id":"web-vitest","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run"}`
- `{"id":"web-playwright","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec playwright test"}`
- `{"id":"release-build","type":"automated","required":true,"environment":"local","profile":"report","command":"bash scripts/build-release.sh"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"doctor"}`
- `{"id":"apk-status","type":"automated","required":true,"environment":"local","profile":"report","command":"git status --short >/dev/null && pnpm exec apk status"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes
- Add Trusted LAN ADR in docs/decisions.md
- Document local DNS deployment and HTTP LAN security in docs/engineering/deployment.md

## Notes

- Milestone: M5. Deployment readiness. One new task; the previous prompt was never run and 0077 did not exist.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- No NAS deployment, host network/system configuration change, DNS router change, force push or publication is authorized.
- If no private network interface is available locally, record the real-network HTTP session smoke as unverified and prepare a reproducible NAS smoke rather than substituting loopback.
- Verification note: build-release.sh restores the tracked asset placeholder with a fresh mtime, so the first `apk status` after it can trip the APK diagnostic snapshot on a stale Git index. The apk-status check refreshes the index with a read-only `git status` before invoking `apk status`; the status check itself is unchanged.
