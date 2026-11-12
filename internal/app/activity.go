package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// ActivityType is the wire name of one history row's type; it reuses the
// store's typed projection variant, including the reserved imported-release
// value no table produces yet.
type ActivityType = store.ActivityType

// App-level names of the activity types.
const (
	ActivityConsumption     ActivityType = store.ActivityConsumption
	ActivityProgress        ActivityType = store.ActivityProgress
	ActivityIntention       ActivityType = store.ActivityIntention
	ActivityRating          ActivityType = store.ActivityRating
	ActivityPublication     ActivityType = store.ActivityPublication
	ActivityNote            ActivityType = store.ActivityNote
	ActivityReference       ActivityType = store.ActivityReference
	ActivityRelation        ActivityType = store.ActivityRelation
	ActivityOrigin          ActivityType = store.ActivityOrigin
	ActivityTag             ActivityType = store.ActivityTag
	ActivityImportedRelease ActivityType = store.ActivityImportedRelease
)

const (
	DefaultActivityLimit = 30
	MaxActivityLimit     = 100
)

// ActivityQuery is one bounded recent-activity request. Types filters the
// feed (empty selects every type); the cursor is opaque and bound to the
// limit and the type fingerprint.
type ActivityQuery struct {
	Types  []ActivityType
	Limit  int
	Cursor *string
}

// HistoryQuery is one bounded entity-history request: exactly one of
// WorkID or EditionID. The cursor is opaque and bound to the entity and
// the limit.
type HistoryQuery struct {
	WorkID    *string
	EditionID *string
	Limit     int
	Cursor    *string
}

// ActivityItem is one wire activity or history entry: the source row, its
// correction label and its target's current annotation, reference and
// title enrichment. Unavailable enrichment is an empty collection, never
// null.
type ActivityItem struct {
	ID           string              `json:"id"`
	Type         ActivityType        `json:"type"`
	WorkID       string              `json:"work_id"`
	EditionID    *string             `json:"edition_id"`
	Title        *string             `json:"title"`
	Value        *string             `json:"value"`
	Detail       *string             `json:"detail"`
	Label        string              `json:"label"`
	Reason       *string             `json:"reason"`
	SupersedesID *string             `json:"supersedes_id"`
	ReplacesID   *string             `json:"replaces_id"`
	ObservedAt   *time.Time          `json:"observed_at"`
	RecordedAt   time.Time           `json:"recorded_at"`
	Actor        *Actor              `json:"actor"`
	Notes        []Note              `json:"notes"`
	References   []ExternalReference `json:"references"`
}

// activityCursorPayload binds a seek point to the listing it belongs to:
// the limit and the request fingerprint keep a cursor from one request
// from replaying on another.
type activityCursorPayload struct {
	V          int    `json:"v"`
	Limit      int    `json:"limit"`
	QHash      string `json:"q"`
	RecordedAt string `json:"r"`
	ID         string `json:"id"`
}

