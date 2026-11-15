package app

import (
	"context"
	"errors"

	"github.com/HaidaDaniel/media-core/internal/importer"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// Per-item import statuses.
const (
	ImportStatusImported = "imported"
	ImportStatusReplayed = "replayed"
	ImportStatusConflict = "conflict"
)

// ImportRequest is one Release Watcher import: the raw version 1.0 envelope
// bytes, the producer namespace and the decode bounds. The raw bytes are the
// file or stdin content exactly as received.
type ImportRequest struct {
	RequestID string
	Namespace string
	Actor     Actor
	Raw       []byte
	MaxBytes  int64
	MaxEvents int
}

// ImportItem is the per-item result of one imported event.
type ImportItem struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	Status    string `json:"status"`
	Pending   bool   `json:"pending"`
	Detail    string `json:"detail,omitempty"`
}

// ImportResult is the outcome of one import: the envelope identity, the
// per-item results and the aggregate counts.
type ImportResult struct {
	SchemaVersion string       `json:"schema_version"`
	NextCursor    *string      `json:"next_cursor,omitempty"`
	Imported      int          `json:"imported"`
	Replayed      int          `json:"replayed"`
	Conflicts     int          `json:"conflicts"`
	Items         []ImportItem `json:"items"`
}

// PendingImport is one pending import record in the queue.
type PendingImport struct {
	ID              string  `json:"id"`
	EventID         string  `json:"event_id"`
	EventType       string  `json:"event_type"`
	Namespace       string  `json:"namespace"`
	PublicationID   *string `json:"publication_id,omitempty"`
	ExternalWorkRef *string `json:"external_work_ref,omitempty"`
	ReleaseID       *string `json:"release_id,omitempty"`
	ReleaseNumber   *string `json:"release_number,omitempty"`
	ReleaseTitle    *string `json:"release_title,omitempty"`
	ReleaseURL      *string `json:"release_url,omitempty"`
	CurrentCount    *int    `json:"current_count,omitempty"`
	CountDelta      *int    `json:"count_delta,omitempty"`
	SourceTime      *string `json:"source_time,omitempty"`
	DetectionTime   *string `json:"detection_time,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

// ImportEvents decodes and stages one Release Watcher version 1.0 envelope
// as immutable pending records. The envelope and major are validated before
// any write: a decode failure returns an error and nothing is persisted.
// Each event is staged idempotently in its own transaction and reported as a
// per-item result. No edition is ever inferred from a title; an event stays
// pending until an explicit binding links it to an edition.
func (s *Service) ImportEvents(ctx context.Context, req ImportRequest) (ImportResult, error) {
	base := MutationBase{RequestID: req.RequestID, Actor: req.Actor}
	namespace := req.Namespace
	if namespace == "" {
		namespace = importer.DefaultNamespace
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = importer.DefaultEnvelopeBytes
	}
	maxEvents := req.MaxEvents
	if maxEvents <= 0 {
		maxEvents = importer.DefaultMaxEvents
	}
	decoded, err := importer.DecodeBytes(req.Raw, maxBytes, maxEvents)
	if err != nil {
		return ImportResult{}, NewAppError(ErrInvalidArgument, "import decode: "+err.Error())
	}
	now := s.now()
	res := ImportResult{SchemaVersion: decoded.SchemaVersion, NextCursor: decoded.NextCursor, Items: []ImportItem{}}
	for _, ev := range decoded.Events {
		in := store.EventInput{
			EventID:         ev.EventID,
			SchemaVersion:   ev.SchemaVersion,
			EventType:       ev.EventType,
			RawPayload:      string(ev.Raw),
			Canonical:       ev.Canonical,
			Namespace:       namespace,
			PublicationID:   ev.PublicationID,
			ExternalWorkRef: ev.ExternalWorkRef,
			ReleaseID:       ev.ReleaseID,
			ReleaseNumber:   ev.ReleaseNumber,
			ReleaseTitle:    ev.ReleaseTitle,
			ReleaseURL:      ev.ReleaseURL,
			CurrentCount:    ev.CurrentCount,
			CountDelta:      ev.CountDelta,
			SourceTime:      ev.SourceTime,
			DetectionTime:   ev.DetectionTime,
			WorkHint:        ev.WorkHint,
			EditionHint:     ev.EditionHint,
			CreatedAt:       now,
		}
		item := ImportItem{EventID: ev.EventID, EventType: ev.EventType}
		outcome, err := s.db.InsertEvent(ctx, in)
		switch {
		case err == nil:
			item.Status = outcome.Status
			if outcome.Status == ImportStatusImported {
				res.Imported++
			} else {
				res.Replayed++
			}
			pending, perr := s.eventPending(ctx, namespace, ev.PublicationID)
			if perr != nil {
				res.Items = append(res.Items, item)
				return res, importStoreError(perr, base)
			}
			item.Pending = pending
		case errors.Is(err, store.ErrEventConflict):
			// A per-item identity conflict: the existing row is never
			// overwritten and the import continues with the other events.
			item.Status = ImportStatusConflict
			item.Detail = err.Error()
			res.Conflicts++
		default:
			res.Items = append(res.Items, item)
			return res, importStoreError(err, base)
		}
		res.Items = append(res.Items, item)
	}
	return res, nil
}

// importStoreError maps an import store error to the application error,
// treating an invalid event or a malformed pending cursor as an
// invalid-argument (caller input) error rather than a storage failure.
func importStoreError(err error, base MutationBase) *AppError {
	if errors.Is(err, store.ErrInvalidEvent) {
		return &AppError{Code: ErrInvalidArgument, Message: err.Error(), RequestID: base.RequestID, Actor: base.Actor.Principal}
	}
	return mapStoreError(err, base)
}

// eventPending reports whether an event has no active binding for its
// (namespace, publication_id). An event without a publication id is always
// pending.
func (s *Service) eventPending(ctx context.Context, namespace string, publicationID *string) (bool, error) {
	if publicationID == nil {
		return true, nil
	}
	bound, err := s.db.HasActiveBinding(ctx, namespace, *publicationID)
	if err != nil {
		return false, err
	}
	return !bound, nil
}

// PendingImportQuery is one bounded pending-queue listing request.
type PendingImportQuery struct {
	Limit  int
	Cursor *string
}

// PendingImports lists the pending import queue: imported Release Watcher
// events with no active binding, oldest first, keyset-paginated.
func (s *Service) PendingImports(ctx context.Context, q PendingImportQuery) (ListPayload, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	events, total, next, err := s.db.ListPendingEvents(ctx, limit, q.Cursor)
	if err != nil {
		return ListPayload{}, importStoreError(err, MutationBase{})
	}
	items := make([]any, 0, len(events))
	for _, e := range events {
		items = append(items, PendingImport{
			ID: e.ID, EventID: e.EventID, EventType: e.EventType, Namespace: e.Namespace,
			PublicationID: e.PublicationID, ExternalWorkRef: e.ExternalWorkRef,
			ReleaseID: e.ReleaseID, ReleaseNumber: e.ReleaseNumber, ReleaseTitle: e.ReleaseTitle,
			ReleaseURL: e.ReleaseURL, CurrentCount: e.CurrentCount, CountDelta: e.CountDelta,
			SourceTime: e.SourceTime, DetectionTime: e.DetectionTime,
			CreatedAt: e.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}
	return ListPayload{Items: items, NextCursor: next, Total: total}, nil
}
