# Task 0068 - Reconcile operator documentation with the implemented CLI

State: todo
Owner: none
Mode: mvp
Lane: planning
Scope: docs
Risk: medium
Parallel: false
Depends on: 0064,0065,0066,0067
Tags: planned,m5,audit,p3

## Goal

Remove factual contradictions between README, architecture, deployment, CLI help and Hermes instructions and the code that is actually shipped.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/engineering/deployment.md
- docs/hermes.md
- internal/cli/cli.go

## Files allowed to edit

- README.md
- docs/architecture.md
- docs/engineering/deployment.md
- docs/hermes.md
- internal/cli/cli.go
- internal/cli/cli_test.go
- docs/progress.md
- .tasks/0068-reconcile-operator-documentation-with-the-implemented-cli.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- docs/decisions.md
- scripts/build-release.sh

## Steps

1. Add the five missing commands (export, doctor, releases, import, pending-imports) to CLI usage and pin them in a test
2. Correct architecture.md: the owner secret is created by `owner generate`, not `init`; restore keeps the old database intact on failure rather than setting one aside; remove the unenforced 512-code-point title limit
3. Correct deployment.md: `serve` does not generate a secret, regeneration cannot replace an existing secret, and `mcp --http` needs a short documented section
4. Reconcile README verification commands with the final CI checks; leave Hermes untouched if tool names and transports already match, and record that conclusion in progress

## Acceptance criteria

- Every command in CLI usage is dispatchable and every implemented command appears in usage
- No document claims a behavior contradicted by the current code
- All local Markdown links still resolve and the structural planning check passes

## Invariants

- Documentation is source of truth only when it agrees with the committed implementation.

## Review questions

- Are there other unenforced limits or first-run claims outside the listed files?

## Counterexample searches

- Compare each usage command with runtime dispatch and each first-run instruction with the corresponding CLI handler.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/cli -run TestUsage && python3 scripts/check-planning.py && git diff --check"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P3: five implemented commands are absent from usage, `init`/`serve` are credited with generating the owner secret, restore wording is stale, and a 512-code-point title limit is documented but unenforced.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
