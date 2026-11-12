package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Reference lookup failure. Application services map these to the stable
// wire error codes; the store stays transport-agnostic.
var ErrReferenceNotFound = errors.New("store: external reference not found")

// ReferenceActor is the acting identity of one reference operation.
type ReferenceActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin"`
	Identity  string `json:"identity,omitempty"`
}

// ReferenceRequest is one external reference to attach: a bounded non-empty
// provider namespace plus an external ID or an absolute http(s) URL, a
// role, the explicit authoritative assertion, an optional language and the
// explicit provenance claim. The source claim is never inferred from the
// URL or a metadata database, and links are never fetched.
type ReferenceRequest struct {
	Provider      string
	ExternalID    *string
	URL           *string
	Role          domain.ReferenceRole
	Authoritative bool
	Language      *string
	Provenance    string
}

// ReferenceInfo is one stored reference row. A retired reference keeps its
// row with the retirement instant and correction provenance, never an
// erase.
type ReferenceInfo struct {
	ID            string
	Provider      string
	ExternalID    *string
	URL           *string
	Role          domain.ReferenceRole
	Authoritative bool
	Language      *string
	Provenance    string
	RetiredAt     *time.Time
}

// validateReferenceRequest checks the reference shape before any journal key
// is reserved: a non-empty bounded provider, exactly one of external ID or
// URL (the URL must be an absolute http(s) URL, so javascript/file and
// every other scheme are rejected), a defined role, a non-empty provenance
// claim and a language that is null or non-empty.
func validateReferenceRequest(in ReferenceRequest) error {
	if strings.TrimSpace(in.Provider) == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidReference)
	}
	if len(in.Provider) > 64 {
		return fmt.Errorf("%w: provider exceeds 64 characters", ErrInvalidReference)
	}
	if (in.ExternalID == nil) == (in.URL == nil) {
		return fmt.Errorf("%w: exactly one of external_id or url is required", ErrInvalidReference)
	}
	if in.ExternalID != nil && strings.TrimSpace(*in.ExternalID) == "" {
		return fmt.Errorf("%w: external id is empty", ErrInvalidReference)
	}
	if in.URL != nil {
		if _, err := NormalizeURL(*in.URL); err != nil {
			return err
		}
	}
	if !in.Role.Valid() {
		return fmt.Errorf("%w: unknown role %q", ErrInvalidReference, in.Role)
	}
	if strings.TrimSpace(in.Provenance) == "" {
		return fmt.Errorf("%w: provenance is required", ErrInvalidReference)
	}
	if in.Language != nil && strings.TrimSpace(*in.Language) == "" {
		return fmt.Errorf("%w: language must be null or non-empty", ErrInvalidReference)
	}
	return nil
}

// describeIdentity names a reference identity for conflict messages.
func describeIdentity(in ReferenceRequest) string {
	if in.ExternalID != nil {
		return fmt.Sprintf("external id %q of provider %q", *in.ExternalID, in.Provider)
	}
	return fmt.Sprintf("url %q", *in.URL)
}

// currentReferences lists the active references of an edition in creation
// order; retired references are absent from the list but stay in history.
func (d *Database) currentReferences(ctx context.Context, editionID string) ([]ReferenceInfo, error) {
	return d.currentReferencesQuery(ctx, d.db, editionID)
}

// currentReferencesTx is the in-transaction variant of currentReferences.
func (d *Database) currentReferencesTx(ctx context.Context, tx *sql.Tx, editionID string) ([]ReferenceInfo, error) {
	return d.currentReferencesQuery(ctx, tx, editionID)
}

