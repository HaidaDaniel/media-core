# Product requirements and acceptance scenarios

Status: target MVP, not tests already executed. One person's offline library; Web UI and Hermes/MCP both required. Scope and canonical rules: [scope](../scope.md), [domain](../domain-model.md), [application contract](../application-contract.md). Requirement ownership is in the [coverage matrix](../implementation-plan.md).

## Requirements

| ID | Observable requirement |
| --- | --- |
| R01 | Reproducible Go executable, build identity, explicit state/config and stable JSON/errors |
| R02 | Persistent works/editions and manual title/URL/ID entry, without external metadata |
| R03 | Multiple sourced titles; Unicode alias search, filters, sorting and counts |
| R04 | Independent consumption, intention and publication state with explicit unknowns |
| R05 | Edition-specific progress with correct units, unknown totals, restart durability and corrections |
| R06 | Notes/revisions, rating history, tags and translation candidates |
| R07 | Multiple publication/metadata/translation references and provenance |
| R08 | Explicit adaptation/translation/version/story graph, origin uncertainty, confirmation/correction and cycle protection |
| R09 | Exact identity dedupe, candidate decision and safe replay; no fuzzy merge |
| R10 | First-class responsive Web UI with library/add/details/relations/activity and complete failure states |
| R11 | First-class typed Hermes/MCP tools and local/SSH connection path |
| R12 | Shared app transactions, DTOs, errors, revisions and CLI/API/MCP parity |
| R13 | Loopback defaults, owner sessions, Origin/CSRF, explicit authenticated TLS remote mode |
| R14 | Consistent backup, isolated restore, readable export and read-only doctor |
| R15 | Safe concurrent CLI/MCP/HTTP, migration integrity and maintenance exclusion |
| R16 | Version 1.0 idempotent Release Watcher import, pending binding and separate publication history |
| R17 | Embedded UI, one-binary Linux x86-64/ARM64 build and documented local deployment |
| R18 | No AI/provider prerequisite; translation and metadata extension boundary without execution |

## Acceptance fixtures and interfaces

Scenarios A–D must run through **both** Web UI and MCP against separate temporary databases and compare shared views with IDs/times normalized. Include CLI shared-service smoke tests. E runs CLI import plus HTTP/MCP projection/binding; F uses CLI maintenance then checks both interfaces. These are future checks; application tests do not exist yet. No real external source, paid inference or deployment is needed.

### A — Watched anime and unread manga source

Create an anime and Japanese/Russian aliases; explicitly mark it completed and rate 8/10. Add manga under the same work (or a distinct work with an explicit link), confirm anime adaptation_of manga with source provenance, and keep manga not_started + saved. Add the smartphone-reading deferral note, publication URL and manga-translation-candidate tag. Search by either title; navigate both directions and see editions, rating, notes and status history. A guessed origin stays proposed until confirmed. Dropped/not_interested must not appear as a side effect. Try self/cyclic source links and verify rejection with no partial writes. Owners: 0020, 0033, 0041, 0045.

### B — Reading progress and correction

Add a web novel, record chapter 42 then 48, reopen processes and verify latest 48 plus both historical rows through Web/MCP. Leave total unknown. Record a lower reread position without deleting history; correct a mistaken row using supersedes_id and verify both versions remain visible. Reject negative positions and page fractions. Changing progress does not implicitly set completed. Owners: 0015, 0027, 0034, 0040, 0046.

### C — Existing work and another edition

Add original title plus explicit Russian/English/romaji aliases. Search by any alias. Re-add its reference with the same identity and get existing-record; same key/different payload fails. Try title-only duplicate and see candidates before any creation; choose existing or create distinct explicitly. Attach another URL to the edition without cloning it. Add a distinct adaptation under an explicit work choice; a similar title does not merge it. Exercise stale candidate token and concurrent exact additions. Owners: 0012, 0013, 0032, 0039, 0044, 0045.

### D — Negative impression is retained

Add an anime, record in_progress then dropped, a negative note and 2/10 rating. Change the note with an explicit revision and preserve its old text. Find the entry later via alias, dropped filter and activity. Dropping does not erase source links, history or imply publication cancelled. A saved manga remains independent. Owners: 0014, 0017, 0018, 0035, 0036, 0041, 0046.

### E — Release Watcher is separate from reading

Import a pinned schema 1.0 event, leave it pending, explicitly bind the known edition, then replay it and verify one projection. Reusing the ID with a conflicting payload fails without replacement. Verify unknown additive fields/type, unsupported major, count-only event, metadata/status change, source release removal and RSS window absence. A malformed item rolls back only that item; invalid envelope/major writes nothing. Explicitly correct a binding while retaining raw payload and old binding history. Chapter read and intention stay unchanged; UI/MCP show labeled releases. Owners: 0042, 0043.

### F — Backup and restore

Create works/editions, aliases, relations, progress, state, notes, ratings, references and import history using existing use cases. Take a consistent backup while another process updates, then restore to a fresh isolated state directory and compare normalized application-visible state, relationships and retained history through Web/MCP. Reject traversal archive entries, corrupt hash, oversized archive and newer schema before modifying destination. Active DB handle blocks restore; interrupted replacement leaves the old or new valid DB recoverable. Owner secret/sessions are not exported and destination security survives restore. Owners: 0022, 0023, 0044, 0047. Task 0023 proves the M1 subset; task 0047 adds the complete M5 data fixture after all capabilities exist.

### G — Security and optional integrations

Require login on loopback, reject missing CSRF/hostile Origin/Host and expired session. Reject non-loopback configuration without all explicit TLS/auth prerequisites. Confirm manual entry, restart, lookup and progress without Internet/LLM/provider credentials. Build artifact serves UI/API and runs CLI/MCP with Node absent. Owners: 0024, 0030, 0047, 0048.
