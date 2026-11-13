package httpapi

import (
	"net/http"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
)

// setIntentionBody is the POST /editions/{id}/intention payload: the
// idempotency key, the expected edition revision, the intention value and
// the optional observation fields. The shared service validates the value.
type setIntentionBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Value            string  `json:"value"`
	Reason           *string `json:"reason"`
	ObservedAt       *string `json:"observed_at"`
	SupersedesID     *string `json:"supersedes_id"`
}

// setRatingBody is the POST /editions/{id}/ratings payload; a null value is
// the explicit clear, never zero.
type setRatingBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Value            *int    `json:"value"`
	Comment          *string `json:"comment"`
	ObservedAt       *string `json:"observed_at"`
	SupersedesID     *string `json:"supersedes_id"`
}

// addReferenceBody is the POST /editions/{id}/references payload: exactly
// one of external_id or url, the role, the explicit authoritative assertion
// and the explicit provenance claim.
type addReferenceBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Provider         string  `json:"provider"`
	ExternalID       *string `json:"external_id"`
	URL              *string `json:"url"`
	Role             string  `json:"role"`
	Authoritative    bool    `json:"authoritative"`
	Language         *string `json:"language"`
	Provenance       string  `json:"provenance"`
}

// retireReferenceBody is the POST /editions/{id}/references/{reference_id}/retire
// payload; the reference id comes from the path.
type retireReferenceBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Reason           *string `json:"reason"`
}

// noteBody is the POST /notes payload: the target scope (work or edition)
// with its id plus the bounded note content.
type noteBody struct {
	RequestID string  `json:"request_id"`
	Target    string  `json:"target"`
	TargetID  string  `json:"target_id"`
	Text      string  `json:"text"`
	Spoiler   bool    `json:"spoiler"`
	Quote     *string `json:"quote"`
	Location  *string `json:"location"`
}

// reviseNoteBody is the PATCH /notes/{id} payload; the note id comes from
// the path and the expected revision is the note's current revision.
type reviseNoteBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Text             string  `json:"text"`
	Spoiler          bool    `json:"spoiler"`
	Quote            *string `json:"quote"`
	Location         *string `json:"location"`
}

// retractNoteBody is the POST /notes/{id}/retract payload; the note id comes
// from the path.
type retractNoteBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Reason           *string `json:"reason"`
}

// tagAssignmentBody is the POST/DELETE /tags/assignments payload: the target
// scope (work or edition) with its id plus the normalized tag name; the
// method chooses attach or detach.
type tagAssignmentBody struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision *int64 `json:"expected_revision"`
	Target           string `json:"target"`
	TargetID         string `json:"target_id"`
	Tag              string `json:"tag"`
}

// parseOptionalObservedAt maps an optional RFC3339 observed instant for the
// *time.Time request fields; a present but malformed value is an invalid
// argument and an absent value is nil.
func parseOptionalObservedAt(v *string) (*time.Time, *app.AppError) {
	if v == nil || *v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return nil, app.NewAppError(app.ErrInvalidArgument, "observed_at is not RFC3339")
	}
	return &t, nil
}

// handleSetIntention records one intention observation (or a supersedes_id
// correction) for an edition and returns the updated edition view.
func (s *Server) handleSetIntention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body setIntentionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	observedAt, appErr := parseOptionalObservedAt(body.ObservedAt)
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.SetIntentionRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		Value:        body.Value,
		Reason:       body.Reason,
		ObservedAt:   observedAt,
		SupersedesID: body.SupersedesID,
	}
	view, err := s.catalog.SetIntention(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleSetRating records one rating observation, an explicit clear (a null
// value) or a supersedes_id correction for an edition and returns the
// updated edition view.
func (s *Server) handleSetRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body setRatingBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	observedAt, appErr := parseOptionalObservedAt(body.ObservedAt)
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.SetRatingRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		Value:        body.Value,
		Comment:      body.Comment,
		ObservedAt:   observedAt,
		SupersedesID: body.SupersedesID,
	}
	view, err := s.catalog.SetRating(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleAddReference attaches one sourced external reference to an edition
// and returns the updated edition view.
func (s *Server) handleAddReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body addReferenceBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AddReferenceRequest{
		MutationBase:  app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:     editionID,
		Provider:      body.Provider,
		ExternalID:    body.ExternalID,
		URL:           body.URL,
		Role:          domain.ReferenceRole(body.Role),
		Authoritative: body.Authoritative,
		Language:      body.Language,
		Provenance:    body.Provenance,
	}
	view, err := s.catalog.AddReference(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRetireReference retires one reference of an edition; the rows stay
// in history, never erased.
func (s *Server) handleRetireReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	referenceID := r.PathValue("reference_id")
	var body retireReferenceBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RetireReferenceRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		ReferenceID:  referenceID,
		Reason:       body.Reason,
	}
	view, err := s.catalog.RetireReference(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleAddNote adds one note to the work or edition named by the target
// scope and returns the updated target view.
func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body noteBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AddNoteRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor()},
		TargetID:     body.TargetID,
		Text:         body.Text,
		Spoiler:      body.Spoiler,
		Quote:        body.Quote,
		Location:     body.Location,
	}
	var (
		view any
		err  error
	)
	switch body.Target {
	case "work":
		view, err = s.catalog.AddWorkNote(r.Context(), req)
	case "edition":
		view, err = s.catalog.AddEditionNote(r.Context(), req)
	default:
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "target must be work or edition"))
		return
	}
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleReviseNote edits one existing note: a new immutable revision points
// to the immediately preceding one.
func (s *Server) handleReviseNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		s.notFoundHandler(w, r)
		return
	}
	noteID := r.PathValue("id")
	var body reviseNoteBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.ReviseNoteRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		NoteID:       noteID,
		Text:         body.Text,
		Spoiler:      body.Spoiler,
		Quote:        body.Quote,
		Location:     body.Location,
	}
	view, err := s.catalog.ReviseNote(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRetractNote withdraws one note; the retraction revision keeps the
// retracted text and its audit, and the note becomes absent from the
// target's view.
func (s *Server) handleRetractNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	noteID := r.PathValue("id")
	var body retractNoteBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RetractNoteRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		NoteID:       noteID,
		Reason:       body.Reason,
	}
	view, err := s.catalog.RetractNote(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleTagAssignment attaches (POST) or detaches (DELETE) one normalized
// tag on the work or edition named by the target scope.
func (s *Server) handleTagAssignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		s.notFoundHandler(w, r)
		return
	}
	var body tagAssignmentBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.TagRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		TargetID:     body.TargetID,
		Tag:          body.Tag,
		Detach:       r.Method == http.MethodDelete,
	}
	var (
		view any
		err  error
	)
	switch body.Target {
	case "work":
		view, err = s.catalog.SetWorkTag(r.Context(), req)
	case "edition":
		view, err = s.catalog.SetEditionTag(r.Context(), req)
	default:
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "target must be work or edition"))
		return
	}
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}
