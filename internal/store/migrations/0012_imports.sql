-- 0012: immutable release-watcher import events and explicit bindings.
--
-- import_events holds one immutable row per producer event, keyed by the
-- producer's stable event_id. Replaying the same event_id with a
-- semantically identical payload is a no-op; the same event_id with a
-- different payload is an identity conflict and is never an overwrite. The
-- raw payload and the canonical payload are preserved so later migrations
-- can re-project fields without asking the producer to resend, and unknown
-- additive fields or unknown event types stay safe in the raw payload.
-- Events are pending until an explicit binding links them to an edition;
-- nothing is ever inferred from a title.
--
-- import_bindings are separate audited rows that explicitly associate a
-- producer namespace + publication_id with one edition. The immutable event
-- payload is never rewritten; a binding change adds a new row.
CREATE TABLE import_events (
	id                TEXT PRIMARY KEY,
	event_id          TEXT NOT NULL UNIQUE,
	schema_version    TEXT NOT NULL,
	event_type        TEXT NOT NULL,
	raw_payload       TEXT NOT NULL,
	canonical_payload TEXT NOT NULL,
	namespace         TEXT NOT NULL,
	publication_id    TEXT,
	external_work_ref TEXT,
	release_id        TEXT,
	release_number    TEXT,
	release_title     TEXT,
	release_url       TEXT,
	current_count     INTEGER,
	count_delta       INTEGER,
	source_time       TEXT,
	detection_time    TEXT,
	work_hint         TEXT,
	edition_hint      TEXT,
	created_at        TEXT NOT NULL
);
CREATE INDEX import_events_type_idx ON import_events (event_type);
CREATE INDEX import_events_publication_idx ON import_events (namespace, publication_id);
CREATE INDEX import_events_release_idx ON import_events (namespace, publication_id, release_id);

CREATE TABLE import_bindings (
	id             TEXT PRIMARY KEY,
	namespace      TEXT NOT NULL,
	publication_id TEXT NOT NULL,
	edition_id     TEXT NOT NULL REFERENCES editions(id),
	state          TEXT NOT NULL CHECK (state IN ('active','revoked')),
	created_by     TEXT NOT NULL,
	created_at     TEXT NOT NULL,
	revoked_by     TEXT,
	revoked_at     TEXT,
	replaces_id    TEXT
);
CREATE UNIQUE INDEX import_bindings_active_idx
	ON import_bindings (namespace, publication_id) WHERE state = 'active';
CREATE INDEX import_bindings_edition_idx ON import_bindings (edition_id);
