# Task 0051 - Restore generated agent instruction freshness

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: planning
Scope: docs,planning
Risk: low
Parallel: false
Depends on: 0063
Tags: planned,m5,review-fix

## Goal

Regenerate stale exported agent instructions so `apk lint` and `apk sync` no longer report drift.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/context-system.md
- .agentic/config.json

## Files allowed to edit

- AGENTS.md
- CLAUDE.md
- GEMINI.md
- docs/progress.md
- .tasks/0051-restore-generated-agent-instruction-freshness.md

## Files forbidden to edit

- .agentic/config.json

## Steps

1. Inspect the reported drift with `pnpm exec apk sync` before writing
2. Regenerate the exported files through `pnpm exec apk sync --write` only; never hand-edit generated content
3. Re-run lint and sync and inspect the resulting diff for unrelated rewrites

## Acceptance criteria

- `pnpm exec apk lint` no longer reports a generated-file-stale finding
- `pnpm exec apk sync` reports all generated files current
- The generated diff contains no hand-authored policy content

## Invariants

- Generated agent files are outputs of neutral policy content, not an independent source of truth.

## Review questions

- Did regeneration change only generated files, or did it silently rewrite a tracked policy source?

## Counterexample searches

- Re-run `apk sync` after a no-op change and confirm it stays clean.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm exec apk lint && pnpm exec apk sync"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of stale generated instructions.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
