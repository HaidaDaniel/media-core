-- 0010: edition external references with roles and provenance.
-- A reference is a sourced pointer attached to an edition: a bounded
-- non-empty provider namespace plus a case-sensitive external ID, or an
-- absolute http(s) URL (normalized for identity). The role classifies the
-- reference; authoritative is an explicit assertion, never inferred from
-- the URL or a metadata database. Identity uniqueness stays in
-- external_identities (0004): a reference add reuses that reservation, so
-- there is no second dedupe policy here. Retirement keeps the row with its
-- retirement instant and correction provenance, never an erase.
CREATE TABLE external_references (
	id             TEXT PRIMARY KEY,
	edition_id     TEXT NOT NULL REFERENCES editions(id),
	provider       TEXT NOT NULL CHECK (LENGTH(TRIM(provider)) BETWEEN 1 AND 64),
	external_id    TEXT,
	normalized_url TEXT,
	role           TEXT NOT NULL CHECK (role IN
		('publication', 'metadata', 'translation', 'release_watcher', 'translation_project', 'content')),
	authoritative  INTEGER NOT NULL,
	language       TEXT,
	provenance     TEXT NOT NULL CHECK (LENGTH(TRIM(provenance)) > 0),
	observed_at    TEXT NOT NULL,
	recorded_at    TEXT NOT NULL,
	actor          TEXT NOT NULL,
	created_at     TEXT NOT NULL,
	retired_at     TEXT,
	retired_reason TEXT,
	CHECK ((external_id IS NOT NULL) OR (normalized_url IS NOT NULL))
);

CREATE INDEX external_references_edition_idx ON external_references (edition_id);
