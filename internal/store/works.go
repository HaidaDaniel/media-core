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

// Work and title failures. Application services map these to the stable
// wire error codes; the store stays transport-agnostic.
var (
	ErrWorkNotFound      = errors.New("store: work not found")
	ErrWorkTitleNotFound = errors.New("store: work title not found")
	ErrNoTitle           = errors.New("store: work requires at least one title")
	ErrInvalidTitle      = errors.New("store: invalid title")
	ErrRevisionMismatch  = errors.New("store: expected revision does not match")
)

// TitleInput is one requested work title. Language is null when unknown and
// is stored verbatim: there is no subtag-count limit.
type TitleInput struct {
	Text       string
	Language   *string
	Kind       domain.TitleKind
	Preferred  bool
	Provenance string
}

// Work is a persisted story grouping with its active titles.
type Work struct {
	ID         string
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
	Titles     []WorkTitle
}

// WorkTitle is one work title row, active or retired. SearchKey is the
// normalized matching key computed from Text.
type WorkTitle struct {
	ID         string
	WorkID     string
	Text       string
	Language   *string
	Kind       domain.TitleKind
	Preferred  bool
	Provenance string
	CreatedAt  time.Time
	RetiredAt  *time.Time
	SearchKey  string
}

// validateTitleInput rejects empty text, unknown kinds, empty languages and
// missing provenance before anything touches the database.
func validateTitleInput(in TitleInput) error {
	if strings.TrimSpace(in.Text) == "" {
		return fmt.Errorf("%w: text is empty", ErrInvalidTitle)
	}
	if !in.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidTitle, in.Kind)
	}
	if in.Language != nil && strings.TrimSpace(*in.Language) == "" {
		return fmt.Errorf("%w: language must be null or non-empty", ErrInvalidTitle)
	}
	if strings.TrimSpace(in.Provenance) == "" {
		return fmt.Errorf("%w: provenance is required", ErrInvalidTitle)
	}
	return nil
}

// beginImmediate starts a short write transaction that issues BEGIN
// IMMEDIATE (the store DSN sets the driver's _txlock=immediate parameter),
// so writes serialize identity checks and history changes from the first
// statement.
func (d *Database) beginImmediate(ctx context.Context) (*sql.Tx, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("store: begin transaction: %w", err)
	}
	return tx, nil
}

