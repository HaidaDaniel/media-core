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

// SortMode is the ordering of a library search; the work ID is always the
// stable tie-breaker.
type SortMode string

// Sort modes.
const (
	SortTitle     SortMode = "title"
	SortCreatedAt SortMode = "created_at"
	SortUpdatedAt SortMode = "updated_at"
)

// Valid reports whether the sort mode is a defined v1 value.
func (m SortMode) Valid() bool {
	switch m {
	case SortTitle, SortCreatedAt, SortUpdatedAt:
		return true
	}
	return false
}

const (
	DefaultLibraryLimit = 30
	// MaxLibraryLimit is the page bound; search internals fetch one probe
	// row beyond it to know whether a next page exists.
	MaxLibraryLimit = 100
)

// LibraryQuery is one bounded library search. Query is raw text; it is
// normalized before matching. Kinds and Publications are edition-qualified
// filters: a work qualifies through editions that satisfy every supplied
// dimension, never by combining unrelated editions. Later filter dimensions
// (consumption, intention, tags, rating) join the same predicate list.
type LibraryQuery struct {
	Query        string
	Kinds        []domain.EditionKind
	Publications []domain.Publication
	Sort         SortMode
	Limit        int
	// AfterKey and AfterID seek past the last served row of the previous
	// page; AfterID empty means the first page.
	AfterKey string
	AfterID  string
}

// CardEdition is one edition line summary inside a work card.
type CardEdition struct {
	ID       string
	Kind     domain.EditionKind
	Language *string
	Revision int64
}

// WorkCard is one search row: the work with its active titles, its edition
// summaries, the editions that matched the query and its sort key for the
// seek cursor.
type WorkCard struct {
	Work
	Editions          []CardEdition
	MatchedEditionIDs []string
	SortKey           string
}

