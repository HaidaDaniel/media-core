# Domain model

Target design, not implemented. SQLite tables with foreign keys and ordinary history rows suffice; these histories are not an event-sourcing architecture.

## Identity and grouping

`MediaWork{id, revision, created_at, updated_at, archived_at?}` is a logical story grouping chosen by the user. Each `Edition{id, work_id, kind, language?, other_label?, revision, created_at, updated_at, archived_at?}` belongs to exactly one work. Creation of a work requires at least one title; it need not have an edition yet. Personal consumption always targets an edition. Same-work grouping does not imply adaptation, origin, identical language or shared progress.

Edition kinds: `web_novel`, `light_novel`, `book`, `manga`, `manhwa`, `manhua`, `anime`, `movie`, `tv`, `game`, `visual_novel`, `other`, `unknown`. `other` requires a bounded descriptive label. Translation is a relation between editions of a medium, not a medium enum: a Russian manga translation remains `manga`. A separately tracked language, release line, revision or adaptation is a distinct edition; extra URLs for the same line are references. Mirrors alone do not create editions.

Different story groupings can be related without merging. Users may add editions under a known work or create a separate work and confirm a cross-work link. The MVP has no destructive merge/reparent operation; incorrect identity is handled by revoking links and archiving a mistaken entry with a replacement reference. IDs remain stable and history stays readable.

## Titles and search

`Title{id, work_id? , edition_id?, text, language?, kind, preferred, provenance, created_at, retired_at?}` targets exactly one work or edition (separate work-title and edition-title tables are the initial physical representation). `kind`: `original`, `localized`, `romanized`, `alias`. BCP-47 language is nullable (unknown); romanization has its own title row. At most one active preferred display title per target, independent of original-language claims. Require a work display title; an edition falls back to its work title if none is supplied. Never fabricate a translation. Correct titles by retiring/replacing rows and retaining provenance.

Store original text plus a Go-computed search key: Unicode NFKC, full Unicode case fold, trim and collapse Unicode whitespace. Preserve diacritics, kana/kanji, punctuation and `ё`; no implicit transliteration or `ё`→`е` equivalence. Romaji and translated aliases must be supplied explicitly. Search normalized substring across active work and edition titles, returning distinct work cards with matching editions. Escape SQL LIKE `%`, `_` and escape characters as literals. SQLite's built-in ASCII `lower()`/NOCASE is insufficient. Start with scans at personal-library scale; FTS/vector search is unnecessary. Stable sort uses normalized preferred title then ID; filters operate on one qualifying edition, not unrelated editions in a work.

## Independent state dimensions

| Dimension | Values | Default / unknown |
| --- | --- | --- |
| Personal consumption | `not_started`, `in_progress`, `paused`, `completed`, `dropped` | NULL = unknown historical consumption; new manually added edition defaults to `not_started` |
| Personal intention | `none`, `interested`, `saved`, `planned`, `not_interested` | `none` means no recorded intention, not dislike |
| Edition publication | `unknown`, `ongoing`, `completed`, `hiatus`, `cancelled` | explicit `unknown`, with nullable source |

`planned` is stronger than `saved` (an actual plan versus a bookmark); they are mutually exclusive intention values. “Read later” and “watch later” are UI labels derived from `saved`/`planned` and medium, not extra states. `paused` is the only temporary consumption suspension; no duplicate `on_hold`. Not interested does not mean dropped; dropped records actual discontinued consumption.

All transitions between consumption values, including unknown, are allowed by explicit user action and recorded; rereading can move `completed` to `in_progress`, restarting can move `dropped` to `in_progress`. Setting the same current value is a no-op. A surprising transition can request a UI reason but is not a hidden business restriction. Status never automatically changes progress or intention; progress never automatically marks completed. All intention transitions are explicit; no automatic “completed means uninterested” rule.

