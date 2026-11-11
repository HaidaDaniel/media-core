-- 0002: works and work titles.
--
-- A work is a logical story grouping chosen by the user; creation requires
-- at least one title. Original display text is preserved exactly; search_key
-- is the Go-computed normalized key (NFKC, full case fold, collapsed
-- whitespace) used for matching only. Titles are corrected by retiring and
-- replacing rows; retired rows keep their history and provenance. The
-- edition table and edition titles land in 0003.
CREATE TABLE works (
	id          TEXT PRIMARY KEY,
	revision    INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	archived_at TEXT
);

CREATE TABLE work_titles (
	id         TEXT PRIMARY KEY,
	work_id    TEXT NOT NULL REFERENCES works(id),
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

-- Per-work insertion order; unique so title order is deterministic even
-- when timestamps coincide.
CREATE UNIQUE INDEX work_titles_work_seq ON work_titles (work_id, seq);

-- Active titles of one work, in creation order.
CREATE INDEX work_titles_work_active_idx ON work_titles (work_id) WHERE retired_at IS NULL;

-- At most one active preferred display title per work; exact normalized
-- titles are deliberately NOT unique: matching titles are candidate
-- matches, not identity.
CREATE UNIQUE INDEX work_titles_one_preferred ON work_titles (work_id) WHERE preferred = 1 AND retired_at IS NULL;
