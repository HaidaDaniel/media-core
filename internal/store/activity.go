package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ActivityType classifies one history row of the personal activity
// projection over the authoritative observation and revision tables.
// ImportedRelease is reserved: no table produces it yet; the release
// importer (0043) extends the projection with it.
type ActivityType string

// Activity types.
const (
	ActivityConsumption     ActivityType = "consumption"
	ActivityProgress        ActivityType = "progress"
	ActivityIntention       ActivityType = "intention"
	ActivityRating          ActivityType = "rating"
	ActivityPublication     ActivityType = "publication"
	ActivityNote            ActivityType = "note"
	ActivityReference       ActivityType = "reference"
	ActivityRelation        ActivityType = "relation"
	ActivityOrigin          ActivityType = "origin"
	ActivityTag             ActivityType = "tag"
	ActivityImportedRelease ActivityType = "imported_release"
)

// Valid reports whether the type is a defined activity type.
func (t ActivityType) Valid() bool {
	switch t {
	case ActivityConsumption, ActivityProgress, ActivityIntention, ActivityRating,
		ActivityPublication, ActivityNote, ActivityReference, ActivityRelation,
		ActivityOrigin, ActivityTag, ActivityImportedRelease:
		return true
	}
	return false
}

// ActivityActor is the acting identity of one history row. The origin and
// identity are asserted, never inferred; publication rows carry no actor
// (the column is absent from the table), so the projection renders them
// without one.
type ActivityActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// parseActivityActor decodes one row's actor column: the observation, note
// and tag tables store canonical actor JSON, while the relation and origin
// tables store the bare principal.
func parseActivityActor(col sql.NullString) (*ActivityActor, error) {
	if !col.Valid {
		return nil, nil
	}
	if strings.HasPrefix(col.String, "{") {
		var a ActivityActor
		if err := json.Unmarshal([]byte(col.String), &a); err != nil {
			return nil, fmt.Errorf("store: parse activity actor: %w", err)
		}
		return &a, nil
	}
	return &ActivityActor{Principal: col.String}, nil
}

// ActivityRow is one projected history row: the source row id, its target
// work and edition, its correction label, the dimension value and the
// current annotation, reference and title enrichment of the target. The
// row is a projection, never a copy: the authoritative rows keep the
// history.
type ActivityRow struct {
	ID           string
	Type         ActivityType
	WorkID       string
	EditionID    *string
	Title        *string
	Value        *string
	Detail       *string
	Label        string
	Reason       *string
	SupersedesID *string
	ReplacesID   *string
	ObservedAt   *time.Time
	RecordedAt   time.Time
	Actor        *ActivityActor
	Notes        []NoteInfo
	References   []ReferenceInfo
}

// ActivityQuery is one bounded recent-activity projection request. Types
// filters the feed (empty selects every type); AfterRecordedAt and AfterID
// seek past the last served row of the previous page in the
// (recorded_at, id) descending order, where the fixed-width instants keep
// the lexicographic order the chronological one.
type ActivityQuery struct {
	Types           []ActivityType
	Limit           int
	AfterRecordedAt string
	AfterID         string
}

// HistoryQuery is one bounded entity-history request: exactly one of
// WorkID or EditionID. A work history spans the work-level rows and the
// rows of all its editions, each row keeping its edition id; an edition
// history holds the edition's own rows plus the relations where the
// edition is an endpoint.
type HistoryQuery struct {
	WorkID          *string
	EditionID       *string
	Limit           int
	AfterRecordedAt string
	AfterID         string
}

const (
	DefaultActivityLimit = 30
	// MaxActivityLimit is the page bound; the projection internals fetch
	// one probe row beyond it to know whether a next page exists.
	MaxActivityLimit = 100
)

// InstantKey renders one instant back into the fixed-width stored form so
// a seek cursor compares against the stored strings, which keep
// chronological lexicographic order.
func InstantKey(t time.Time) string { return instantUTC(t) }

