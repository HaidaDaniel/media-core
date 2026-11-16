# Task 0062 - Refresh CLI usage text and doctor capability detection

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: application
Scope: application,planning
Risk: low
Parallel: true
Depends on: 0049
Tags: planned,m5,review-fix

## Goal

Remove the stale "MCP not implemented" usage line and restore the doctor warning for a missing web lint script if the script is not provided.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/engineering/configuration.md
- internal/cli/cli.go

## Files allowed to edit

- internal/cli/cli.go
- internal/cli/cli_test.go
- package.json
- web/package.json
- docs/progress.md
- .tasks/0062-refresh-cli-usage-text-and-doctor-capability-detection.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Describe `mcp` in the usage text as the implemented stdio and opt-in Streamable HTTP mode
2. Add the missing `web:lint` script with the project's real TypeScript/lint tooling, or remove the stale doctor expectation with a documented reason
3. Add a test pinning the usage output; re-run doctor and record which remaining warnings are operator decisions

## Acceptance criteria

- No command help claims MCP is reserved or unimplemented
- `apk doctor` no longer warns about a missing `web:lint` script
- Remaining warnings about license and `.env.example` are explicitly recorded as operator decisions or not applicable, not silently ignored

## Invariants

- User-facing help and tooling diagnostics describe the implemented system.

## Review questions

- Does the new lint script actually run a configured linter rather than a no-op that only satisfies detection?

## Counterexample searches

- Run every listed usage command and confirm each is reachable and no longer reserved.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/cli -run TestUsage && pnpm exec apk doctor"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of stale operator-facing text and tooling health. Scope expanded before editing: doctor detects the web lint capability from web/package.json, so that manifest is included alongside the root script.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