`ConsumptionStatus` and `IntentionRecord` have edition, value, observed_at, recorded_at, actor, reason and optional supersedes_id. Publication observations have edition, value, observed_at, recorded_at, provenance and supersedes_id. Current state is the last non-retracted observation by `(observed_at, recorded_at, id)`; past-dated observations do not overwrite newer ones. Status source/reference is required for a known publication claim (a manual assertion is a source). Unknown or silence never means cancelled/completed. Conflicting source assertions stay in history; the selected latest observation and its source are shown, without implying consensus.

## History and corrections

Personal observations are normal rows written transactionally; current queries select active observations. All store UTC RFC3339 instants, actor (`user`, `agent`, `import` plus identity), source and recorded_at. observed_at defaults to now and cannot exceed server time by more than five minutes. Record a genuine change as a new observation. Correct a mistake via `supersedes_id` on a replacement for the same entity and dimension/unit, or explicit retraction with reason. Corrections keep the old row and its audit fields; no in-place history overwrite. One row can be superseded at most once. History exposes both effective and superseded rows. Idempotency request keys prevent retry-created observations. Work/edition `revision` increments on any child mutation; expected_revision prevents lost updates.

## ProgressRecord

`{id, edition_id, unit, position, total?, label?, observed_at, recorded_at, actor, supersedes_id?}`. Units: chapter, volume, episode, page, percent. Position is a non-negative decimal stored exactly (decimal string over JSON; fixed precision up to 3 fractional digits). Page is integral; percent is 0–100 with implicit total 100. Other known totals must be positive and at least the position; missing total is NULL, never zero or an estimate. Bound values to 1,000,000,000; label (e.g. “Extra 42.5”) supplements a numeric position, never substitutes parsing arbitrary text.

Progress belongs to one edition and one unit. The latest effective observation per unit is current; never sum chapters with volumes or progress across translations. Lower positions are allowed for rereads with an optional reason. Unknown current progress is absence of a record; not position zero. New observations may have an unknown total even if an older record had one; no silent carry-forward. Release counts never set total or personal progress without a separate explicit personal update. History and current queries survive reopen.

## Notes, ratings, intentions and tags

`Note` targets exactly one work or edition, with text, spoiler flag and optional quote/location. Immutable `NoteRevision` rows preserve text corrections and retractions. A revision points to the immediately preceding revision; expected_revision rejects concurrent overwrites. A note edit is visibly an edit; adding a later thought creates a new note. Bounded plain text, rendered escaped; raw HTML is not supported.

`Rating` is edition-scoped, integral 1–10, timestamped with actor and optional comment. Clearing it is an explicit null observation, not zero. Current rating follows the history rule; a change is distinct from correcting a mistaken rating. Work cards show the qualifying edition's rating, not an unexplained average.

`Tag` has a normalized unique name (NFKC/case fold/whitespace, 1–64 characters); tags can attach to a work or edition. Attach/detach is audited, idempotent and never alters consumption. `favorite` and `manga-translation-candidate` are simple tags; `read-later` may be a user tag but the saved/planned filter uses intention only. No workflow engine.

## ExternalReference and origin assertions

`ExternalReference{id, edition_id, provider, external_id?, url?, role, authoritative, language?, provenance, created_at, retired_at?}` requires external_id or absolute HTTP(S) URL; provider is a bounded nonempty namespace. Roles: `publication`, `metadata`, `translation`, `release_watcher`, `translation_project`, `content`. Authoritative publication status must be explicitly asserted, not inferred from a URL or metadata database. Store source claims with actor/source URI, observed_at and optional evidence text. Links are never fetched in the MVP; schemes such as javascript/file are rejected.

URL normalization: lowercase scheme/host, remove default port and fragment; preserve path case, trailing slash, query order/values and percent-encoding. Do not conflate http/https, mirrors or tracking query strings. Provider external IDs are case-sensitive opaque strings unless a future documented provider contract says otherwise. Unique active `(provider, external_id)` and normalized URL identify a reference globally; conflicts on another edition require identity resolution, never reassignment. Same edition + new URL adds a reference; it does not create an edition. Retire erroneous references with correction provenance; do not erase them.

