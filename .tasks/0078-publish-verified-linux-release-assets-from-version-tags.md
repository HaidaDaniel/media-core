# Task 0078 - Publish verified Linux release assets from version tags

State: doing
Owner: opencode-release-20261123
Mode: mvp
Lane: implementation
Type: feature
Scope: ci,release,docs
Risk: high
Parallel: false
Depends on: none
Tags: ci

## Goal

Publish Linux amd64/arm64 binaries and SHA256SUMS as public GitHub Release assets only after the existing release gate passes on a v* tag, using GITHUB_TOKEN with contents: write, with anonymous HTTPS verification and operator documentation.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/scope.md
- docs/decisions.md
- docs/engineering/deployment.md
- .github/workflows/release-gate.yml
- .github/workflows/ci.yml
- scripts/build-release.sh
- internal/buildinfo/buildinfo.go

## Files allowed to edit

- .github/workflows/release-gate.yml
- docs/engineering/deployment.md
- docs/progress.md
- docs/implementation-plan.md
- README.md
- .tasks/0078-publish-verified-linux-release-assets-from-version-tags.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- .github/workflows/ci.yml
- scripts/build-release.sh
- deploy/*

## Steps

1. Inspect the current release gate, build identity and packaging tests.
2. Keep the existing gate job unchanged in behavior; add a separate publish job gated on gate success and a pushed v* tag.
3. Resolve version, tag and commit from one checkout; verify both binaries report that identity before publication.
4. Generate SHA256SUMS from the exact built binaries and include it in the Actions artifact.
5. Refuse to publish when a release for the tag already exists.
6. Verify each published asset and the release page anonymously over HTTPS and check downloaded bytes against SHA256SUMS.
7. Document download, checksum verification and NAS deployment from a published release.
8. Add the plan inventory and progress evidence; run focused checks and independent review.

## Acceptance criteria

- Existing verification steps and Actions Artifacts are preserved.
- Publishing happens only after the gate job succeeds on a push of a v* tag.
- workflow_dispatch builds and uploads artifacts without creating a release.
- The publish job uses only GITHUB_TOKEN with contents: write; no PAT is referenced.
- Version, commit SHA and artifacts come from one checkout and are verified against the binaries.
- An existing release for the tag is never overwritten.
- No binary is uploaded when any check fails.
- Published assets are anonymously retrievable over HTTPS and match SHA256SUMS.
- Documentation contains download, SHA256SUMS verification and NAS deployment commands.

## Correctness assumptions

- A pushed tag points at one immutable commit; the workflow checks out that exact commit by SHA.
- GitHub Release assets are immutable by name in this workflow because publication refuses to reuse an existing tag.
- Anonymous HTTPS checks run only after the release and assets exist.

## Invariants

- No NAS deployment, NAS configuration change, tag, push or real release from local work.
- No personal access token or extra permission scope.
- Existing checks, security headers and release build behavior are unchanged.
- No overwrite of an existing release.

## Required evidence

- Focused workflow contract checks, the planning/link check, APK lint and doctor, and an independent review of the committed candidate.

## Review questions

- Can workflow_dispatch or a non-version ref publish anything?
- Can the publish job get contents: write before gate success or on a non-tag ref?
- Can a mismatched commit, tag, version string or binary be published?
- Can a failed check still publish?
- Are assets verified without any Authorization header, and do downloaded bytes match SHA256SUMS?

## Counterexample searches

- Set event_name to workflow_dispatch with a v-like ref.
- Make a gate step fail and confirm no publish job runs.
- Pre-create a release for the tag and confirm publication refuses to overwrite it.
- Change MEDIA_CORE_VERSION away from the tag and confirm identity verification fails.
- Swap an artifact after hashing and confirm the checksum verification fails.
- Look for a token in the anonymous HTTPS check or a PAT secret in the workflow.

## Verification

- `{"id":"yaml-parse","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 -c \"import yaml;yaml.safe_load(open('.github/workflows/release-gate.yml'))\""}`
- `{"id":"publish-perms","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 -c \"import yaml;w=yaml.safe_load(open('.github/workflows/release-gate.yml'));j=w['jobs'];assert j['gate']['permissions']['contents']=='read' and j['publish']['permissions']['contents']=='write'\""}`
- `{"id":"publish-trigger","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 -c \"import yaml;w=yaml.safe_load(open('.github/workflows/release-gate.yml'));p=w['jobs']['publish'];assert p['needs']=='gate' and 'refs/tags/v' in p['if'] and 'event_name' in p['if']\""}`
- `{"id":"planning-links","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"doctor"}`

## Documentation updates

- Update docs/engineering/deployment.md with release download, SHA256SUMS verification and NAS install from a published release.
- Update README.md release gate description.
- Record lifecycle in docs/progress.md.

## Notes

- Milestone: M5. Public release publication. No NAS deploy, no host configuration change and no real tag, push or release is authorized in this task; the final handoff only provides tag commands for the operator.
- The real anonymous HTTPS and GitHub API behavior can only be observed after a tag is pushed; this task provides the workflow check for that future run, not a claim that it has already executed.
