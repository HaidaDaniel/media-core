# Task 0081 - Upgrade AgenticProjectKit to v0.6.2

State: doing
Owner: opencode-apk-upgrade-20261128
Mode: maintenance
Lane: maintenance
Scope: tooling,dependencies,docs,workflow
Risk: medium
Parallel: false
Depends on: none
Tags: tooling,apk,upgrade,dependencies,workflow

## Goal

Upgrade the repository development tooling dependency AgenticProjectKit from 0.6.0 to the stable v0.6.2 tag without changing Media Core runtime behavior, and validate lifecycle, generated instructions and multi-worktree safety.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/decisions.md
- docs/progress.md
- docs/engineering/deployment.md
- package.json
- pnpm-lock.yaml

## Files allowed to edit

- package.json
- pnpm-lock.yaml
- AGENTS.md
- CLAUDE.md
- GEMINI.md
- .gitignore
- docs/progress.md
- docs/decisions.md
- docs/engineering/apk-tooling.md
- .tasks/0081-upgrade-agenticprojectkit-to-v062.md

## Files forbidden to edit

- cmd/**
- internal/**
- web/**
- deploy/**
- scripts/**
- go.mod
- go.sum
- .github/workflows/**
- release/**
- tmp/**

## Steps

1. Capture pre-upgrade APK diagnostics (version, doctor, sync, lint --json, audit, status, status --detail, workers --json, quality detect) with exit codes and timings.
2. Create the separate APK upgrade task, claim it, and record the baseline before any implementation change.
3. Pin agentic-project-kit to tag v0.6.2 with pnpm add -D and verify the peeled commit b4df405205e6ea66ae88c1093ea9bd6ae28080a0, the lockfile resolution and the repository-local executable.
4. Run read-only apk sync, inspect the proposed generated-instruction diff, then apply sync --write only if required while preserving project-specific guidance.
5. Re-run APK diagnostics and compare with baseline; add canonical ignore entries only if doctor requires them.
6. Re-validate historical tasks 0079 and 0080 gates and provenance; do not reopen them or rewrite their evidence.
7. Run Go build, vet and tests plus an isolated fixture lifecycle and multi-worktree safety check.
8. Perform independent review with a different registered identity, gate the task, commit, integrate into main and verify remote SHA and CI.
9. Write the upgrade verification report under docs/engineering/apk-tooling.md and update docs/progress.md.

## Acceptance criteria

- APK resolves to 0.6.2 and pnpm-lock records the tag commit b4df405205e6ea66ae88c1093ea9bd6ae28080a0.
- No runtime files, Go modules, Docker, systemd, deployment scripts or release artifacts change.
- Generated instructions remain in sync and project-specific instructions are preserved.
- apk lint, doctor, sync check and status pass and diagnostics are compared before and after.
- Historical 0079/0080 verdicts are unchanged and documented without editing task states or evidence.
- Go build, vet and tests pass and the isolated fixture exercises claim, verify, review, gate and done plus a linked worktree.
- Independent review by a different registered identity passes and the task gate passes.
- The upgrade commit is integrated into main and origin/main matches it; CI status is checked.

## Verification

- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"doctor"}`
- `{"id":"apk-sync","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"sync-check"}`
- `{"id":"apk-status","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"status"}`
- `{"id":"go-build","type":"automated","required":true,"environment":"local","profile":"report","command":"go build ./..."}`
- `{"id":"go-vet","type":"automated","required":true,"environment":"local","profile":"report","command":"go vet ./..."}`
- `{"id":"go-tests","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./..."}`

## Documentation updates

- docs/engineering/apk-tooling.md: upgrade report with baseline and final diagnostics, commit, backups and findings
- docs/progress.md: record the tooling upgrade and validation outcome

## Notes

- Base the upgrade on main in an isolated worktree; do not merge feature/dark-ui-covers-0079 or publish runtime commits.
- Do not modify node_modules or APK sources; report APK defects instead.
- Keep backups outside the repository.
- The v0.6.2 upstream release index still marks v0.6.1 as last validated; record this observation and do not block on it.
- Use task id 0081 to avoid collision with 0079/0080 on the feature branch.