`OriginClaim{id, edition_id, kind, state, provenance, replaces_id?}` has kind `original`, `anime_original`, or `unknown`; state `proposed`, `confirmed`, `revoked`. Unknown is the default absence of a confirmed origin. A confirmed source edge says only that an adaptation derives from this edition, not that this is the ultimate original. Trace accepted source edges and show confirmed origin assertions; there can be multiple sources. A source may be uncatalogued: keep a note/reference rather than invent an edition or an ultimate origin.

## MediaRelation graph

One relational table contains typed endpoints and foreign-key/check constraints. Both endpoints are of the same scope; mixed work→edition links are invalid.

| Scope | Relation | Direction / rule |
| --- | --- | --- |
| Edition | `adaptation_of` | adaptation → source; multiple sources allowed |
| Edition | `translation_of` | translation → immediate source; same medium, different language or explicit translator/version distinction |
| Edition | `alternative_version` | symmetric; same story but distinct line/version |
| Work | `same_underlying_work` | symmetric assertion, does not merge IDs |
| Work | `sequel_of` | later story → earlier story; prequel is the inverse display |
| Work | `side_story_of` | side story → main story |
| Work or edition | `related` | symmetric, same scope |

Each edge has state proposed/confirmed/revoked, provenance, created_at, confirmed_by/at and optional replaces_id. Agent-fetched claims default to proposed; they do not appear as confirmed facts or identity keys. Only explicit user-authorized confirmation creates a confirmed fact. MCP may submit confirmation when conveying an explicit user instruction; it must carry confirmation and actor context. Confidence is an optional hint, never a threshold for confirmation.

No self links. Symmetric pairs store IDs in canonical order. Duplicate active edges return the existing edge; uniqueness is over scope/type/endpoints for non-revoked states. Reject cycles in the combined confirmed `adaptation_of` + `translation_of` source graph, and separately in confirmed `sequel_of` and `side_story_of` graphs, inside the same write transaction as insertion/confirmation. Undirected related/version links can form cycles. Revoking a bad link preserves its history; a replacement is a new edge. Work grouping and same_underlying_work never create progress-sharing or automatic identity collapse. Related-media reads show proposals separately and can traverse both directions with bounded depth/visited IDs.

## Duplicate policy

1. Same idempotency key + same canonical request returns the original response; same key + different request returns `conflict`.
2. Matching active external ID or normalized URL returns existing edition (`created:false`, `reason:already_exists`) only when no contradictory explicit work/kind/language is supplied; contradictory identity returns `identity_conflict` without writes.
3. Exact normalized title is a **possible match**, not identity. Return candidates with evidence and a short-lived candidate token; do not create until the caller chooses existing, add-edition-under-work, or explicit create-distinct.
4. Different edition of an existing work requires explicit work_id and edition attributes. Same edition with a new URL uses add-reference.
5. Unresolved match stays unresolved. Recheck candidates and uniqueness inside the creation transaction; stale candidate decisions return `conflict`. No silent fuzzy merge or title uniqueness constraint. Similar-title hints may be lexical later, but never authoritative.

## Release and import records

`ImportRecord` preserves immutable event_id, schema_version, raw JSON and typed source IDs. Linking lives in separate audited `ImportBinding` rows so the raw import stays immutable. `ReleaseRecord` is an immutable edition-scoped projection keyed by event and release identity; revoked/replaced bindings hide obsolete projections without deleting them. Pending imports have no current binding. Unique event_id ensures replay is a no-op; different payload for the same ID is `identity_conflict`. Counts, source time and detection time are separate from personal progress. See [integration contract](integration-contract.md).

## Future boundaries

`ContentReference` is a pointer; `ContentAsset` would hold hash/location/type/language/provenance; `ContentProvider` would resolve it; `DeliveryTarget` would describe a destination. No unused asset tables are needed now. Translator Agent/manga translation projects use reference roles and translation relations with language/provenance. OCR, acquisition, rendering, inpainting, model execution and delivery remain outside the MVP.
