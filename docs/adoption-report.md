# Adoption Report

Agentic Project Kit was added in adopt mode.

## Guardrails

- Existing files were not overwritten.
- Application source files were not rewritten.
- Follow-up work should happen through task files.

## Observed Repository

- Name: media-core
- Stack: Node.js, pnpm
- Application runtime: unknown
- Repository tooling: APK, Node.js, pnpm

## Pre-adoption Gaps

- Missing kit docs: 0
- Missing agent exports: 0
- Agentic config present: yes
- Existing task files: 2

## Compatibility

- Contract: gated
- Config schema: gated v2
- Task contract: mixed (legacy=1, gated=1, unknown=0)
- Config schema-marker migration required: no
- Task-contract repair required: no known incompatibilities (see coverage)
- Task parseability: parseable=2; incompatible=0; unreadable=0; not-inspected=0; coverage=complete. Verification heading families alone do not prove parseability.
- Task inspection limits: 4096 entries/directory; 1024 files; 262144 bytes/file; 32 issues/file; 512 characters/issue (long text ends with …).

## Missing Kit Docs

- none

## Missing Agent Exports

- none
