-- 0003: editions, edition titles and sourced publication observations.
--
-- An edition is one concrete medium line of a work (a translation keeps its
-- medium: a Russian manga translation is kind manga). It belongs to exactly
-- one work; no shared progress is inferred between editions. Publication
-- observations are sourced history rows: current state is the last
-- non-superseded observation by (observed_at, recorded_at, id); corrections
-- add a superseding row instead of overwriting, and a row is superseded at
-- most once.
CREATE TABLE editions (
	id          TEXT PRIMARY KEY,
	work_id     TEXT NOT NULL REFERENCES works(id),
	kind        TEXT NOT NULL CHECK (kind IN ('web_novel', 'light_novel', 'book', 'manga', 'manhwa', 'manhua', 'anime', 'movie', 'tv', 'game', 'visual_novel', 'other', 'unknown')),
	language    TEXT,
	other_label TEXT,
	revision    INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	archived_at TEXT,
	CHECK ((kind = 'other') = (other_label IS NOT NULL)),
	CHECK (other_label IS NULL OR LENGTH(other_label) BETWEEN 1 AND 64)
);

CREATE INDEX editions_work_idx ON editions (work_id);

CREATE TABLE edition_titles (
	id         TEXT PRIMARY KEY,
	edition_id TEXT NOT NULL REFERENCES editions(id),
	seq        INTEGER NOT NULL CHECK (seq >= 1),
	text       TEXT NOT NULL,
	search_key TEXT NOT NULL,
	language   TEXT,
	kind       TEXT NOT NULL CHECK (kind IN ('original', 'localized', 'romanized', 'alias')),
	preferred  INTEGER NOT NULL DEFAULT 0 CHECK (preferred IN (0, 1)),
	provenance TEXT NOT NULL,
	created_at TEXT NOT NULL,
	retired_at TEXT
);

CREATE UNIQUE INDEX edition_titles_edition_seq ON edition_titles (edition_id, seq);

-- Active titles of one edition, in creation order.
CREATE INDEX edition_titles_edition_active_idx ON edition_titles (edition_id) WHERE retired_at IS NULL;

-- At most one active preferred display title per edition; an edition
-- without active titles falls back to its work title.
CREATE UNIQUE INDEX edition_titles_one_preferred ON edition_titles (edition_id) WHERE preferred = 1 AND retired_at IS NULL;

CREATE TABLE publication_observations (
	id            TEXT PRIMARY KEY,
	edition_id    TEXT NOT NULL REFERENCES editions(id),
	value         TEXT NOT NULL CHECK (value IN ('unknown', 'ongoing', 'completed', 'hiatus', 'cancelled')),
	observed_at   TEXT NOT NULL,
	recorded_at   TEXT NOT NULL,
	source        TEXT,
	provenance    TEXT NOT NULL,
	supersedes_id TEXT REFERENCES publication_observations(id)
);

CREATE INDEX publication_observations_edition_idx ON publication_observations (edition_id);

-- A row is superseded at most once.
CREATE UNIQUE INDEX publication_observations_superseded_once ON publication_observations (supersedes_id) WHERE supersedes_id IS NOT NULL;
