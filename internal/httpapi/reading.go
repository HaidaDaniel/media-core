package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
)

// recordProgressBody is the POST /editions/{id}/progress payload: the
// idempotency key, the expected edition revision, the unit with its exact
// decimal position and optional total, label and reason, an optional RFC3339
// observed instant, and, for a correction, the superseded record id. The
// shared service validates the unit, the decimals and the revision.
type recordProgressBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Unit             string  `json:"unit"`
	Position         string  `json:"position"`
	Total            *string `json:"total"`
	Label            *string `json:"label"`
	ObservedAt       *string `json:"observed_at"`
	Reason           *string `json:"reason"`
	SupersedesID     *string `json:"supersedes_id"`
}

// setConsumptionBody is the POST /editions/{id}/consumption payload: the
// idempotency key, the expected edition revision, the consumption value and
// the optional observation fields. The shared service validates the value
// and the revision.
type setConsumptionBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Value            string  `json:"value"`
	ObservedAt       *string `json:"observed_at"`
	Reason           *string `json:"reason"`
	SupersedesID     *string `json:"supersedes_id"`
}

// retractProgressBody is the POST /editions/{id}/history/{record_id}/retract
// payload: the idempotency key, the expected edition revision and the
// required retraction reason; the record id comes from the path.
type retractProgressBody struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// parseObservedAt maps an optional RFC3339 observed instant; a present but
// malformed value is an invalid argument, an absent value the server clock.
func parseObservedAt(v *string) (time.Time, *app.AppError) {
	if v == nil || *v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return time.Time{}, app.NewAppError(app.ErrInvalidArgument, "observed_at is not RFC3339")
	}
	return t, nil
}

// handleRecordProgress records one progress observation (or a supersedes_id
// correction) for an edition and returns the updated edition view. Progress
// never changes consumption, intention or publication.
func (s *Server) handleRecordProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body recordProgressBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	observedAt, appErr := parseObservedAt(body.ObservedAt)
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		Unit:         domain.ProgressUnit(body.Unit),
		Position:     body.Position,
		Total:        body.Total,
		Label:        body.Label,
		ObservedAt:   observedAt,
		Reason:       body.Reason,
		SupersedesID: body.SupersedesID,
	}
	view, err := s.catalog.RecordProgress(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleSetConsumption records one consumption observation (or a
// supersedes_id correction) for an edition and returns the updated edition
// view. Status never changes progress, intention or publication.
func (s *Server) handleSetConsumption(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body setConsumptionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	var observedAt *time.Time
	if v, appErr := parseObservedAt(body.ObservedAt); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	} else if body.ObservedAt != nil && *body.ObservedAt != "" {
		observedAt = &v
	}
	req := app.SetConsumptionRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		Value:        body.Value,
		ObservedAt:   observedAt,
		Reason:       body.Reason,
		SupersedesID: body.SupersedesID,
	}
	view, err := s.catalog.SetConsumption(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRetractProgress retracts one progress observation through the typed
// retraction route: the retracted row stays in history superseded and the
// unit's current position falls back.
func (s *Server) handleRetractProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	recordID := r.PathValue("record_id")
	var body retractProgressBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RetractProgressRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		RecordID:     recordID,
		Reason:       body.Reason,
	}
	view, err := s.catalog.RetractProgress(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleGetHistory projects the bounded complete history of the edition from
// the path; limit and cursor come from the query string and are validated by
// the shared service.
func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	q := app.HistoryQuery{EditionID: &editionID}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "limit is not a number: "+v))
			return
		}
		q.Limit = n
	}
	if v := r.URL.Query().Get("cursor"); v != "" {
		q.Cursor = &v
	}
	payload, err := s.catalog.GetHistory(r.Context(), q)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, payload)
}
