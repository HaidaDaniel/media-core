-- 0009: edition rating observations.
-- Ratings are append-only edition-scoped history rows like consumption:
-- the current rating is the last non-superseded observation by
-- (observed_at, recorded_at, id). The value is an integral 1-10 rating or
-- NULL, where NULL is the explicit clear (a cleared rating is never zero
-- and never invented). A change appends a new observation; a correction of
-- a mistaken rating supersedes the named row, which stays in history, and
-- a partial unique index keeps a row superseded at most once.

CREATE TABLE rating_observations (
	id            TEXT PRIMARY KEY,
	edition_id    TEXT NOT NULL REFERENCES editions(id),
	value         INTEGER CHECK (value IS NULL OR (value >= 1 AND value <= 10)),
	comment       TEXT,
	observed_at   TEXT NOT NULL,
	recorded_at   TEXT NOT NULL,
	actor         TEXT NOT NULL,
	supersedes_id TEXT REFERENCES rating_observations(id)
);

CREATE INDEX rating_observations_edition_idx ON rating_observations (edition_id);

CREATE UNIQUE INDEX rating_observations_supersede_uq
	ON rating_observations (supersedes_id) WHERE supersedes_id IS NOT NULL;