// libraryWhere builds the shared search predicate: the normalized query
// match, the edition-qualified filters and, when seeking, the cursor
// condition. The work ID tie-breaker keeps every sort stable.
func libraryWhere(q LibraryQuery, key string, seek bool) (string, []any) {
	conds := []string{"1=1"}
	var args []any
	if key != "" {
		pattern := "%" + escapeLike(key) + "%"
		conds = append(conds, `(
			EXISTS (SELECT 1 FROM work_titles t
				WHERE t.work_id = w.id AND t.retired_at IS NULL AND t.search_key LIKE ? ESCAPE '\')
			OR EXISTS (SELECT 1 FROM edition_titles t
				JOIN editions e ON e.id = t.edition_id
				WHERE e.work_id = w.id AND t.retired_at IS NULL AND t.search_key LIKE ? ESCAPE '\')
		)`)
		args = append(args, pattern, pattern)
	}
	if len(q.Kinds) > 0 || len(q.Publications) > 0 {
		sub := "SELECT 1 FROM editions e WHERE e.work_id = w.id"
		if len(q.Kinds) > 0 {
			sub += fmt.Sprintf(" AND e.kind IN (%s)", placeholders(len(q.Kinds)))
			for _, k := range q.Kinds {
				args = append(args, string(k))
			}
		}
		if len(q.Publications) > 0 {
			sub += " AND " + currentPublicationIn(len(q.Publications))
			for _, p := range q.Publications {
				args = append(args, string(p))
			}
		}
		conds = append(conds, "EXISTS ("+sub+")")
	}
	if seek && q.AfterID != "" {
		conds = append(conds, seekClause(q.Sort))
		args = append(args, q.AfterKey, q.AfterID)
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// currentPublicationIn reports editions whose current (last non-superseded)
// publication observation is one of the given values.
func currentPublicationIn(n int) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM publication_observations o
		WHERE o.edition_id = e.id
		  AND o.value IN (%s)
		  AND NOT EXISTS (SELECT 1 FROM publication_observations s WHERE s.supersedes_id = o.id)
		  AND NOT EXISTS (
			SELECT 1 FROM publication_observations n
			WHERE n.edition_id = e.id
			  AND NOT EXISTS (SELECT 1 FROM publication_observations s2 WHERE s2.supersedes_id = n.id)
			  AND (n.observed_at, n.recorded_at, n.id) > (o.observed_at, o.recorded_at, o.id)
		  )
	)`, placeholders(n))
}

func seekClause(sort SortMode) string {
	switch sort {
	case SortCreatedAt:
		return "(w.created_at, w.id) > (?, ?)"
	case SortUpdatedAt:
		return "(w.updated_at, w.id) > (?, ?)"
	default:
		return "(p.search_key, w.id) > (?, ?)"
	}
}

func orderClause(sort SortMode) string {
	switch sort {
	case SortCreatedAt:
		return "ORDER BY w.created_at, w.id"
	case SortUpdatedAt:
		return "ORDER BY w.updated_at, w.id"
	default:
		return "ORDER BY p.search_key, w.id"
	}
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

const workCardFrom = `
	FROM works w
	JOIN work_titles p ON p.work_id = w.id AND p.retired_at IS NULL AND p.preferred = 1 `

// SearchLibrary lists distinct works matching the query, paginated by an
// opaque seek point, with the total count before pagination. Every work
// keeps exactly one active preferred title, so the preferred-title join is
// total.
func (d *Database) SearchLibrary(ctx context.Context, q LibraryQuery) ([]WorkCard, int64, error) {
	if !q.Sort.Valid() {
		return nil, 0, fmt.Errorf("store: invalid sort %q", q.Sort)
	}
	if q.Limit <= 0 {
		q.Limit = DefaultLibraryLimit
	}
	if q.Limit > MaxLibraryLimit+1 {
		return nil, 0, fmt.Errorf("store: limit exceeds %d", MaxLibraryLimit)
	}
	key := domain.SearchKey(q.Query)

	where, args := libraryWhere(q, key, false)
	var total int64
	if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) "+workCardFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count library: %w", err)
	}

	where, args = libraryWhere(q, key, true)
	rows, err := d.db.QueryContext(ctx, "SELECT w.id, w.revision, w.created_at, w.updated_at, w.archived_at, p.search_key "+workCardFrom+where+" "+orderClause(q.Sort)+" LIMIT ?", append(args, q.Limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: search library: %w", err)
	}
	type pageRow struct {
		card      WorkCard
		createdAt string
		updatedAt string
	}
	page := []pageRow{}
	for rows.Next() {
		var (
			r          pageRow
			archivedAt sql.NullString
		)
		if err := rows.Scan(&r.card.ID, &r.card.Revision, &r.createdAt, &r.updatedAt, &archivedAt, &r.card.SortKey); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("store: scan library row: %w", err)
		}
		if archivedAt.Valid {
			t, err := parseInstant(archivedAt.String)
			if err != nil {
				rows.Close()
				return nil, 0, fmt.Errorf("store: parse library archived_at: %w", err)
			}
			r.card.ArchivedAt = &t
		}
		page = append(page, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	// The single-connection pool serves one query at a time, so the page
	// is collected and closed before the per-work detail fetches.
	cards := make([]WorkCard, 0, len(page))
	for _, r := range page {
		r.card.CreatedAt, err = parseInstant(r.createdAt)
		if err != nil {
			return nil, 0, fmt.Errorf("store: parse library created_at: %w", err)
		}
		r.card.UpdatedAt, err = parseInstant(r.updatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("store: parse library updated_at: %w", err)
		}
		w, err := d.GetWork(ctx, r.card.ID)
		if err != nil {
			return nil, 0, err
		}
		r.card.Work = w
		editions, err := d.ListEditions(ctx, r.card.ID)
		if err != nil {
			return nil, 0, err
		}
		for _, e := range editions {
			r.card.Editions = append(r.card.Editions, CardEdition{ID: e.ID, Kind: e.Kind, Language: e.Language, Revision: e.Revision})
		}
		r.card.MatchedEditionIDs = matchedEditionIDs(key, w, editions, q)
		cards = append(cards, r.card)
	}
	return cards, total, nil
}

// matchedEditionIDs lists the editions of a matching work that satisfy the
// whole predicate set. A work-title match counts for every edition of the
// work; an edition-title match counts only for that edition.
func matchedEditionIDs(key string, w Work, editions []Edition, q LibraryQuery) []string {
	workMatch := key == ""
	if !workMatch {
		for _, t := range w.Titles {
			if strings.Contains(t.SearchKey, key) {
				workMatch = true
				break
			}
		}
	}
	out := []string{}
	for _, e := range editions {
		if len(q.Kinds) > 0 && !containsKind(q.Kinds, e.Kind) {
			continue
		}
		if len(q.Publications) > 0 && (e.Publication == nil || !containsPublication(q.Publications, e.Publication.Value)) {
			continue
		}
		matched := key == "" || workMatch
		if !matched {
			for _, t := range e.Titles {
				if strings.Contains(t.SearchKey, key) {
					matched = true
					break
				}
			}
		}
		if matched {
			out = append(out, e.ID)
		}
	}
	return out
}

func containsKind(kinds []domain.EditionKind, k domain.EditionKind) bool {
	for _, v := range kinds {
		if v == k {
			return true
		}
	}
	return false
}

func containsPublication(values []domain.Publication, p domain.Publication) bool {
	for _, v := range values {
		if v == p {
			return true
		}
	}
	return false
}

// entityResultJSON is the journal summary of one entity mutation: the
// replay re-derives the current view of the target.
func entityResultJSON(target, id string, revision int64) string {
	b, err := json.Marshal(map[string]any{target: id, "revision": revision})
	if err != nil {
		return ""
	}
	return string(b)
}

// recordJournalOr is a no-op when the mutation carries no journal input;
// the result summary is filled in before the row is written.
func recordJournalOr(tx *sql.Tx, ctx context.Context, j *JournalInput, now time.Time, resultJSON string) error {
	if j == nil {
		return nil
	}
	row := *j
	row.ResultJSON = resultJSON
	return recordJournalTx(ctx, tx, row, now)
}

// SetWorkArchived archives or unarchives a work. Archiving never deletes:
// titles, editions and history stay intact, the archive instant is
// recorded. Setting the same state again succeeds without incrementing the
// revision.
func (d *Database) SetWorkArchived(ctx context.Context, now time.Time, workID string, expectedRevision int64, archived bool, journal *JournalInput) (Work, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	revision, err := requireRevision(tx, ctx, workID, expectedRevision)
	if err != nil {
		return Work{}, err
	}
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT archived_at FROM works WHERE id = ?`, workID).Scan(&current); err != nil {
		return Work{}, fmt.Errorf("store: read work archive state: %w", err)
	}
	nowS := instantUTC(now)
	if current.Valid != archived {
		var set any
		if archived {
			set = nowS
		}
		if _, err := tx.ExecContext(ctx, `UPDATE works SET archived_at = ? WHERE id = ?`, set, workID); err != nil {
			return Work{}, fmt.Errorf("store: set work archive: %w", err)
		}
		if err := bumpWorkRevision(tx, ctx, workID, revision, now); err != nil {
			return Work{}, err
		}
		revision++
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("work_id", workID, revision)); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit work archive: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// SetEditionArchived archives or unarchives an edition; its titles,
// observations and history are preserved. Same-state calls succeed without
// incrementing the revision.
func (d *Database) SetEditionArchived(ctx context.Context, now time.Time, editionID string, expectedRevision int64, archived bool, journal *JournalInput) (Edition, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT archived_at FROM editions WHERE id = ?`, editionID).Scan(&current); err != nil {
		return Edition{}, fmt.Errorf("store: read edition archive state: %w", err)
	}
	nowS := instantUTC(now)
	if current.Valid != archived {
		var set any
		if archived {
			set = nowS
		}
		if _, err := tx.ExecContext(ctx, `UPDATE editions SET archived_at = ? WHERE id = ?`, set, editionID); err != nil {
			return Edition{}, fmt.Errorf("store: set edition archive: %w", err)
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
		return Edition{}, fmt.Errorf("store: commit edition archive: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// UpdateEditionMeta updates the edition kind, language and other label.
// Only the supplied fields change; the resulting combination is validated.
// A same-value no-op succeeds without incrementing the revision.
func (d *Database) UpdateEditionMeta(ctx context.Context, now time.Time, editionID string, expectedRevision int64, kind *domain.EditionKind, language, otherLabel *string, journal *JournalInput) (Edition, error) {
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
		curKind  string
		curLang  sql.NullString
		curLabel sql.NullString
	)
	if err := tx.QueryRowContext(ctx, `SELECT kind, language, other_label FROM editions WHERE id = ?`, editionID).
		Scan(&curKind, &curLang, &curLabel); err != nil {
		return Edition{}, fmt.Errorf("store: read edition metadata: %w", err)
	}
	resKind := curKind
	if kind != nil {
		resKind = string(*kind)
	}
	resLang := curLang.String
	var resLangPtr *string
	if language != nil {
		resLang = *language
		resLangPtr = language
	} else if curLang.Valid {
		v := curLang.String
		resLangPtr = &v
	}
	resLabel := curLabel.String
	var resLabelPtr *string
	if otherLabel != nil {
		resLabel = *otherLabel
		resLabelPtr = otherLabel
	} else if curLabel.Valid {
		v := curLabel.String
		resLabelPtr = &v
	}
	if kind != nil || language != nil || otherLabel != nil {
		if err := domain.ValidateEditionAttrs(domain.EditionKind(resKind), resLangPtr, resLabelPtr); err != nil {
			return Edition{}, fmt.Errorf("%w: %v", ErrInvalidEdition, err)
		}
	}
	noOp := (kind == nil || resKind == curKind) &&
		(language == nil || (curLang.Valid && resLang == curLang.String)) &&
		(otherLabel == nil || (curLabel.Valid == (resLabel != "") && resLabel == curLabel.String))
	if !noOp {
		var lang, label any
		if resLangPtr != nil {
			lang = *resLangPtr
		}
		if resLabelPtr != nil {
			label = *resLabelPtr
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE editions SET kind = ?, language = ?, other_label = ? WHERE id = ?`,
			resKind, lang, label, editionID); err != nil {
			return Edition{}, fmt.Errorf("store: update edition metadata: %w", err)
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
		return Edition{}, fmt.Errorf("store: commit edition metadata: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// TitleMutation is one atomic title correction: titles to add, active
// title IDs to retire and the title that must be preferred afterwards.
// History rows are kept; the operation is applied in one write transaction
// together with the optional journal row.
type TitleMutation struct {
	Add       []TitleInput
	Retire    []string
	Preferred *string
}

func (m TitleMutation) empty() bool {
	return len(m.Add) == 0 && len(m.Retire) == 0 && m.Preferred == nil
}

// ApplyWorkTitles applies one title correction to a work. A work keeps at
// least one active title; retiring the preferred title promotes the oldest
// remaining active title unless the mutation names a new preferred one.
func (d *Database) ApplyWorkTitles(ctx context.Context, now time.Time, workID string, expectedRevision int64, m TitleMutation, journal *JournalInput) (Work, error) {
	if m.empty() {
		return Work{}, fmt.Errorf("%w: nothing to apply", ErrInvalidTitle)
	}
	for _, t := range m.Add {
		if err := validateTitleInput(t); err != nil {
			return Work{}, err
		}
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
	preferred, err := applyTitleMutationTx(tx, ctx, now, "work", workID, m)
	if err != nil {
		return Work{}, err
	}
	if preferred {
		// Retiring the preferred title without naming a new one promotes
		// the oldest remaining active title.
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
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("work_id", workID, revision+1)); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit work titles: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// ApplyEditionTitles applies one title correction to an edition. Unlike a
// work, an edition may retire all of its titles and then falls back to its
// work title for display.
func (d *Database) ApplyEditionTitles(ctx context.Context, now time.Time, editionID string, expectedRevision int64, m TitleMutation, journal *JournalInput) (Edition, error) {
	if m.empty() {
		return Edition{}, fmt.Errorf("%w: nothing to apply", ErrInvalidTitle)
	}
	for _, t := range m.Add {
		if err := validateTitleInput(t); err != nil {
			return Edition{}, err
		}
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	preferred, err := applyTitleMutationTx(tx, ctx, now, "edition", editionID, m)
	if err != nil {
		return Edition{}, err
	}
	if preferred {
		if _, err := tx.ExecContext(ctx, `
			UPDATE edition_titles SET preferred = 1
			WHERE id = (
				SELECT id FROM edition_titles
				WHERE edition_id = ? AND retired_at IS NULL
				ORDER BY seq
				LIMIT 1
			)`, editionID); err != nil {
			return Edition{}, fmt.Errorf("store: promote preferred title: %w", err)
		}
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision+1)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition titles: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// applyTitleMutationTx retires, adds and re-points the titles of one work or
// edition inside the caller's transaction. It reports whether the preferred
// title was retired without a named or added replacement, in which case the
// caller promotes the oldest remaining active title.
func applyTitleMutationTx(tx *sql.Tx, ctx context.Context, now time.Time, target, id string, m TitleMutation) (bool, error) {
	targetTable, fk := "work_titles", "work_id"
	if target == "edition" {
		targetTable, fk = "edition_titles", "edition_id"
	}
	nowS := instantUTC(now)

	retirePreferred := false
	for _, titleID := range m.Retire {
		var preferred int
		err := tx.QueryRowContext(ctx, fmt.Sprintf(
			`SELECT preferred FROM %s WHERE id = ? AND %s = ? AND retired_at IS NULL`, targetTable, fk),
			titleID, id).Scan(&preferred)
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("store: %s title %s not found or already retired", target, titleID)
		}
		if err != nil {
			return false, fmt.Errorf("store: read title for retirement: %w", err)
		}
		if preferred == 1 {
			retirePreferred = true
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET retired_at = ? WHERE id = ? AND %s = ? AND retired_at IS NULL`, targetTable, fk),
			nowS, titleID, id); err != nil {
			return false, fmt.Errorf("store: retire %s title: %w", target, err)
		}
	}
	if m.Preferred != nil {
		for _, retired := range m.Retire {
			if retired == *m.Preferred {
				return false, fmt.Errorf("%w: the preferred title is being retired", ErrInvalidTitle)
			}
		}
	}
	active, err := activeTitleCount(tx, ctx, targetTable, fk, id)
	if err != nil {
		return false, fmt.Errorf("store: count active titles: %w", err)
	}
	if target == "work" && active == 0 && len(m.Add) == 0 {
		return false, fmt.Errorf("%w: the last active title cannot be retired", ErrInvalidTitle)
	}

	addedPreferred := false
	if len(m.Add) > 0 {
		preferredCount := 0
		for _, t := range m.Add {
			if t.Preferred {
				preferredCount++
			}
		}
		if preferredCount > 1 {
			return false, fmt.Errorf("%w: at most one preferred title", ErrInvalidTitle)
		}
		addedPreferred = preferredCount > 0
		var seq int
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(
			`SELECT COALESCE(MAX(seq), 0) + 1 FROM %s WHERE %s = ?`, targetTable, fk),
			id).Scan(&seq); err != nil {
			return false, fmt.Errorf("store: next title seq: %w", err)
		}
		for _, in := range m.Add {
			titleID, err := domain.NewUUID()
			if err != nil {
				return false, err
			}
			preferred := in.Preferred
			if !preferred && target == "edition" && !addedPreferred && active == 0 {
				// The first title of an edition becomes preferred so the
				// edition has a display title of its own; a work already
				// keeps its preferred title.
				preferred = true
			}
			if preferred {
				if _, err := tx.ExecContext(ctx, fmt.Sprintf(
					`UPDATE %s SET preferred = 0 WHERE %s = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`, targetTable, fk),
					id, titleID); err != nil {
					return false, fmt.Errorf("store: demote preferred title: %w", err)
				}
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
				INSERT INTO %s (id, %s, seq, text, search_key, language, kind, preferred, provenance, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, targetTable, fk),
				titleID, id, seq, in.Text, domain.SearchKey(in.Text), in.Language, string(in.Kind), boolInt(preferred), in.Provenance, nowS); err != nil {
				return false, fmt.Errorf("store: insert %s title: %w", target, err)
			}
			seq++
			active++
		}
	}

	if m.Preferred != nil {
		var n int
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(
			`SELECT COUNT(*) FROM %s WHERE id = ? AND %s = ? AND retired_at IS NULL`, targetTable, fk),
			*m.Preferred, id).Scan(&n); err != nil {
			return false, fmt.Errorf("store: read preferred title: %w", err)
		}
		if n != 1 {
			return false, fmt.Errorf("store: %s title %s is not active", target, *m.Preferred)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET preferred = 0 WHERE %s = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`, targetTable, fk),
			id, *m.Preferred); err != nil {
			return false, fmt.Errorf("store: demote preferred title: %w", err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET preferred = 1 WHERE id = ? AND %s = ? AND retired_at IS NULL`, targetTable, fk),
			*m.Preferred, id); err != nil {
			return false, fmt.Errorf("store: set preferred title: %w", err)
		}
	}
	return retirePreferred && m.Preferred == nil && !addedPreferred, nil
}

func activeTitleCount(tx *sql.Tx, ctx context.Context, table, fk, id string) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE %s = ? AND retired_at IS NULL`, table, fk), id).Scan(&n)
	return n, err
}
