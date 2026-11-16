# Task 0072 - Verify staged assets recursively in the release manifest gate

State: todo
Owner: none
Mode: mvp
Lane: packaging
Scope: packaging
Risk: medium
Parallel: false
Depends on: 0066
Tags: planned,m5,final-audit,p3

## Goal

Make the web-asset manifest verifier cover the full staged tree, including nested directories, and fail closed when a mandatory check is skipped.

## Context files

- AGENTS.md
- docs/task-system.md
- scripts/build-release.sh
- tests/packaging/packaging_test.go
- docs/engineering/deployment.md

## Files allowed to edit

- tests/packaging/packaging_test.go
- docs/engineering/deployment.md
- docs/progress.md
- .tasks/0072-verify-staged-assets-recursively-in-the-release-manifest-gate.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- release/**
- internal/webassets/dist/**
- web/dist/**
- scripts/build-release.sh

## Steps

1. Reproduce the gap: add an unregistered file under `assets/` and confirm the current verifier still exits 0
2. Replace the top-level `os.ReadDir` scan with a recursive `filepath.WalkDir` comparison over all regular files, excluding only the root `manifest.json`
3. Reject duplicates, absolute paths, `..`/backslash traversal, symlinks, non-regular entries, missing files, size/hash mismatches and malformed manifests; require the manifest itself to be a regular file
4. Fail when the release-gate flag is set but the staging directory was not provided, so the check cannot be silently skipped
5. Add table-driven negative tests for extra nested file, missing file, changed hash, symlink and invalid path, plus a positive recursive case

## Acceptance criteria

- Every real file except root `manifest.json` appears exactly once in the manifest and matches size and SHA-256
- No manifest entry is a symlink, directory, absolute path or traversal path
- An extra nested file, a missing file, a changed hash, a symlink and an invalid path each fail the verifier
- A malformed manifest and a gate run without a staging directory both fail
- Existing release build and artifact gate remain green

## Invariants

- The manifest describes the complete staged file tree; a skipped check is a failed gate.

## Review questions

- Could a nested symlink to a file outside staging be hashed as a regular file?

## Counterexample searches

- Nested extra file, nested missing file, nested symlink, self-entry, duplicate, traversal name, corrupt JSON, gate without stage dir.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/packaging -run 'TestPackageReleaseArtifacts|TestPackageReleaseArtifactsManifest' -count=1"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P3: the verifier skipped directory entries, so unregistered files under assets/ were not detected.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