func (d *Database) currentReferencesQuery(ctx context.Context, q queryer, editionID string) ([]ReferenceInfo, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, provider, external_id, normalized_url, role, authoritative, language, provenance, retired_at
		FROM external_references
		WHERE edition_id = ? AND retired_at IS NULL
		ORDER BY created_at, id`, editionID)
	if err != nil {
		return nil, fmt.Errorf("store: list references: %w", err)
	}
	defer rows.Close()
	out := []ReferenceInfo{}
	for rows.Next() {
		var (
			r        ReferenceInfo
			ext      sql.NullString
			norm     sql.NullString
			role     string
			auth     int
			language sql.NullString
			retired  sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.Provider, &ext, &norm, &role, &auth, &language, &r.Provenance, &retired); err != nil {
			return nil, fmt.Errorf("store: scan reference: %w", err)
		}
		r.Role = domain.ReferenceRole(role)
		r.Authoritative = auth != 0
		if ext.Valid {
			r.ExternalID = &ext.String
		}
		if norm.Valid {
			r.URL = &norm.String
		}
		if language.Valid {
			r.Language = &language.String
		}
		if retired.Valid {
			t, err := parseInstant(retired.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse reference retired_at: %w", err)
			}
			r.RetiredAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// activeReferenceOn finds the active reference of the same identity on the
// edition, if any.
func activeReferenceOn(ctx context.Context, q queryer, editionID string, in ReferenceRequest, normURL *string) (*ReferenceInfo, error) {
	var (
		query string
		args  []any
	)
	if in.ExternalID != nil {
		query = `SELECT id, provider, external_id, normalized_url, role, authoritative, language, provenance, retired_at
			FROM external_references
			WHERE edition_id = ? AND retired_at IS NULL AND provider = ? AND external_id = ?
			LIMIT 1`
		args = []any{editionID, in.Provider, *in.ExternalID}
	} else {
		query = `SELECT id, provider, external_id, normalized_url, role, authoritative, language, provenance, retired_at
			FROM external_references
			WHERE edition_id = ? AND retired_at IS NULL AND normalized_url = ?
			LIMIT 1`
		args = []any{editionID, *normURL}
	}
	var (
		r        ReferenceInfo
		ext      sql.NullString
		norm     sql.NullString
		role     string
		auth     int
		language sql.NullString
		retired  sql.NullString
	)
	err := q.QueryRowContext(ctx, query, args...).
		Scan(&r.ID, &r.Provider, &ext, &norm, &role, &auth, &language, &r.Provenance, &retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find active reference: %w", err)
	}
	r.Role = domain.ReferenceRole(role)
	r.Authoritative = auth != 0
	if ext.Valid {
		r.ExternalID = &ext.String
	}
	if norm.Valid {
		r.URL = &norm.String
	}
	if language.Valid {
		r.Language = &language.String
	}
	return &r, nil
}

// sameReference reports whether a request re-asserts the exact active
// reference: the same role, authoritative assertion, language and
// provenance of the same identity.
func sameReference(existing ReferenceInfo, in ReferenceRequest) bool {
	if existing.Role != in.Role || existing.Authoritative != in.Authoritative || existing.Provenance != in.Provenance {
		return false
	}
	if (existing.Language == nil) != (in.Language == nil) {
		return false
	}
	if in.Language != nil && *existing.Language != *in.Language {
		return false
	}
	return true
}

// AddReference attaches one sourced external reference to an edition. The
// 0012 identity reservation decides conflicts: an active identity on
// another edition is an identity conflict, never a silent reassignment, and
// the same edition plus the same identity is one reference, so a new URL on
// the same edition adds a reference and never creates another edition. An
// exact re-assertion of the active reference is a no-op without a revision
// bump but the journal still settles; a reservation from the add path is
// promoted into a reference in the same transaction; otherwise the
// reference row and its 0004 identity reservation are written together. The
// edition revision guards lost updates and the optional journal row commits
// in the same transaction.
func (d *Database) AddReference(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in ReferenceRequest, actor ReferenceActor, journal *JournalInput) (Edition, error) {
	if err := validateReferenceRequest(in); err != nil {
		return Edition{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return Edition{}, fmt.Errorf("store: encode reference actor: %w", err)
	}
	normURL := (*string)(nil)
	if in.URL != nil {
		normalized, err := NormalizeURL(*in.URL)
		if err != nil {
			return Edition{}, err
		}
		normURL = &normalized
	}
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	// The 0012 identity reservation decides the conflict; the unique active
	// indexes make a racing reservation a conflict, never a duplicate.
	match, err := d.findIdentity(ctx, tx, ReferenceInput{Provider: &in.Provider, ExternalID: in.ExternalID, URL: in.URL})
	if err != nil {
		return Edition{}, err
	}
	if match != nil && match.EditionID != editionID {
		return Edition{}, fmt.Errorf("%w: %s is already reserved on edition %s", ErrIdentityConflict, describeIdentity(in), match.EditionID)
	}
	if match != nil {
		existing, err := activeReferenceOn(ctx, tx, editionID, in, normURL)
		if err != nil {
			return Edition{}, err
		}
		if existing != nil {
			if sameReference(*existing, in) {
				// The exact same reference: a no-op without a revision
				// bump; the journal still settles below.
				if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
					return Edition{}, err
				}
				if err := tx.Commit(); err != nil {
					return Edition{}, fmt.Errorf("store: commit reference no-op: %w", err)
				}
				return d.GetEdition(ctx, editionID)
			}
			return Edition{}, fmt.Errorf("%w: %s is already attached to this edition", ErrIdentityConflict, describeIdentity(in))
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO external_references (id, edition_id, provider, external_id, normalized_url, role, authoritative, language, provenance, observed_at, recorded_at, actor, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, editionID, in.Provider, in.ExternalID, normURL, string(in.Role), boolInt(in.Authoritative), in.Language, in.Provenance, nowS, nowS, string(actorJSON), nowS); err != nil {
		return Edition{}, fmt.Errorf("store: insert reference: %w", err)
	}
	if match == nil {
		reservationID, err := domain.NewUUID()
		if err != nil {
			return Edition{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO external_identities (id, edition_id, provider, external_id, normalized_url, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			reservationID, editionID, in.Provider, in.ExternalID, normURL, nowS); err != nil {
			return Edition{}, fmt.Errorf("%w: %v", ErrIdentityConflict, err)
		}
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision+1)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit reference: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// RetireReference retires one reference of an edition with correction
// provenance: the reference row and its 0004 identity reservation keep
// their rows with the retirement instant and the current view drops them,
// so the identity is free for resolution, never silently reassigned.
// Retiring an already-retired reference is a no-op without a revision bump
// but the journal still settles. The edition revision guards lost updates.
func (d *Database) RetireReference(ctx context.Context, now time.Time, editionID string, expectedRevision int64, referenceID string, reason *string, actor ReferenceActor, journal *JournalInput) (Edition, error) {
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	var (
		r        ReferenceInfo
		ext      sql.NullString
		norm     sql.NullString
		role     string
		auth     int
		language sql.NullString
		retired  sql.NullString
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, provider, external_id, normalized_url, role, authoritative, language, provenance, retired_at
		 FROM external_references WHERE id = ? AND edition_id = ?`, referenceID, editionID).
		Scan(&r.ID, &r.Provider, &ext, &norm, &role, &auth, &language, &r.Provenance, &retired)
	if errors.Is(err, sql.ErrNoRows) {
		return Edition{}, fmt.Errorf("%w: %s", ErrReferenceNotFound, referenceID)
	}
	if err != nil {
		return Edition{}, fmt.Errorf("store: read reference: %w", err)
	}
	if ext.Valid {
		r.ExternalID = &ext.String
	}
	if norm.Valid {
		r.URL = &norm.String
	}
	if language.Valid {
		r.Language = &language.String
	}
	if retired.Valid {
		t, err := parseInstant(retired.String)
		if err != nil {
			return Edition{}, fmt.Errorf("store: parse reference retired_at: %w", err)
		}
		r.RetiredAt = &t
	}
	if r.RetiredAt == nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE external_references SET retired_at = ?, retired_reason = ? WHERE id = ?`, nowS, reason, referenceID); err != nil {
			return Edition{}, fmt.Errorf("store: retire reference: %w", err)
		}
		if r.ExternalID != nil {
			if _, err := tx.ExecContext(ctx,
				`UPDATE external_identities SET retired_at = ?
				 WHERE edition_id = ? AND provider = ? AND external_id = ? AND retired_at IS NULL`,
				nowS, editionID, r.Provider, *r.ExternalID); err != nil {
				return Edition{}, fmt.Errorf("store: retire identity reservation: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE external_identities SET retired_at = ?
				 WHERE edition_id = ? AND normalized_url = ? AND retired_at IS NULL`,
				nowS, editionID, *r.URL); err != nil {
				return Edition{}, fmt.Errorf("store: retire identity reservation: %w", err)
			}
		}
		if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
			return Edition{}, err
		}
		revision++
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit reference retirement: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}