// activityBranches is the UNION ALL over every authoritative history
// source. Every branch emits the same column list: the type, the source
// row id, the target work and edition (the relation's canonical from
// entity, the relation's other endpoint in arelated), the dimension value,
// the detail, the correction label, the reason, the forward correction
// links, the instants and the actor. The projection never writes: the
// tables keep the history.
const activityBranches = `
	SELECT 'consumption' AS atype, o.id, e.work_id, o.edition_id,
		o.value AS avalue, NULL AS adetail,
		CASE WHEN EXISTS (SELECT 1 FROM consumption_observations s WHERE s.supersedes_id = o.id) THEN 'superseded'
		     WHEN o.value = 'unknown' THEN 'retracted'
		     ELSE 'effective' END AS alabel,
		o.reason AS areason, o.supersedes_id AS asupersedes, NULL AS areplaces,
		o.observed_at AS aobserved, o.recorded_at AS arecorded, o.actor AS aactor,
		NULL AS arelated
	FROM consumption_observations o
	JOIN editions e ON e.id = o.edition_id
	UNION ALL
	SELECT 'progress', o.id, e.work_id, o.edition_id,
		o.unit,
		CASE WHEN o.position IS NULL THEN NULL
		     ELSE o.position || '/' || COALESCE(o.total, '') END,
		CASE WHEN EXISTS (SELECT 1 FROM progress_observations s WHERE s.supersedes_id = o.id) THEN 'superseded'
		     WHEN o.position IS NULL THEN 'retracted'
		     ELSE 'effective' END,
		o.reason, o.supersedes_id, NULL,
		o.observed_at, o.recorded_at, o.actor,
		NULL
	FROM progress_observations o
	JOIN editions e ON e.id = o.edition_id
	UNION ALL
	SELECT 'intention', o.id, e.work_id, o.edition_id,
		o.value, NULL,
		CASE WHEN EXISTS (SELECT 1 FROM intention_observations s WHERE s.supersedes_id = o.id) THEN 'superseded'
		     WHEN o.value = 'none' THEN 'retracted'
		     ELSE 'effective' END,
		o.reason, o.supersedes_id, NULL,
		o.observed_at, o.recorded_at, o.actor,
		NULL
	FROM intention_observations o
	JOIN editions e ON e.id = o.edition_id
	UNION ALL
	SELECT 'rating', o.id, e.work_id, o.edition_id,
		o.value, o.comment,
		CASE WHEN EXISTS (SELECT 1 FROM rating_observations s WHERE s.supersedes_id = o.id) THEN 'superseded'
		     WHEN o.value IS NULL THEN 'retracted'
		     ELSE 'effective' END,
		NULL, o.supersedes_id, NULL,
		o.observed_at, o.recorded_at, o.actor,
		NULL
	FROM rating_observations o
	JOIN editions e ON e.id = o.edition_id
	UNION ALL
	SELECT 'publication', o.id, e.work_id, o.edition_id,
		o.value,
		CASE WHEN o.source IS NULL THEN o.provenance
		     ELSE o.source || ' (' || o.provenance || ')' END,
		CASE WHEN EXISTS (SELECT 1 FROM publication_observations s WHERE s.supersedes_id = o.id) THEN 'superseded'
		     ELSE 'effective' END,
		NULL, o.supersedes_id, NULL,
		o.observed_at, o.recorded_at, NULL,
		NULL
	FROM publication_observations o
	JOIN editions e ON e.id = o.edition_id
	UNION ALL
	SELECT 'note', nr.id, COALESCE(n.work_id, ne.work_id), n.edition_id,
		substr(nr.text, 1, 200), 'revision ' || nr.revision,
		CASE WHEN nr.revision = n.revision AND nr.retracted = 1 THEN 'retracted'
		     WHEN nr.revision = n.revision THEN 'current'
		     ELSE 'superseded' END,
		nr.reason, NULL, NULL,
		NULL, nr.recorded_at, nr.actor,
		NULL
	FROM note_revisions nr
	JOIN notes n ON n.id = nr.note_id
	LEFT JOIN editions ne ON ne.id = n.edition_id
	UNION ALL
	SELECT 'reference', r.id, e.work_id, r.edition_id,
		r.role,
		r.provider || ':' || COALESCE(r.external_id, r.normalized_url),
		CASE WHEN r.retired_at IS NOT NULL THEN 'retired' ELSE 'active' END,
		r.retired_reason, NULL, NULL,
		r.observed_at, r.recorded_at, r.actor,
		NULL
	FROM external_references r
	JOIN editions e ON e.id = r.edition_id
	UNION ALL
	SELECT 'relation', m.id,
		CASE WHEN m.scope = 'work' THEN m.from_id ELSE me.work_id END,
		CASE WHEN m.scope = 'edition' THEN m.from_id ELSE NULL END,
		m.relation, m.scope || ':' || m.from_id || ' -> ' || m.to_id,
		m.state,
		NULL, NULL, m.replaces_id,
		NULL, m.created_at, m.created_by,
		m.to_id
	FROM media_relations m
	LEFT JOIN editions me ON me.id = m.from_id AND m.scope = 'edition'
	UNION ALL
	SELECT 'origin', c.id, e.work_id, c.edition_id,
		c.kind, c.provenance,
		c.state,
		NULL, NULL, c.replaces_id,
		NULL, c.created_at, c.created_by,
		NULL
	FROM origin_claims c
	JOIN editions e ON e.id = c.edition_id
	UNION ALL
	SELECT 'tag', ta.id, COALESCE(ta.work_id, te.work_id), ta.edition_id,
		tg.name, NULL,
		CASE WHEN ta.detached_at IS NOT NULL THEN 'detached' ELSE 'active' END,
		NULL, NULL, NULL,
		NULL, ta.attached_at, ta.actor,
		NULL
	FROM tag_assignments ta
	JOIN tags tg ON tg.id = ta.tag_id
	LEFT JOIN editions te ON te.id = ta.edition_id`

