-- 0007: personal intention observations and simple audited tag assignments.
-- Intentions are append-only edition-scoped history rows like consumption:
-- the current intention is the last non-superseded row; the explicit value
-- 'none' records a retraction to no intention. Tags are normalized unique
-- names attached to exactly one work or edition; a detach keeps the
-- assignment row as audit with its detach instant and actor.

CREATE TABLE intention_observations (
	id            TEXT PRIMARY KEY,
	edition_id    TEXT NOT NULL REFERENCES editions(id),
	value         TEXT NOT NULL CHECK (value IN ('none', 'interested', 'saved', 'planned', 'not_interested')),
	observed_at   TEXT NOT NULL,
	recorded_at   TEXT NOT NULL,
	actor         TEXT NOT NULL,
	reason        TEXT,
	supersedes_id TEXT REFERENCES intention_observations(id)
);

CREATE INDEX intention_observations_edition_idx ON intention_observations (edition_id);

CREATE UNIQUE INDEX intention_observations_supersede_uq
	ON intention_observations (supersedes_id) WHERE supersedes_id IS NOT NULL;

CREATE TABLE tags (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL CHECK (length(name) >= 1 AND length(name) <= 64),
	created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX tags_name_uq ON tags (name);

CREATE TABLE tag_assignments (
	id           TEXT PRIMARY KEY,
	tag_id       TEXT NOT NULL REFERENCES tags(id),
	work_id      TEXT REFERENCES works(id),
	edition_id   TEXT REFERENCES editions(id),
	attached_at  TEXT NOT NULL,
	actor        TEXT NOT NULL,
	detached_at  TEXT,
	detach_actor TEXT,
	CHECK ((work_id IS NULL) != (edition_id IS NULL))
);

CREATE INDEX tag_assignments_edition_idx ON tag_assignments (edition_id);

CREATE INDEX tag_assignments_work_idx ON tag_assignments (work_id);

CREATE UNIQUE INDEX tag_assignments_edition_active_uq
	ON tag_assignments (edition_id, tag_id) WHERE edition_id IS NOT NULL AND detached_at IS NULL;

CREATE UNIQUE INDEX tag_assignments_work_active_uq
	ON tag_assignments (work_id, tag_id) WHERE work_id IS NOT NULL AND detached_at IS NULL;
