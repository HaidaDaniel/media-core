package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// atoiParam parses one integer query parameter.
func atoiParam(v string) (int, error) { return strconv.Atoi(v) }

// bindingBody is the POST /releases/bindings and /releases/bindings/rebind
// payload: the explicit map of one confirmed publication to one edition.
type bindingBody struct {
	RequestID     string `json:"request_id"`
	Namespace     string `json:"namespace"`
	PublicationID string `json:"publication_id"`
	EditionID     string `json:"edition_id"`
}

func (b bindingBody) req() app.BindReleaseRequest {
	return app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: b.RequestID, Actor: trustedActor()},
		Namespace:     b.Namespace,
		PublicationID: b.PublicationID,
		EditionID:     b.EditionID,
	}
}

// revokeBindingBody is the DELETE /releases/bindings payload.
type revokeBindingBody struct {
	RequestID     string `json:"request_id"`
	Namespace     string `json:"namespace"`
	PublicationID string `json:"publication_id"`
}

// handleImportReleases accepts a bounded Release Watcher version 1.0 envelope
// and stages it as immutable pending records through the shared import
// service. The body is the envelope itself (the bounded JSON batch); the
// optional namespace is a query parameter defaulting to the stable producer
// name. A malformed envelope or unsupported major is an invalid argument that
// writes nothing; a valid envelope returns the per-item results (a partial
// import reports its conflicts in the body, not as an error).
func (s *Server) handleImportReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "reading the import envelope: "+err.Error()))
		return
	}
	req := app.ImportRequest{
		RequestID: "http-import-releases",
		Namespace: r.URL.Query().Get("namespace"),
		Actor:     trustedActor(),
		Raw:       raw,
		MaxBytes:  MaxRequestBody,
	}
	res, err := s.catalog.ImportEvents(r.Context(), req)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, res)
}

// handleBindRelease explicitly maps one confirmed publication to an edition.
// A publication already bound to a different edition is a conflict; the same
// publication bound to the same edition replays the existing binding.
func (s *Server) handleBindRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body bindingBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	view, err := s.catalog.BindRelease(r.Context(), body.req())
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRebindRelease explicitly corrects a publication's binding: it revokes
// the current active binding and creates its replacement in one step.
func (s *Server) handleRebindRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	var body bindingBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	view, err := s.catalog.RebindRelease(r.Context(), body.req())
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleRevokeRelease revokes one publication's active binding, un-projecting
// its events while keeping the binding row and the raw event history.
func (s *Server) handleRevokeRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.notFoundHandler(w, r)
		return
	}
	var body revokeBindingBody
	if appErr := decodeMutationBody(r, &body); appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	req := app.RevokeReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: body.RequestID, Actor: trustedActor()},
		Namespace:     body.Namespace,
		PublicationID: body.PublicationID,
	}
	if err := s.catalog.RevokeRelease(r.Context(), req); err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, map[string]any{"status": "revoked", "namespace": req.Namespace, "publication_id": req.PublicationID})
}

// handleListReleases projects the bounded release history of one edition.
func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	editionID := r.PathValue("id")
	q := app.ReleaseQuery{EditionID: editionID}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := atoiParam(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "limit is not a number: "+v))
			return
		}
		q.Limit = n
	}
	if v := r.URL.Query().Get("cursor"); v != "" {
		q.Cursor = &v
	}
	payload, err := s.catalog.ListReleases(r.Context(), q)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, payload)
}

// handlePendingImports lists the bounded pending import queue.
func (s *Server) handlePendingImports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	q := app.PendingImportQuery{}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := atoiParam(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "limit is not a number: "+v))
			return
		}
		q.Limit = n
	}
	if v := r.URL.Query().Get("cursor"); v != "" {
		q.Cursor = &v
	}
	payload, err := s.catalog.PendingImports(r.Context(), q)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, payload)
}

// handleGetReleaseBinding returns the active binding of one publication, or
// an explicit empty object when there is none.
func (s *Server) handleGetReleaseBinding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	namespace := r.URL.Query().Get("namespace")
	publicationID := r.URL.Query().Get("publication")
	if publicationID == "" {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "publication query parameter is required"))
		return
	}
	binding, err := s.catalog.GetReleaseBinding(r.Context(), namespace, publicationID)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, map[string]any{"active": binding})
}
