-- 0011: explicit media relations and origin claims.
-- One relation table holds typed, scoped, provenance-bearing edges with a
-- proposed/confirmed/revoked lifecycle. Both endpoints share the scope;
-- self links are impossible; symmetric types store endpoints in canonical
-- order so (a,b) and (b,a) are one edge and uniqueness is over
-- scope/type/endpoints for the non-revoked states. A revoked edge keeps
-- its row, state and replacement link; a replacement is a new edge with
-- replaces_id, never a silent rewrite. Origin claims are explicit
-- assertions on one edition with the same lifecycle; no originality is
-- ever inferred from a kind or a provider.
CREATE TABLE media_relations (
	id           TEXT PRIMARY KEY,
	scope        TEXT NOT NULL CHECK (scope IN ('work','edition')),
	relation     TEXT NOT NULL CHECK (relation IN ('adaptation_of','translation_of','alternative_version','same_underlying_work','sequel_of','side_story_of','related')),
	from_id      TEXT NOT NULL,
	to_id        TEXT NOT NULL,
	state        TEXT NOT NULL CHECK (state IN ('proposed','confirmed','revoked')),
	provenance   TEXT NOT NULL CHECK (LENGTH(TRIM(provenance)) > 0),
	confidence   REAL,
	replaces_id  TEXT,
	confirmed_by TEXT,
	confirmed_at TEXT,
	created_by   TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	revoked_by   TEXT,
	revoked_at   TEXT,
	CHECK (from_id <> to_id)
);
CREATE UNIQUE INDEX media_relations_active_idx
	ON media_relations (scope, relation, from_id, to_id) WHERE state != 'revoked';
CREATE INDEX media_relations_from_idx ON media_relations (scope, from_id);
CREATE INDEX media_relations_to_idx ON media_relations (scope, to_id);

CREATE TABLE origin_claims (
	id           TEXT PRIMARY KEY,
	edition_id   TEXT NOT NULL REFERENCES editions(id),
	kind         TEXT NOT NULL CHECK (kind IN ('original','anime_original','unknown')),
	state        TEXT NOT NULL CHECK (state IN ('proposed','confirmed','revoked')),
	provenance   TEXT NOT NULL CHECK (LENGTH(TRIM(provenance)) > 0),
	replaces_id  TEXT,
	confirmed_by TEXT,
	confirmed_at TEXT,
	created_by   TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	revoked_by   TEXT,
	revoked_at   TEXT
);
CREATE UNIQUE INDEX origin_claims_active_idx
	ON origin_claims (edition_id, kind) WHERE state != 'revoked';
CREATE INDEX origin_claims_edition_idx ON origin_claims (edition_id);
