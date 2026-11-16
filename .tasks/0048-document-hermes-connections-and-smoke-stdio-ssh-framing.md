# Task 0048 - MCP connectivity for any agent: stdio and Streamable HTTP, documented and smoked

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: mcp
Scope: mcp
Risk: high
Parallel: false
Depends on: 0038,0039,0040,0041,0047
Tags: planned,m5

## Goal

Prepare media-core to work with Hermes and any other MCP agent over both supported transports — stdio (same host) and Streamable HTTP (opt-in, authenticated) — with deterministic connection smokes and client-agnostic instructions.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md

## Files allowed to edit

- docs/hermes.md
- tests/acceptance/hermes_test.go
- internal/mcp/http.go
- internal/runtime/runtime.go
- docs/progress.md
- package.json
- pnpm-lock.yaml

- .tasks/0048-document-hermes-connections-and-smoke-stdio-ssh-framing.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add the Streamable HTTP transport as an opt-in MCP mode: `media-core mcp --http --listen <addr> [--allow-remote] [--public-origin <url>] [--tls-cert <file>] [--tls-key <file>]`, built over the same in-process service with the pinned go-sdk's `NewStreamableHTTPHandler`. Every request requires the state-dir owner secret as a Bearer token (constant-time compare, same secret file and 0600 mode as the web session; a missing file is a distinct error). Non-loopback listeners reuse the explicit remote trio already enforced at configuration resolution. Plain `mcp` (stdio) and `serve` never enable any HTTP MCP endpoint. No new dependencies.
2. Smoke the stdio lifecycle against a temp DB from the built binary: initialize (pinned protocol version), tools/list, create_entry, search_library by a Russian/romaji alias, record_progress, get_history, then kill the process, restart it, and verify the same IDs and state; a killed process must release the database handle so the state directory reopens cleanly.
3. Smoke the same lifecycle over loopback Streamable HTTP from the built binary: initialize, tools/list, the same tool calls with the Bearer secret, plus rejections — a missing or wrong secret is 401 with no state touched, a non-loopback listen without the trio fails before any listener, and `serve` exposes no MCP endpoint.
4. Rewrite docs/hermes.md as the any-client connection document: stdio and Streamable HTTP examples for any MCP client (Hermes, Claude, opencode, and a generic JSON config), the SSH-tunnel topology kept as a documented alternative, the owner-secret / host-key / account boundaries, and an explicit note that real-client (Hermes) and NAS checks are pending actual client validation because no Hermes client is installed on this machine.

## Acceptance criteria

- Local MCP (stdio) needs no HTTP/Node/provider
- HTTP MCP is opt-in, owner-secret authenticated, loopback by default, remote only with the explicit trio, reuses the same in-process service, and adds no dependency
- Any-client instructions for both transports are provided, with real-client and NAS checks reported as unrun unless actually authorized and performed
- No real host deployment, publish/tag/push or invented license

## Invariants

- No real host deployment or arbitrary remote command construction
- No new dependencies; the Streamable HTTP transport comes from the pinned go-sdk
- `serve` never silently enables an MCP endpoint

## Review questions

- Does failure leave the prior state intact and the error distinguishable?
- Does the HTTP transport's authentication boundary match the existing owner-secret model (same secret, same rejection semantics, no second secret)?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory: missing and wrong Bearer secret, non-loopback listen without the trio, restart after a killed process, idempotent replay of a write tool call, and the serve-no-MCP-endpoint regression.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/acceptance -run \"TestHermes\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Verification-window note: the operator-ordered APK tooling upgrade (commit 71d70a9, v0.5.2 to v0.6.0: package.json, pnpm-lock.yaml, docs/progress.md) was committed during this task's released state between claim and reclaim; the operator-approved verification epoch carries those tooling paths into this task's verification scope, so package.json and pnpm-lock.yaml are listed as allowed here and receive no further task-owned changes.
- Hermes client is not installed on this machine (a stale data directory only); every client config example is labeled pending actual client validation, per the acceptance criteria.
- The previous SSH-framing fixture step is superseded: Streamable HTTP is now the cross-host transport, and the SSH topology remains documented in docs/hermes.md as an alternative for clients that cannot reach the endpoint directly.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
