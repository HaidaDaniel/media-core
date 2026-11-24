package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Artwork lookup failures. Application adapters map these to stable wire
// errors; the store stays transport-agnostic.
var (
	ErrArtworkNotFound = errors.New("store: artwork not found")
	ErrArtworkExists   = errors.New("store: a user-selected artwork already exists")
)

// ArtworkRecord is one stored artwork selection. The image bytes are in the
// regenerable state-directory cache; the row keeps provenance and the local
// reference. A retired row is retained, never erased.
type ArtworkRecord struct {
	ID             string
	WorkID         string
	EditionID      *string
	Provider       string
	ExternalID     string
	Title          string
	SourceURL      string
	AttributionURL string
	Kind           string
	LocalRef       string
	MIMEType       string
	ByteSize       int64
	SelectedByUser bool
	Provenance     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ArtworkInput is one user-confirmed artwork selection.
type ArtworkInput struct {
	WorkID         string
	EditionID      *string
	Provider       string
	ExternalID     string
	Title          string
	SourceURL      string
	AttributionURL string
	Kind           string
	LocalRef       string
	MIMEType       string
	ByteSize       int64
	Provenance     string
}

func validateArtworkInput(in ArtworkInput) error {
	if strings.TrimSpace(in.WorkID) == "" {
		return fmt.Errorf("store: artwork work id is required")
	}
	if in.EditionID != nil && strings.TrimSpace(*in.EditionID) == "" {
		return fmt.Errorf("store: artwork edition id must be null or non-empty")
	}
	bounded := map[string]string{
		"provider": in.Provider, "external id": in.ExternalID, "title": in.Title,
		"source url": in.SourceURL, "attribution url": in.AttributionURL,
		"kind": in.Kind, "local reference": in.LocalRef, "mime type": in.MIMEType,
		"provenance": in.Provenance,
	}
	for label, value := range bounded {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("store: artwork %s is required", label)
		}
		if len(value) > 2048 {
			return fmt.Errorf("store: artwork %s exceeds 2048 characters", label)
		}
	}
	if in.ByteSize <= 0 {
		return fmt.Errorf("store: artwork byte size must be positive")
	}
	return nil
}

