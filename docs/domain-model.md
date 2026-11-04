# Domain model

Status: accepted design; not implemented. This document defines the minimum concepts and invariants for the future Go + SQLite implementation.

## Library concepts

```text
MediaWork 1---* Edition 1---* ProgressRecord
    |              |
    |              *---* ExternalReference
    |              |
    *---* MediaRelation (explicit)
                   |
                   *---* ReleaseRecord (linked)
                              ^
                              |
                        ImportRecord (staging, edition nullable)
```

### MediaWork

A logical story or title, for example "The Dreaming Boy Is a Realist". It is the user's grouping container, not an automatic merge key.

| Field | Meaning |
| --- | --- |
| `id` | internal id |
| `original_title` | title in the original language |
| `title_ru`, `title_en` | user/agent supplied localized titles, nullable |
| `created_at`, `updated_at` | local timestamps |

A localized title is stored once and reused. If absent, the CLI shows the original; it never fabricates a translation.

### Edition

One concrete edition/line: WN, LN, book, manga, film, series, anime, translation, other. Editions belong to a MediaWork.

| Field | Meaning | Origin |
| --- | --- | --- |
| `id` | internal id | computed |
| `work_id` | owning MediaWork | relation |
| `kind` | `wn`, `ln`, `book`, `manga`, `film`, `series`, `anime`, `translation`, `other`, `unknown` | user fact |
| `title` | edition title, nullable | user/source fact |
| `language` | BCP-47 tag or unknown | user/source fact |
| `original_url` | canonical source URL, nullable | user/source fact |
| `external_ref` | import reference such as Release Watcher `publication_id` / `external_work_reference`, nullable | integration fact |
| `status` | publication status (`ongoing`, `completed`, `unknown`) | source fact when proven |
| `status_source` | provenance of status | provenance |
| `first_release_at`, `last_release_at` | known release range, nullable | computed from releases |
| `created_at`, `updated_at` | local timestamps | computed |

WN and LN are separate Editions unless the user explicitly relates them. A translation is an Edition with its own language and provenance. A mirror is not a new story.

### ExternalReference

A user or integration reference attached to an Edition/Release: provider, URL, external id, kind. Multiple references may point at the same edition. References do not merge editions automatically.

### ProgressRecord

One progress observation, not a mutable counter without history.

| Field | Meaning |
| --- | --- |
| `id` | internal id |
| `edition_id` | tracked edition |
| `unit` | `chapter`, `volume`, `episode`, `page`, `percent` |
| `position` | numeric/text position, nullable |
| `total` | total units if known, nullable |
| `occurred_at` | when the user reached it |
| `source` | `manual`, `agent`, `import` |
| `note` | optional bounded comment |

The current progress is derived from the latest record. History is preserved. Two positions for different editions are never summed.

### ConsumptionStatus

A user-owned state: `planned`, `in_progress`, `paused`, `completed`, `dropped`, `on_hold`, `unknown`, with timestamps and optional reason. Publication completion from a source never forces the user's status.

### Rating and Note

- Rating: bounded user rating (for example 1–10) with a scale definition and timestamp; changes are new records or explicitly audited updates.
- Note: free text with created/updated timestamps, optional quote/page reference, optional spoiler flag.

### MediaRelation

An explicit, directed or symmetric relation between MediaWorks or Editions: `same_story`, `translation_of`, `adaptation_of`, `side_story`, `prequel`, `sequel`, `related`, `alternative`. It has provenance (`manual`, `agent`) and confidence/unknown. Relations are created deliberately and never inferred from title similarity alone.

### ImportRecord (staging)

An immutable imported Release Watcher event before it is linked to an edition: event id, event type, publication id, external work reference, raw payload, schema version, detection time, and a nullable `edition_id`. Unknown additive fields are preserved. An event with no explicit edition link stays `pending`; it is never auto-attached by title. Linking is an explicit user/agent action.

### ReleaseRecord (imported)

A release projection created after an ImportRecord is linked to an edition: event id, type, edition/publication reference, external release ids, numbers, titles, URLs, source times, detection time, schema version. It is immutable after creation; corrections are new events or explicit annotations. The original ImportRecord remains the provenance source.

## Content delivery concepts (future, concept only)

These concepts are documented so the MVP schema does not block them. They are not implemented and perform no network calls.

### ContentReference

A pointer to external content: provider, URL/external id, media type, language, edition/work relation, optional version. It is not the content itself.

### ContentAsset

A stored or deliverable content object: hash, byte size, media type, language, provenance (origin reference), rights/license note, created/verified timestamps, optional storage location. The same content may have several representations.

### ContentProvider

A source that can resolve a ContentReference or acquire/produce a ContentAsset. Providers are explicit, optional integrations with timeout/error contracts. Media Core does not crawl arbitrary sites as a core feature.

### DeliveryTarget

A destination such as an e-reader, a local folder or another library. It declares capabilities (formats, languages, size limits) and a delivery state. Delivery is a future module, separate from library truth.

### Original/translation link

A translation asset links to its original asset/reference through a `translation_of` relation, with translator/harness provenance, source language, target language and version. A translation is never presented as the original. If a translation is unavailable, the UI shows the original; it does not invent one.

### External translation harness

Media Core may record that an external harness produced a translated asset. It does not embed an LLM, scheduling or translation execution in the core MVP. The harness is a provider/provenance boundary.

## Invariants

1. One Edition belongs to one MediaWork; a MediaWork may have several Editions.
2. No automatic merging by title, cover or fuzzy string.
3. Progress, status, rating and notes are user-owned and edition-scoped.
4. Release Watcher events are deduplicated by `event_id`; importing twice is a no-op. Pending ImportRecords may have no edition; a ReleaseRecord requires one.
5. A publication line's release counts are never summed with another edition's counts.
6. Facts, computed values and unknowns are stored distinctly; unknown stays NULL/unknown.
7. Content assets and delivery are future concepts; library truth does not depend on them.
8. Deleting or pausing a Release Watcher watch does not delete Media Core history.