// instantUTC renders one UTC instant as fixed-width millisecond RFC3339 so
// that stored strings keep chronological lexicographic order and two
// observations recorded within the same second stay distinguishable; the
// (observed_at, recorded_at, id) ordering of current-state selection must
// reflect the real order of operations.
func instantUTC(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func parseInstant(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateWork stores a new work with its initial titles in one write
// transaction. At least one title is required; at most one may be marked
// preferred, and when none is, the first title becomes preferred. It
// returns the stored work.
func (d *Database) CreateWork(ctx context.Context, now time.Time, titles []TitleInput) (Work, error) {
	if len(titles) == 0 {
		return Work{}, ErrNoTitle
	}
	preferredCount := 0
	for _, in := range titles {
		if err := validateTitleInput(in); err != nil {
			return Work{}, err
		}
		if in.Preferred {
			preferredCount++
		}
	}
	if preferredCount > 1 {
		return Work{}, fmt.Errorf("%w: at most one preferred title", ErrInvalidTitle)
	}
	workID, err := domain.NewUUID()
	if err != nil {
		return Work{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	createdAt := instantUTC(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO works (id, revision, created_at, updated_at) VALUES (?, 0, ?, ?)`,
		workID, createdAt, createdAt); err != nil {
		return Work{}, fmt.Errorf("store: insert work: %w", err)
	}
	for i, in := range titles {
		id, err := domain.NewUUID()
		if err != nil {
			return Work{}, err
		}
		preferred := in.Preferred
		if preferredCount == 0 && i == 0 {
			preferred = true
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_titles (id, work_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, workID, i+1, in.Text, domain.SearchKey(in.Text), in.Language, string(in.Kind), boolInt(preferred), in.Provenance, createdAt); err != nil {
			return Work{}, fmt.Errorf("store: insert work title: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit work: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// GetWork returns one work with its active titles; retired titles are
// history and are not listed.
func (d *Database) GetWork(ctx context.Context, workID string) (Work, error) {
	var (
		w          Work
		createdAt  string
		updatedAt  string
		archivedAt sql.NullString
	)
	err := d.db.QueryRowContext(ctx,
		`SELECT id, revision, created_at, updated_at, archived_at FROM works WHERE id = ?`, workID).
		Scan(&w.ID, &w.Revision, &createdAt, &updatedAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, fmt.Errorf("%w: %s", ErrWorkNotFound, workID)
	}
	if err != nil {
		return Work{}, fmt.Errorf("store: query work: %w", err)
	}
	w.CreatedAt, err = parseInstant(createdAt)
	if err != nil {
		return Work{}, fmt.Errorf("store: parse work created_at: %w", err)
	}
	w.UpdatedAt, err = parseInstant(updatedAt)
	if err != nil {
		return Work{}, fmt.Errorf("store: parse work updated_at: %w", err)
	}
	if archivedAt.Valid {
		t, err := parseInstant(archivedAt.String)
		if err != nil {
			return Work{}, fmt.Errorf("store: parse work archived_at: %w", err)
		}
		w.ArchivedAt = &t
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, text, search_key, language, kind, preferred, provenance, created_at
		 FROM work_titles WHERE work_id = ? AND retired_at IS NULL ORDER BY seq`, workID)
	if err != nil {
		return Work{}, fmt.Errorf("store: query work titles: %w", err)
	}
	defer rows.Close()
	w.Titles = []WorkTitle{}
	for rows.Next() {
		var (
			t         WorkTitle
			createdAt string
			language  sql.NullString
			kind      string
			preferred int
		)
		if err := rows.Scan(&t.ID, &t.Text, &t.SearchKey, &language, &kind, &preferred, &t.Provenance, &createdAt); err != nil {
			return Work{}, fmt.Errorf("store: scan work title: %w", err)
		}
		t.WorkID = workID
		t.Kind = domain.TitleKind(kind)
		t.Preferred = preferred == 1
		if language.Valid {
			t.Language = &language.String
		}
		t.CreatedAt, err = parseInstant(createdAt)
		if err != nil {
			return Work{}, fmt.Errorf("store: parse title created_at: %w", err)
		}
		w.Titles = append(w.Titles, t)
	}
	return w, rows.Err()
}

// ListWorks returns every work ordered by the normalized preferred title,
// then ID, with active titles attached.
func (d *Database) ListWorks(ctx context.Context) ([]Work, error) {
	workIDs, err := d.workIDsByPreferredKey(ctx)
	if err != nil {
		return nil, err
	}
	works := make([]Work, 0, len(workIDs))
	for _, id := range workIDs {
		w, err := d.GetWork(ctx, id)
		if err != nil {
			return nil, err
		}
		works = append(works, w)
	}
	return works, nil
}

// SearchWorks returns distinct works whose active titles contain the
// normalized query as a substring, ordered like ListWorks. LIKE wildcards in
// the query are escaped so percent and underscore match literally. An empty
// normalized query lists all works.
func (d *Database) SearchWorks(ctx context.Context, query string) ([]Work, error) {
	key := domain.SearchKey(query)
	if key == "" {
		return d.ListWorks(ctx)
	}
	pattern := "%" + escapeLike(key) + "%"
	rows, err := d.db.QueryContext(ctx, `
		SELECT w.id
		FROM works w
		JOIN work_titles p ON p.work_id = w.id AND p.retired_at IS NULL AND p.preferred = 1
		JOIN work_titles m ON m.work_id = w.id AND m.retired_at IS NULL AND m.search_key LIKE ? ESCAPE '\'
		ORDER BY p.search_key, w.id`, pattern)
	if err != nil {
		return nil, fmt.Errorf("store: search works: %w", err)
	}
	defer rows.Close()
	workIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan work search: %w", err)
		}
		workIDs = append(workIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	works := make([]Work, 0, len(workIDs))
	for _, id := range workIDs {
		w, err := d.GetWork(ctx, id)
		if err != nil {
			return nil, err
		}
		works = append(works, w)
	}
	return works, nil
}

// workIDsByPreferredKey lists work IDs ordered by normalized preferred
// title, then ID.
func (d *Database) workIDsByPreferredKey(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT w.id
		FROM works w
		JOIN work_titles t ON t.work_id = w.id AND t.retired_at IS NULL AND t.preferred = 1
		ORDER BY t.search_key, w.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list works: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan work list: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// escapeLike escapes the LIKE pattern metacharacters and the escape
// character so the pattern matches literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// requireRevision checks the expected revision against the stored work
// inside the write transaction and rejects stale writers with a
// distinguishable conflict.
func requireRevision(tx *sql.Tx, ctx context.Context, workID string, expectedRevision int64) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM works WHERE id = ?`, workID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: %s", ErrWorkNotFound, workID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: read work revision: %w", err)
	}
	if revision != expectedRevision {
		return 0, fmt.Errorf("%w: expected %d, current %d", ErrRevisionMismatch, expectedRevision, revision)
	}
	return revision, nil
}

// bumpWorkRevision advances the work revision and updated_at; zero affected
// rows means a concurrent writer won the race.
func bumpWorkRevision(tx *sql.Tx, ctx context.Context, workID string, revision int64, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE works SET revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`,
		instantUTC(now), workID, revision)
	if err != nil {
		return fmt.Errorf("store: bump work revision: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: bump work revision: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("%w: expected %d, but the work was updated concurrently", ErrRevisionMismatch, revision)
	}
	return nil
}

// AddWorkTitle appends a title to a work. When the new title is marked
// preferred it demotes the current preferred title (display choice, not
// correction). expectedRevision guards against lost updates.
func (d *Database) AddWorkTitle(ctx context.Context, now time.Time, workID string, expectedRevision int64, in TitleInput) (Work, error) {
	if err := validateTitleInput(in); err != nil {
		return Work{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Work{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	revision, err := requireRevision(tx, ctx, workID, expectedRevision)
	if err != nil {
		return Work{}, err
	}
	var seq int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM work_titles WHERE work_id = ?`, workID).Scan(&seq); err != nil {
		return Work{}, fmt.Errorf("store: next title seq: %w", err)
	}
	if in.Preferred {
		if _, err := tx.ExecContext(ctx,
			`UPDATE work_titles SET preferred = 0 WHERE work_id = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`,
			workID, id); err != nil {
			return Work{}, fmt.Errorf("store: demote preferred title: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO work_titles (id, work_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, workID, seq, in.Text, domain.SearchKey(in.Text), in.Language, string(in.Kind), boolInt(in.Preferred), in.Provenance, instantUTC(now)); err != nil {
		return Work{}, fmt.Errorf("store: insert work title: %w", err)
	}
	if err := bumpWorkRevision(tx, ctx, workID, revision, now); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit title: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// RetireWorkTitle retires one active title, keeping the row as history with
// its provenance. If the retired title was preferred, the oldest remaining
// active title is promoted so every work keeps exactly one active preferred
// display title. Retiring the last active title is rejected.
func (d *Database) RetireWorkTitle(ctx context.Context, now time.Time, workID string, expectedRevision int64, titleID string) (Work, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	revision, err := requireRevision(tx, ctx, workID, expectedRevision)
	if err != nil {
		return Work{}, err
	}
	var (
		preferred int
		active    int
	)
	err = tx.QueryRowContext(ctx,
		`SELECT preferred, (SELECT COUNT(*) FROM work_titles WHERE work_id = ? AND retired_at IS NULL)
		 FROM work_titles WHERE id = ? AND work_id = ? AND retired_at IS NULL`,
		workID, titleID, workID).Scan(&preferred, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, fmt.Errorf("%w: %s", ErrWorkTitleNotFound, titleID)
	}
	if err != nil {
		return Work{}, fmt.Errorf("store: read title for retirement: %w", err)
	}
	if active <= 1 {
		return Work{}, fmt.Errorf("%w: the last active title cannot be retired", ErrInvalidTitle)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE work_titles SET retired_at = ? WHERE id = ? AND work_id = ? AND retired_at IS NULL`,
		instantUTC(now), titleID, workID); err != nil {
		return Work{}, fmt.Errorf("store: retire title: %w", err)
	}
	if preferred == 1 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE work_titles SET preferred = 1
			WHERE id = (
				SELECT id FROM work_titles
				WHERE work_id = ? AND retired_at IS NULL
				ORDER BY seq
				LIMIT 1
			)`, workID); err != nil {
			return Work{}, fmt.Errorf("store: promote preferred title: %w", err)
		}
	}
	if err := bumpWorkRevision(tx, ctx, workID, revision, now); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit retirement: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// SetPreferredWorkTitle makes one active title the preferred display title,
// demoting the current preferred title without retiring it.
func (d *Database) SetPreferredWorkTitle(ctx context.Context, now time.Time, workID string, expectedRevision int64, titleID string) (Work, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	revision, err := requireRevision(tx, ctx, workID, expectedRevision)
	if err != nil {
		return Work{}, err
	}
	// Demote before promoting so the partial unique index on active
	// preferred titles is never violated.
	if _, err := tx.ExecContext(ctx,
		`UPDATE work_titles SET preferred = 0 WHERE work_id = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`,
		workID, titleID); err != nil {
		return Work{}, fmt.Errorf("store: demote preferred title: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE work_titles SET preferred = 1 WHERE id = ? AND work_id = ? AND retired_at IS NULL`,
		titleID, workID)
	if err != nil {
		return Work{}, fmt.Errorf("store: set preferred title: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Work{}, fmt.Errorf("store: set preferred title: %w", err)
	}
	if n != 1 {
		return Work{}, fmt.Errorf("%w: %s", ErrWorkTitleNotFound, titleID)
	}
	if err := bumpWorkRevision(tx, ctx, workID, revision, now); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit preferred: %w", err)
	}
	return d.GetWork(ctx, workID)
}
