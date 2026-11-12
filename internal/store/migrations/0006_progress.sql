-- 0006: edition progress observations by unit.
--
-- Progress belongs to one edition and one unit. Observations are
-- append-only history rows: the latest non-superseded row per unit by
-- (observed_at, recorded_at, id) is the current position; a correction adds
-- a superseding row for the same unit instead of overwriting, and a row is
-- superseded at most once. Past-dated rows are stored but never overtake a
-- newer one. A unit without a live observation has no current progress
-- (absence, never position zero).
-- position is the exact decimal string (up to 3 fractional digits); total
-- is NULL when unknown, never zero or an estimate. A retraction is a marker
-- row for the same unit with position NULL, a required reason and the
-- superseded observation id; it renders as the absence of progress and
-- stays in history distinct from a unit never observed.
CREATE TABLE progress_observations (
	id            TEXT PRIMARY KEY,
	edition_id    TEXT NOT NULL REFERENCES editions(id),
	unit          TEXT NOT NULL CHECK (unit IN ('chapter', 'volume', 'episode', 'page', 'percent')),
	position      TEXT,
	total         TEXT,
	label         TEXT,
	reason        TEXT,
	observed_at   TEXT NOT NULL,
	recorded_at   TEXT NOT NULL,
	actor         TEXT NOT NULL,
	supersedes_id TEXT REFERENCES progress_observations(id)
);

CREATE INDEX progress_observations_edition_unit_idx ON progress_observations (edition_id, unit);

-- A row is superseded at most once.
CREATE UNIQUE INDEX progress_observations_superseded_once ON progress_observations (supersedes_id) WHERE supersedes_id IS NOT NULL;
