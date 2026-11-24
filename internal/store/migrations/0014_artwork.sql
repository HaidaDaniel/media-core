-- Artwork selection is sourced metadata with an explicit user choice. The
-- image bytes live in a regenerable state-directory cache; this table keeps
-- the provenance and the local cache key. It never changes personal progress.
CREATE TABLE work_artwork (
  id TEXT PRIMARY KEY,
  work_id TEXT NOT NULL REFERENCES works(id),
  edition_id TEXT REFERENCES editions(id),
  provider TEXT NOT NULL,
  external_id TEXT NOT NULL,
  title TEXT NOT NULL,
  source_url TEXT NOT NULL,
  attribution_url TEXT NOT NULL,
  kind TEXT NOT NULL,
  local_ref TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  byte_size INTEGER NOT NULL CHECK (byte_size > 0),
  selected_by_user INTEGER NOT NULL DEFAULT 1 CHECK (selected_by_user IN (0, 1)),
  provenance TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  retired_at TEXT
);

CREATE INDEX work_artwork_work_idx
  ON work_artwork(work_id) WHERE retired_at IS NULL;

CREATE UNIQUE INDEX work_artwork_identity_idx
  ON work_artwork(work_id, COALESCE(edition_id, ''), provider, external_id)
  WHERE retired_at IS NULL;
