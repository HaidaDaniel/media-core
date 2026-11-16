# Task 0056 - Configure Origin protection for Streamable HTTP MCP

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp,security
Risk: medium
Parallel: true
Depends on: 0048
Tags: planned,m5,review-fix

## Goal

Apply the documented MCP Origin protection to the opt-in Streamable HTTP transport.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/hermes.md
- internal/mcp/http.go

## Files allowed to edit

- internal/mcp/http.go
- internal/mcp/http_test.go
- docs/progress.md
- .tasks/0056-configure-origin-protection-for-streamable-http-mcp.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Set `CrossOriginProtection` in the `StreamableHTTPOptions` of the shared handler
2. Confirm the SDK default is not relied on, since a nil value disables the check unless a compatibility flag is set
3. Add a test that a hostile Origin is rejected while a same-origin request with a valid Bearer token still reaches the protocol

## Acceptance criteria

- Cross-origin requests fail before protocol handling with the SDK's denial response
- Bearer authentication and the distinct missing-secret/wrong-token 401s still behave as before
- Stdio MCP and `serve` still expose no MCP endpoint

## Invariants

- The authenticated Streamable HTTP transport keeps the same origin and authentication protections promised by the architecture.

## Review questions

- Does the configured protection affect the loopback CLI/agent smokes that send no Origin header?

## Counterexample searches

- Send a well-formed initialize with a valid Bearer token and a foreign Origin through both the handler test and the built-binary smoke harness.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp ./tests/acceptance -run TestMCPHTTPOrigin"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of an unmet documented security requirement.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
