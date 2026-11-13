package httpapi

import (
	"net/http"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// titleWire is one requested title in a write payload: text, optional
// language, kind, preferred flag and provenance. The shared service
// validates the values; the transport only type-maps them.
type titleWire struct {
	Text       string  `json:"text"`
	Language   *string `json:"language"`
	Kind       string  `json:"kind"`
	Preferred  bool    `json:"preferred"`
	Provenance string  `json:"provenance"`
}

func (w titleWire) input() store.TitleInput {
	return store.TitleInput{Text: w.Text, Language: w.Language, Kind: domain.TitleKind(w.Kind), Preferred: w.Preferred, Provenance: w.Provenance}
}

// referenceWire is one external reference: exactly one of external_id or
// url. The shared service validates the shape.
type referenceWire struct {
	Provider   *string `json:"provider"`
	ExternalID *string `json:"external_id"`
	URL        *string `json:"url"`
}

func (w referenceWire) input() store.ReferenceInput {
	return store.ReferenceInput{Provider: w.Provider, ExternalID: w.ExternalID, URL: w.URL}
}

// additionPayload is the shared addition wire payload: titles, optional
// edition attributes and references. resolve and create use it; create adds
// the idempotency key and, for the candidate flow, the explicit choice with
// its token.
type additionPayload struct {
	Titles     []titleWire     `json:"titles"`
	Kind       *string         `json:"kind"`
	Language   *string         `json:"language"`
	OtherLabel *string         `json:"other_label"`
	References []referenceWire `json:"references"`
}

func (p additionPayload) apply(req *app.AdditionRequest) {
	for _, t := range p.Titles {
		req.Titles = append(req.Titles, t.input())
	}
	if p.Kind != nil {
		k := domain.EditionKind(*p.Kind)
		req.Kind = &k
	}
	req.Language = p.Language
	req.OtherLabel = p.OtherLabel
	for _, r := range p.References {
		req.References = append(req.References, r.input())
	}
}

// resolveBody is the POST /additions/resolve payload. Resolve never writes,
// so it carries no candidate decision and no work id (the shared service
// accepts a work id only together with an explicit choice); a choice, a
// token or a work id here are unknown fields.
type resolveBody struct {
	RequestID string `json:"request_id"`
	additionPayload
}

// createWorkBody is the POST /works payload: the idempotency key plus the
// shared payload and, when candidates were resolved, the explicit choice
// with its token. The optional work_id disambiguates a use_existing choice
// among several candidate works, as the shared service requires.
type createWorkBody struct {
	RequestID string `json:"request_id"`
	WorkID    string `json:"work_id"`
	additionPayload
	Choice         *string `json:"choice"`
	CandidateToken *string `json:"candidate_token"`
}

// addEditionBody is the POST /works/{id}/editions payload: the idempotency
// key, the shared payload and the candidate token. The transport fixes the
// choice to add_edition and the work to the path, so neither appears in the
// body.
type addEditionBody struct {
	RequestID string `json:"request_id"`
	additionPayload
	CandidateToken *string `json:"candidate_token"`
}

// correctTitlesBody is the PATCH /works/{id}/titles payload: the idempotency
// key, the expected work revision and the title changes.
type correctTitlesBody struct {
	RequestID        string      `json:"request_id"`
	ExpectedRevision *int64      `json:"expected_revision"`
	Add              []titleWire `json:"add"`
	Retire           []string    `json:"retire"`
	Preferred        *string     `json:"preferred"`
}

// updateEditionBody is the PATCH /editions/{id} payload: the idempotency
// key, the expected edition revision and the metadata fields to change; at
// least one field is required by the shared service.
type updateEditionBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	Kind             *string `json:"kind"`
	Language         *string `json:"language"`
	OtherLabel       *string `json:"other_label"`
}

// trustedActor is the transport-set actor for every catalog mutation: the
// single owner principal with the asserted user origin. The body never
// supplies an actor, so a client cannot assert one.
func trustedActor() app.Actor {
	return app.Actor{Principal: "local", Origin: domain.OriginUser}
}

// decodeMutationBody decodes one strict JSON write body; a malformed or
// unknown-field body is an invalid argument before any service call.
func decodeMutationBody(r *http.Request, dst any) *app.AppError {
	if err := decodeBody(r, dst); err != nil {
		return app.NewAppError(app.ErrInvalidArgument, err.Error())
	}
	return nil
}

// handleResolveAddition resolves a manual addition without writing. It
// reports ready, an existing identity, or candidates with a short-lived
// decision token; it never treats a title match as confirmed identity.
func (s *Server) handleResolveAddition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body resolveBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AdditionRequest{MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor()}}
	body.apply(&req)
	res, err := s.catalog.ResolveAddition(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, res)
}

// handleCreateWork creates a library entry (or reports an existing one) with
// exact idempotent replay. A title match without an explicit choice is a
// duplicate-candidate conflict, never an automatic identity.
func (s *Server) handleCreateWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body createWorkBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AdditionRequest{MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor()}}
	body.apply(&req)
	if body.WorkID != "" {
		workID := body.WorkID
		req.WorkID = &workID
	}
	if body.Choice != nil {
		choice := app.AdditionChoice(*body.Choice)
		req.Choice = &choice
	}
	req.CandidateToken = body.CandidateToken
	res, err := s.catalog.CreateLibraryEntry(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, res)
}

// handleAddEdition adds a distinct edition under an existing work through
// the same creation use case with the add_edition choice fixed by the
// route; the work id comes from the path.
func (s *Server) handleAddEdition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	workID := r.PathValue("id")
	var body addEditionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AdditionRequest{MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor()}}
	body.apply(&req)
	choice := app.ChoiceAddEdition
	req.Choice = &choice
	req.WorkID = &workID
	req.CandidateToken = body.CandidateToken
	res, err := s.catalog.CreateLibraryEntry(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, res)
}

// handleCorrectWorkTitles applies one title correction to a work: add,
// retire and re-point the preferred display title in one atomic mutation.
func (s *Server) handleCorrectWorkTitles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		s.notFoundHandler(w, r)
		return
	}
	workID := r.PathValue("id")
	var body correctTitlesBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.TitleMutationRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		TargetID:     workID,
		Retire:       body.Retire,
		Preferred:    body.Preferred,
	}
	for _, t := range body.Add {
		req.Add = append(req.Add, t.input())
	}
	view, err := s.catalog.CorrectWorkTitles(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleUpdateEdition applies one edition metadata correction: kind,
// language and other label; only supplied fields change.
func (s *Server) handleUpdateEdition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	var body updateEditionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.UpdateEditionRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    editionID,
		Language:     body.Language,
		OtherLabel:   body.OtherLabel,
	}
	if body.Kind != nil {
		k := domain.EditionKind(*body.Kind)
		req.Kind = &k
	}
	view, err := s.catalog.UpdateEdition(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}