// activityWhere builds the feed filter: the optional type dimension, the
// entity scope of one history request and the seek condition. The type
// and entity dimensions are ANDed; the seek keeps the (recorded_at, id)
// descending order stable across pages.
func activityWhere(q interface {
	typeFilter() []ActivityType
	entityScope() (kind, id string)
	seekPoint() (recordedAt, id string)
}) (string, []any) {
	conds := []string{"1=1"}
	var args []any
	if ts := q.typeFilter(); len(ts) > 0 {
		conds = append(conds, "u.atype IN ("+placeholders(len(ts))+")")
		for _, t := range ts {
			args = append(args, string(t))
		}
	}
	if kind, id := q.entityScope(); kind != "" {
		if kind == "edition" {
			// The edition's own rows plus the relations where the
			// edition is an endpoint, through the related column.
			conds = append(conds, "(u.edition_id = ? OR u.arelated = ?)")
			args = append(args, id, id)
		} else {
			// The work-level rows plus the rows of all its editions,
			// each row keeping its edition id, so no state is mixed
			// silently across editions.
			conds = append(conds, "(u.work_id = ? OR u.edition_id IN (SELECT id FROM editions WHERE work_id = ?))")
			args = append(args, id, id)
		}
	}
	if recordedAt, id := q.seekPoint(); id != "" {
		conds = append(conds, "(u.arecorded, u.id) < (?, ?)")
		args = append(args, recordedAt, id)
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// activityQueryAdapter shapes one request for activityWhere.
type activityQueryAdapter struct {
	types []ActivityType
	kind  string
	id    string
	seekR string
	seekI string
}

func (a activityQueryAdapter) typeFilter() []ActivityType    { return a.types }
func (a activityQueryAdapter) entityScope() (string, string) { return a.kind, a.id }
func (a activityQueryAdapter) seekPoint() (string, string)   { return a.seekR, a.seekI }

// activityCounts reports the total of one projection request before
// pagination.
func (d *Database) activityCounts(ctx context.Context, adapter activityQueryAdapter) (int64, error) {
	where, args := activityWhere(adapter)
	var total int64
	if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ( "+activityBranches+" ) u "+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count activity: %w", err)
	}
	return total, nil
}

