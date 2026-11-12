-- 0005: personal consumption observations.
--
-- Personal consumption always targets an edition. Observations are
-- append-only history rows: the current state is the last non-superseded
-- observation by (observed_at, recorded_at, id); a correction or retraction
-- adds a superseding row instead of overwriting, and a row is superseded at
-- most once. Past-dated rows are stored but never overtake a newer one.
-- 'unknown' is the explicit retraction value: the user withdrew the state
-- to unknown; it renders as the absence of a consumption state and stays in
-- history distinct from an edition that never had an observation (unknown
-- historical consumption).
CREATE TABLE consumption_observations (
	id            TEXT PRIMARY KEY,
	edition_id    TEXT NOT NULL REFERENCES editions(id),
	value         TEXT NOT NULL CHECK (value IN ('not_started', 'in_progress', 'paused', 'completed', 'dropped', 'unknown')),
	observed_at   TEXT NOT NULL,
	recorded_at   TEXT NOT NULL,
	actor         TEXT NOT NULL,
	reason        TEXT,
	supersedes_id TEXT REFERENCES consumption_observations(id)
);

CREATE INDEX consumption_observations_edition_idx ON consumption_observations (edition_id);

-- A row is superseded at most once.
CREATE UNIQUE INDEX consumption_observations_superseded_once ON consumption_observations (supersedes_id) WHERE supersedes_id IS NOT NULL;