// activityFingerprint is the canonical identity of the non-cursor part of
// one activity request; a cursor from a different request is rejected.
func activityFingerprint(types []ActivityType, kind, id string) string {
	ts := make([]string, 0, len(types))
	for _, t := range types {
		ts = append(ts, string(t))
	}
	sort.Strings(ts)
	b, err := json.Marshal(map[string]any{
		"types": ts,
		"kind":  kind,
		"id":    id,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func issueActivityCursor(limit int, qHash, recordedAt, id string) string {
	b, _ := json.Marshal(activityCursorPayload{V: 1, Limit: limit, QHash: qHash, RecordedAt: recordedAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func parseActivityCursor(raw string, limit int, qHash string) (*activityCursorPayload, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("the cursor is not decodable")
	}
	var c activityCursorPayload
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("the cursor is not decodable")
	}
	if c.V != 1 {
		return nil, fmt.Errorf("the cursor version is not supported")
	}
	if c.Limit != limit || c.QHash != qHash {
		return nil, fmt.Errorf("the cursor does not match the query; start a new listing")
	}
	if c.ID == "" {
		return nil, fmt.Errorf("the cursor does not match the query; start a new listing")
	}
	return &c, nil
}

func activityLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultActivityLimit, nil
	}
	if limit < 1 || limit > MaxActivityLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", MaxActivityLimit)
	}
	return limit, nil
}

// activityItemFrom renders one projected history row as the wire item;
// the current annotation, reference and title enrichment of the row's
// target travels with the row, so a feed entry is self-describing.
func activityItemFrom(r store.ActivityRow) ActivityItem {
	item := ActivityItem{
		ID:           r.ID,
		Type:         r.Type,
		WorkID:       r.WorkID,
		EditionID:    r.EditionID,
		Title:        r.Title,
		Value:        r.Value,
		Detail:       r.Detail,
		Label:        r.Label,
		Reason:       r.Reason,
		SupersedesID: r.SupersedesID,
		ReplacesID:   r.ReplacesID,
		ObservedAt:   r.ObservedAt,
		RecordedAt:   r.RecordedAt,
		Notes:        notesViewFrom(r.Notes),
		References:   referencesViewFrom(r.References),
	}
	if r.Actor != nil {
		item.Actor = &Actor{
			Principal: r.Actor.Principal,
			Origin:    domain.ActorOrigin(r.Actor.Origin),
			Identity:  r.Actor.Identity,
		}
	}
	return item
}

// ListActivity projects the bounded recent personal activity over every
// history source: progress, status, opinions and relation corrections,
// ordered recorded_at descending then id, with the total count before
// pagination. Reads never mutate; a failed read leaves the prior state
// intact.
func (s *Service) ListActivity(ctx context.Context, q ActivityQuery) (ListPayload, error) {
	base := MutationBase{}
	limit, err := activityLimit(q.Limit)
	if err != nil {
		return ListPayload{}, invalidArgument(err.Error(), base)
	}
	for _, t := range q.Types {
		if !t.Valid() {
			return ListPayload{}, invalidArgument("unknown activity type "+string(t), base)
		}
	}
	qHash := activityFingerprint(q.Types, "", "")
	var afterRecordedAt, afterID string
	if q.Cursor != nil {
		c, err := parseActivityCursor(*q.Cursor, limit, qHash)
		if err != nil {
			return ListPayload{}, invalidArgument(err.Error(), base)
		}
		afterRecordedAt, afterID = c.RecordedAt, c.ID
	}
	rows, total, err := s.db.RecentActivity(ctx, store.ActivityQuery{
		Types:           q.Types,
		Limit:           limit + 1,
		AfterRecordedAt: afterRecordedAt,
		AfterID:         afterID,
	})
	if err != nil {
		return ListPayload{}, mapStoreError(err, base)
	}
	var next *string
	if len(rows) > limit {
		// A probe row beyond the page proves more rows exist; the cursor
		// seeks after the last served row, never after the probe, so no
		// row is skipped at a page boundary.
		last := rows[limit-1]
		next = strPtr(issueActivityCursor(limit, qHash, store.InstantKey(last.RecordedAt), last.ID))
		rows = rows[:limit]
	}
	items := make([]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, activityItemFrom(r))
	}
	return ListPayload{Items: items, NextCursor: next, Total: total}, nil
}

// GetHistory projects the bounded complete history of one work or one
// edition in the same order as the feed. A missing entity is a stable
// not-found; a history of an empty entity is an empty page, never an
// error.
func (s *Service) GetHistory(ctx context.Context, q HistoryQuery) (ListPayload, error) {
	base := MutationBase{}
	if (q.WorkID == nil) == (q.EditionID == nil) {
		return ListPayload{}, invalidArgument("exactly one of work_id or edition_id is required", base)
	}
	var kind, id string
	if q.EditionID != nil {
		if strings.TrimSpace(*q.EditionID) == "" {
			return ListPayload{}, invalidArgument("edition_id is required", base)
		}
		if _, err := s.db.GetEdition(ctx, *q.EditionID); err != nil {
			return ListPayload{}, mapStoreError(err, base)
		}
		kind, id = "edition", *q.EditionID
	} else {
		if strings.TrimSpace(*q.WorkID) == "" {
			return ListPayload{}, invalidArgument("work_id is required", base)
		}
		if _, err := s.db.GetWork(ctx, *q.WorkID); err != nil {
			return ListPayload{}, mapStoreError(err, base)
		}
		kind, id = "work", *q.WorkID
	}
	limit, err := activityLimit(q.Limit)
	if err != nil {
		return ListPayload{}, invalidArgument(err.Error(), base)
	}
	qHash := activityFingerprint(nil, kind, id)
	var afterRecordedAt, afterID string
	if q.Cursor != nil {
		c, err := parseActivityCursor(*q.Cursor, limit, qHash)
		if err != nil {
			return ListPayload{}, invalidArgument(err.Error(), base)
		}
		afterRecordedAt, afterID = c.RecordedAt, c.ID
	}
	rows, total, err := s.db.EntityHistory(ctx, store.HistoryQuery{
		WorkID:          q.WorkID,
		EditionID:       q.EditionID,
		Limit:           limit + 1,
		AfterRecordedAt: afterRecordedAt,
		AfterID:         afterID,
	})
	if err != nil {
		return ListPayload{}, mapStoreError(err, base)
	}
	var next *string
	if len(rows) > limit {
		last := rows[limit-1]
		next = strPtr(issueActivityCursor(limit, qHash, store.InstantKey(last.RecordedAt), last.ID))
		rows = rows[:limit]
	}
	items := make([]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, activityItemFrom(r))
	}
	return ListPayload{Items: items, NextCursor: next, Total: total}, nil
}