// activityPage fetches one page of the projection in the (recorded_at,
// id) descending order; the caller passes limit+1 to probe for a next
// page.
func (d *Database) activityPage(ctx context.Context, adapter activityQueryAdapter, limit int) ([]ActivityRow, error) {
	where, args := activityWhere(adapter)
	rows, err := d.db.QueryContext(ctx,
		"SELECT u.atype, u.id, u.work_id, u.edition_id, u.avalue, u.adetail, u.alabel, u.areason, u.asupersedes, u.areplaces, u.aobserved, u.arecorded, u.aactor "+
			"FROM ( "+activityBranches+" ) u "+where+" ORDER BY u.arecorded DESC, u.id DESC LIMIT ?",
		append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("store: query activity: %w", err)
	}
	defer rows.Close()
	out := []ActivityRow{}
	for rows.Next() {
		var (
			r          ActivityRow
			workID     sql.NullString
			editionID  sql.NullString
			value      sql.NullString
			detail     sql.NullString
			reason     sql.NullString
			supersedes sql.NullString
			replaces   sql.NullString
			observedAt sql.NullString
			recordedAt string
			actorCol   sql.NullString
		)
		if err := rows.Scan(&r.Type, &r.ID, &workID, &editionID, &value, &detail, &r.Label, &reason, &supersedes, &replaces, &observedAt, &recordedAt, &actorCol); err != nil {
			return nil, fmt.Errorf("store: scan activity row: %w", err)
		}
		if workID.Valid {
			r.WorkID = workID.String
		}
		if editionID.Valid {
			r.EditionID = &editionID.String
		}
		if value.Valid {
			r.Value = &value.String
		}
		if detail.Valid {
			r.Detail = &detail.String
		}
		if reason.Valid {
			r.Reason = &reason.String
		}
		if supersedes.Valid {
			r.SupersedesID = &supersedes.String
		}
		if replaces.Valid {
			r.ReplacesID = &replaces.String
		}
		if observedAt.Valid {
			t, err := parseInstant(observedAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse activity observed_at: %w", err)
			}
			r.ObservedAt = &t
		}
		t, err := parseInstant(recordedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse activity recorded_at: %w", err)
		}
		r.RecordedAt = t
		r.Actor, err = parseActivityActor(actorCol)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// enrichActivityRows attaches the current annotation, reference and title
// enrichment of each row's target. The single-connection pool serves one
// query at a time, so the page is collected and closed before the per-
// target fetches, and a target seen twice in one page is fetched once.
func (d *Database) enrichActivityRows(ctx context.Context, rows []ActivityRow) error {
	cache := map[string]*targetEnrichment{}
	enrich := func(kind, id string) (*targetEnrichment, error) {
		if e, ok := cache[kind+id]; ok {
			return e, nil
		}
		e := &targetEnrichment{}
		var err error
		if kind == "edition" {
			e, err = d.editionEnrichment(ctx, id)
		} else {
			e, err = d.workEnrichment(ctx, id)
		}
		if err != nil {
			return nil, err
		}
		cache[kind+id] = e
		return e, nil
	}
	for i := range rows {
		r := &rows[i]
		var kind, id string
		switch {
		case r.EditionID != nil:
			kind, id = "edition", *r.EditionID
		case r.WorkID != "":
			kind, id = "work", r.WorkID
		}
		if kind == "" {
			r.Notes = []NoteInfo{}
			r.References = []ReferenceInfo{}
			continue
		}
		e, err := enrich(kind, id)
		if err != nil {
			return err
		}
		r.Title = e.Title
		r.Notes = e.Notes
		r.References = e.References
	}
	return nil
}

// targetEnrichment is the current annotation, reference and title state
// of one target work or edition.
type targetEnrichment struct {
	Title      *string
	Notes      []NoteInfo
	References []ReferenceInfo
}

func (d *Database) workEnrichment(ctx context.Context, workID string) (*targetEnrichment, error) {
	w, err := d.GetWork(ctx, workID)
	if err != nil {
		return nil, err
	}
	e := &targetEnrichment{Notes: []NoteInfo{}, References: []ReferenceInfo{}}
	for _, t := range w.Titles {
		if t.Preferred {
			e.Title = &t.Text
			break
		}
	}
	notes, err := d.ListWorkNotes(ctx, workID)
	if err != nil {
		return nil, err
	}
	e.Notes = notes
	return e, nil
}

func (d *Database) editionEnrichment(ctx context.Context, editionID string) (*targetEnrichment, error) {
	e, err := d.GetEdition(ctx, editionID)
	if err != nil {
		return nil, err
	}
	out := &targetEnrichment{Notes: e.Notes, References: e.References}
	if out.Notes == nil {
		out.Notes = []NoteInfo{}
	}
	if out.References == nil {
		out.References = []ReferenceInfo{}
	}
	for _, t := range e.Titles {
		if t.Preferred {
			out.Title = &t.Text
			break
		}
	}
	return out, nil
}

// RecentActivity projects the bounded recent personal activity over every
// history source, ordered (recorded_at, id) descending, with the total
// count before pagination.
func (d *Database) RecentActivity(ctx context.Context, q ActivityQuery) ([]ActivityRow, int64, error) {
	if q.Limit <= 0 {
		q.Limit = DefaultActivityLimit
	}
	if q.Limit > MaxActivityLimit+1 {
		return nil, 0, fmt.Errorf("store: activity limit exceeds %d", MaxActivityLimit)
	}
	for _, t := range q.Types {
		if !t.Valid() {
			return nil, 0, fmt.Errorf("store: invalid activity type %q", t)
		}
	}
	// The total counts before pagination, so it is built without the
	// seek condition; the page applies it.
	total, err := d.activityCounts(ctx, activityQueryAdapter{types: q.Types})
	if err != nil {
		return nil, 0, err
	}
	rows, err := d.activityPage(ctx, activityQueryAdapter{types: q.Types, seekR: q.AfterRecordedAt, seekI: q.AfterID}, q.Limit)
	if err != nil {
		return nil, 0, err
	}
	if err := d.enrichActivityRows(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// EntityHistory projects the bounded complete history of one work or one
// edition in the same order and with the same enrichment as the feed.
func (d *Database) EntityHistory(ctx context.Context, q HistoryQuery) ([]ActivityRow, int64, error) {
	if (q.WorkID == nil) == (q.EditionID == nil) {
		return nil, 0, fmt.Errorf("store: history needs exactly one of work or edition")
	}
	if q.Limit <= 0 {
		q.Limit = DefaultActivityLimit
	}
	if q.Limit > MaxActivityLimit+1 {
		return nil, 0, fmt.Errorf("store: history limit exceeds %d", MaxActivityLimit)
	}
	var kind, id string
	if q.EditionID != nil {
		kind, id = "edition", *q.EditionID
	} else {
		kind, id = "work", *q.WorkID
	}
	// The total counts before pagination, so it is built without the
	// seek condition; the page applies it.
	total, err := d.activityCounts(ctx, activityQueryAdapter{kind: kind, id: id})
	if err != nil {
		return nil, 0, err
	}
	rows, err := d.activityPage(ctx, activityQueryAdapter{kind: kind, id: id, seekR: q.AfterRecordedAt, seekI: q.AfterID}, q.Limit)
	if err != nil {
		return nil, 0, err
	}
	if err := d.enrichActivityRows(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