// SaveArtwork stores a user selection. An existing active selection on the
// same work/edition is never silently overwritten: the caller must pass an
// explicit replace decision, which retires the old row in the same write
// transaction and keeps it in history.
func (d *Database) SaveArtwork(ctx context.Context, in ArtworkInput, now time.Time, replace bool) (ArtworkRecord, error) {
	if err := validateArtworkInput(in); err != nil {
		return ArtworkRecord{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return ArtworkRecord{}, err
	}
	stamp := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: begin artwork save: %w", err)
	}
	defer tx.Rollback()
	var workExists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM works WHERE id = ?`, in.WorkID).Scan(&workExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ArtworkRecord{}, fmt.Errorf("store: artwork work not found")
		}
		return ArtworkRecord{}, fmt.Errorf("store: check artwork work: %w", err)
	}
	if in.EditionID != nil {
		var editionWork string
		if err := tx.QueryRowContext(ctx, `SELECT work_id FROM editions WHERE id = ?`, *in.EditionID).Scan(&editionWork); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ArtworkRecord{}, fmt.Errorf("store: artwork edition not found")
			}
			return ArtworkRecord{}, fmt.Errorf("store: check artwork edition: %w", err)
		}
		if editionWork != in.WorkID {
			return ArtworkRecord{}, fmt.Errorf("store: artwork edition belongs to another work")
		}
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_artwork
		WHERE work_id = ? AND COALESCE(edition_id, '') = COALESCE(?, '') AND retired_at IS NULL`,
		in.WorkID, in.EditionID).Scan(&existing); err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: check existing artwork: %w", err)
	}
	if existing > 0 {
		if !replace {
			return ArtworkRecord{}, ErrArtworkExists
		}
		if _, err := tx.ExecContext(ctx, `UPDATE work_artwork SET retired_at = ?, updated_at = ?
			WHERE work_id = ? AND COALESCE(edition_id, '') = COALESCE(?, '') AND retired_at IS NULL`,
			stamp, stamp, in.WorkID, in.EditionID); err != nil {
			return ArtworkRecord{}, fmt.Errorf("store: retire replaced artwork: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO work_artwork
		(id, work_id, edition_id, provider, external_id, title, source_url,
		 attribution_url, kind, local_ref, mime_type, byte_size,
		 selected_by_user, provenance, created_at, updated_at, retired_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, NULL)`,
		id, in.WorkID, in.EditionID, in.Provider, in.ExternalID, in.Title,
		in.SourceURL, in.AttributionURL, in.Kind, in.LocalRef, in.MIMEType,
		in.ByteSize, in.Provenance, stamp, stamp)
	if err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: insert artwork: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: commit artwork: %w", err)
	}
	return ArtworkRecord{
		ID: id, WorkID: in.WorkID, EditionID: in.EditionID, Provider: in.Provider,
		ExternalID: in.ExternalID, Title: in.Title, SourceURL: in.SourceURL,
		AttributionURL: in.AttributionURL, Kind: in.Kind, LocalRef: in.LocalRef,
		MIMEType: in.MIMEType, ByteSize: in.ByteSize, SelectedByUser: true,
		Provenance: in.Provenance, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// RetireArtwork marks an active selection as replaced. The row and its
// provenance remain in the database and in backups.
func (d *Database) RetireArtwork(ctx context.Context, artworkID, reason string, now time.Time) error {
	if strings.TrimSpace(artworkID) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("store: artwork id and retirement reason are required")
	}
	res, err := d.db.ExecContext(ctx, `UPDATE work_artwork SET retired_at = ?, updated_at = ?
		WHERE id = ? AND retired_at IS NULL`, instantUTC(now), instantUTC(now), artworkID)
	if err != nil {
		return fmt.Errorf("store: retire artwork: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: retire artwork rows: %w", err)
	}
	if n == 0 {
		return ErrArtworkNotFound
	}
	return nil
}

const artworkColumns = `id, work_id, edition_id, provider, external_id, title,
	source_url, attribution_url, kind, local_ref, mime_type, byte_size,
	selected_by_user, provenance, created_at, updated_at`

func scanArtwork(scan func(...any) error) (ArtworkRecord, error) {
	var (
		r       ArtworkRecord
		edition sql.NullString
		created string
		updated string
		selected int
	)
	if err := scan(&r.ID, &r.WorkID, &edition, &r.Provider, &r.ExternalID, &r.Title,
		&r.SourceURL, &r.AttributionURL, &r.Kind, &r.LocalRef, &r.MIMEType,
		&r.ByteSize, &selected, &r.Provenance, &created, &updated); err != nil {
		return ArtworkRecord{}, err
	}
	if edition.Valid {
		r.EditionID = &edition.String
	}
	r.SelectedByUser = selected != 0
	var err error
	if r.CreatedAt, err = parseInstant(created); err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: parse artwork created_at: %w", err)
	}
	if r.UpdatedAt, err = parseInstant(updated); err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: parse artwork updated_at: %w", err)
	}
	return r, nil
}

// ArtworkByID loads one active selection.
func (d *Database) ArtworkByID(ctx context.Context, id string) (ArtworkRecord, error) {
	r, err := scanArtwork(d.db.QueryRowContext(ctx,
		`SELECT `+artworkColumns+` FROM work_artwork WHERE id = ? AND retired_at IS NULL`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtworkRecord{}, ErrArtworkNotFound
	}
	if err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: load artwork: %w", err)
	}
	return r, nil
}

// ArtworkForWorks loads at most one active artwork per requested work in a
// single query, so a library page never performs N+1 reads. Work-level art
// wins over edition-level art when both exist.
func (d *Database) ArtworkForWorks(ctx context.Context, workIDs []string) (map[string]ArtworkRecord, error) {
	out := map[string]ArtworkRecord{}
	if len(workIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(workIDs))
	marks := make([]string, 0, len(workIDs))
	for _, id := range workIDs {
		args = append(args, id)
		marks = append(marks, "?")
	}
	rows, err := d.db.QueryContext(ctx, `SELECT `+artworkColumns+` FROM work_artwork
		WHERE retired_at IS NULL AND work_id IN (`+strings.Join(marks, ",")+`)
		ORDER BY CASE WHEN edition_id IS NULL THEN 0 ELSE 1 END, created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list artwork for works: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanArtwork(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan artwork row: %w", err)
		}
		if _, exists := out[r.WorkID]; !exists {
			out[r.WorkID] = r
		}
	}
	return out, rows.Err()
}

// ArtworkForEdition loads the active selection of one edition, if any.
func (d *Database) ArtworkForEdition(ctx context.Context, editionID string) (ArtworkRecord, error) {
	r, err := scanArtwork(d.db.QueryRowContext(ctx,
		`SELECT `+artworkColumns+` FROM work_artwork WHERE edition_id = ? AND retired_at IS NULL
		 ORDER BY created_at DESC, id DESC LIMIT 1`, editionID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtworkRecord{}, ErrArtworkNotFound
	}
	if err != nil {
		return ArtworkRecord{}, fmt.Errorf("store: load edition artwork: %w", err)
	}
	return r, nil
}

// MissingArtworkWork is one work without an active cover, in stable order.
type MissingArtworkWork struct {
	ID    string
	Title string
	Kind  string
}

// ListWorksMissingArtwork lists works with no active artwork. It is one
// bounded query used by the explicit Find missing covers action; it never
// fetches provider data.
func (d *Database) ListWorksMissingArtwork(ctx context.Context, offset, limit int) ([]MissingArtworkWork, int, error) {
	if offset < 0 || limit < 1 || limit > 20 {
		return nil, 0, fmt.Errorf("store: invalid artwork batch bounds")
	}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM works w
		WHERE w.archived_at IS NULL AND NOT EXISTS
		  (SELECT 1 FROM work_artwork a WHERE a.work_id = w.id AND a.retired_at IS NULL)`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count works missing artwork: %w", err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT w.id, COALESCE(t.text, w.id),
		COALESCE((SELECT e.kind FROM editions e WHERE e.work_id = w.id ORDER BY e.created_at, e.id LIMIT 1), 'other')
		FROM works w
		LEFT JOIN work_titles t ON t.work_id = w.id AND t.retired_at IS NULL AND t.preferred = 1
		WHERE w.archived_at IS NULL AND NOT EXISTS
		  (SELECT 1 FROM work_artwork a WHERE a.work_id = w.id AND a.retired_at IS NULL)
		ORDER BY w.created_at, w.id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list works missing artwork: %w", err)
	}
	defer rows.Close()
	out := []MissingArtworkWork{}
	for rows.Next() {
		var w MissingArtworkWork
		if err := rows.Scan(&w.ID, &w.Title, &w.Kind); err != nil {
			return nil, 0, fmt.Errorf("store: scan missing artwork work: %w", err)
		}
		out = append(out, w)
	}
	return out, total, rows.Err()
}

// AllLocalRefs lists every active cache reference, used by cache accounting.
func (d *Database) AllLocalRefs(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT local_ref FROM work_artwork WHERE retired_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("store: list artwork refs: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("store: scan artwork ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
