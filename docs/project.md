# Media Core project

Status: accepted documentation. No runtime implementation exists in this repository yet.

## Mission

Media Core is a personal, local-first library for serialized and non-serialized media. It answers:

- What did I read or watch?
- Where did I stop, and what is the current progress?
- What is its status in my life (planned, in progress, paused, completed, dropped)?
- What did I think of it (rating, notes, quotes)?
- Which editions, translations and adaptations are the same story, and which are separate?
- What was published recently in a work I follow?

It is not a content marketplace, streaming service, social network or recommendation engine.

## Primary user and workflow

One person manages their own library on a home Linux machine, optionally driven by an agent.

```text
media-core add --title "..." --type wn
media-core progress <id> --chapter 42
media-core status <id> completed --rating 8
media-core note <id> "The ending ..."
media-core list --json
```

Release Watcher can supply new-release events. Media Core stores them as release history and can link them to the user's edition/progress, but remains usable without Release Watcher.

## Product principles

1. Local-first: all core reads/writes work offline. No mandatory cloud, telemetry or hosted identity.
2. Agent-first, not AI-dependent: every operation is a deterministic CLI use case with stable JSON. Agents can drive it without special server access.
3. One binary, SQLite, on-demand execution; no always-running daemon or web server in the MVP.
4. Durable history and readable export/backup are core, not afterthoughts.
5. Facts, computed values and unknowns are distinct. Missing data stays missing.
6. Editions are explicit. WN, LN, manga, anime and translations are separate publication/edition lines unless the user links them.
7. Release Watcher events are immutable inputs. Import is idempotent by `event_id`; events are never rewritten to fit a title match.
8. Content delivery, translation execution and provider crawling are future modules, not MVP dependencies.

## Status

| Area | State |
| --- | --- |
| Product/scope/architecture/domain/roadmap docs | implemented |
| Integration contract with Release Watcher | documented |
| Go module, SQLite schema, CLI | planned |
| MCP stdio | planned after CLI |
| ContentReference/ContentAsset/ContentProvider/DeliveryTarget | concept only |
| Translation harness integration | concept only |
| Content delivery | future module |

See [scope](scope.md), [architecture](architecture.md), [domain model](domain-model.md), [roadmap](roadmap.md), [integration contract](integration-contract.md) and [decisions](decisions.md).
