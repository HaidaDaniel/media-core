-- 0004: external identity reservations and the mutation result journal.
--
-- An external identity is a provider + case-sensitive external ID, or a
-- normalized absolute http(s) URL. An active identity identifies an edition
-- globally: unique active (provider, external_id) and unique active
-- normalized_url, so a conflicting reservation on another edition requires
-- identity resolution, never reassignment. URL identities are global and
-- need no provider; http and https stay distinct.
--
-- The journal stores each mutation's canonical request hash and result in
-- the same transaction as the changes: an exact replay returns the stored
-- result, the same key with a different payload is a conflict, and a
-- failed validation never reserves a key. Results and keys are retained in
-- backups for the MVP (no premature expiration).
CREATE TABLE external_identities (
	id             TEXT PRIMARY KEY,
	edition_id     TEXT NOT NULL REFERENCES editions(id),
	provider       TEXT CHECK (provider IS NULL OR LENGTH(TRIM(provider)) BETWEEN 1 AND 64),
	external_id    TEXT,
	normalized_url TEXT,
	created_at     TEXT NOT NULL,
	retired_at     TEXT,
	CHECK ((external_id IS NOT NULL) OR (normalized_url IS NOT NULL)),
	CHECK (external_id IS NULL OR provider IS NOT NULL)
);

CREATE INDEX external_identities_edition_idx ON external_identities (edition_id);

CREATE UNIQUE INDEX external_identities_external_idx ON external_identities (provider, external_id)
WHERE external_id IS NOT NULL AND retired_at IS NULL;

CREATE UNIQUE INDEX external_identities_url_idx ON external_identities (normalized_url)
WHERE normalized_url IS NOT NULL AND retired_at IS NULL;

CREATE TABLE mutation_journal (
	request_id   TEXT PRIMARY KEY,
	operation    TEXT NOT NULL CHECK (LENGTH(TRIM(operation)) > 0),
	request_hash TEXT NOT NULL CHECK (LENGTH(request_hash) > 0),
	result_json  TEXT NOT NULL,
	actor        TEXT NOT NULL,
	created_at   TEXT NOT NULL
);
