-- 0008: plain-text notes with immutable revision chains.
-- A note targets exactly one work or edition. Its revisions are append-only
-- rows: each revision points to the immediately preceding revision, and
-- text corrections and retractions are preserved, so the original text and
-- its audit survive every edit. A retracted note's latest revision is a
-- retraction marker; such a note is absent from views, never deleted.
-- Note text is bounded plain text (the byte bound is enforced at the store
-- boundary; the character bound below is a weaker defense in depth).

CREATE TABLE notes (
	id         TEXT PRIMARY KEY,
	work_id    TEXT REFERENCES works(id),
	edition_id TEXT REFERENCES editions(id),
	revision   INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	CHECK ((work_id IS NULL) != (edition_id IS NULL))
);

CREATE INDEX notes_work_idx ON notes (work_id);

CREATE INDEX notes_edition_idx ON notes (edition_id);

CREATE TABLE note_revisions (
	id          TEXT PRIMARY KEY,
	note_id     TEXT NOT NULL REFERENCES notes(id),
	revision    INTEGER NOT NULL,
	prev_id     TEXT REFERENCES note_revisions(id),
	retracted   INTEGER NOT NULL DEFAULT 0,
	text        TEXT NOT NULL CHECK (length(text) >= 1 AND length(text) <= 16384),
	spoiler     INTEGER NOT NULL DEFAULT 0,
	quote       TEXT,
	location    TEXT,
	reason      TEXT,
	recorded_at TEXT NOT NULL,
	actor       TEXT NOT NULL
);

CREATE UNIQUE INDEX note_revisions_note_revision_uq
	ON note_revisions (note_id, revision);

CREATE INDEX note_revisions_note_idx ON note_revisions (note_id);
