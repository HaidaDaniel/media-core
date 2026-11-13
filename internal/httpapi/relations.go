package httpapi

import (
	"net/http"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
)

// addRelationBody is the POST /relations payload: the typed, scoped,
// provenance-bearing assertion with its explicit state; the shared service
// owns every graph, scope and cycle rule.
type addRelationBody struct {
	RequestID        string   `json:"request_id"`
	ExpectedRevision *int64   `json:"expected_revision"`
	Scope            string   `json:"scope"`
	Type             string   `json:"type"`
	FromID           string   `json:"from_id"`
	ToID             string   `json:"to_id"`
	State            string   `json:"state"`
	Provenance       string   `json:"provenance"`
	Confidence       *float64 `json:"confidence"`
	ReplacesID       *string  `json:"replaces_id"`
}

// relationActionBody is the POST /relations/{id}/confirm and /revoke
// payload; the relation id comes from the path and the expected revision
// is the canonical from entity's current revision.
type relationActionBody struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

// recordOriginBody is the POST /origins payload: one explicit origin claim
// on an edition; unknown is an explicit kind, never an inference.
type recordOriginBody struct {
	RequestID        string  `json:"request_id"`
	ExpectedRevision *int64  `json:"expected_revision"`
	EditionID        string  `json:"edition_id"`
	Kind             string  `json:"kind"`
	State            string  `json:"state"`
	Provenance       string  `json:"provenance"`
	ReplacesID       *string `json:"replaces_id"`
}

// writeRelated decodes the optional bounded traversal depth and delegates
// the both-directions traversal to the shared service; a present but
// unparsable depth is an invalid argument, an absent one the service
// default.
func (s *Server) writeRelated(w http.ResponseWriter, r *http.Request, scope domain.RelationScope, entityID string) {
	depth := 0
	if v := r.URL.Query().Get("depth"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "depth is not a number: "+v))
			return
		}
		depth = n
	}
	view, err := s.catalog.Related(r.Context(), scope, entityID, depth)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleWorkRelations traverses the non-revoked work-scope relations of one
// work in both directions; the edge direction is never flipped.
func (s *Server) handleWorkRelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	s.writeRelated(w, r, domain.ScopeWork, r.PathValue("id"))
}

// handleEditionRelations traverses the non-revoked edition-scope relations
// of one edition in both directions; the edge direction is never flipped.
func (s *Server) handleEditionRelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	s.writeRelated(w, r, domain.ScopeEdition, r.PathValue("id"))
}

// handleAddRelation asserts one typed, scoped, provenance-bearing relation
// and returns the relation view of its canonical from entity.
func (s *Server) handleAddRelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body addRelationBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.AddRelationRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		Scope:        domain.RelationScope(body.Scope),
		Type:         domain.RelationType(body.Type),
		FromID:       body.FromID,
		ToID:         body.ToID,
		State:        domain.ClaimState(body.State),
		Provenance:   body.Provenance,
		Confidence:   body.Confidence,
		ReplacesID:   body.ReplacesID,
	}
	view, err := s.catalog.AddRelation(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleConfirmRelation explicitly confirms one proposed relation; a
// confidence threshold never confirms, only this explicit route.
func (s *Server) handleConfirmRelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	relationID := r.PathValue("id")
	var body relationActionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.ConfirmRelationRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		RelationID:   relationID,
	}
	view, err := s.catalog.ConfirmRelation(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRevokeRelation revokes one active relation; the row keeps its
// history, never an erase.
func (s *Server) handleRevokeRelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	relationID := r.PathValue("id")
	var body relationActionBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RevokeRelationRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		RelationID:   relationID,
	}
	view, err := s.catalog.RevokeRelation(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRecordOrigin asserts one explicit origin claim on an edition and
// returns the origin view.
func (s *Server) handleRecordOrigin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body recordOriginBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RecordOriginRequest{
		MutationBase: app.MutationBase{RequestID: body.RequestID, Actor: trustedActor(), ExpectedRevision: body.ExpectedRevision},
		EditionID:    body.EditionID,
		Kind:         domain.OriginKind(body.Kind),
		State:        domain.ClaimState(body.State),
		Provenance:   body.Provenance,
		ReplacesID:   body.ReplacesID,
	}
	view, err := s.catalog.RecordOrigin(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}
