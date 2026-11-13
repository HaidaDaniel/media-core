package httpapi

import (
	"errors"
	"net/http"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// registerCatalogRoutes wires the authenticated catalog routes. They exist
// only when a catalog service was attached; every route goes through the
// same security boundary and requires a live session, and every write
// additionally requires the per-session CSRF token. Paths without a method
// prefix dispatch on the method inside the handler so that a wrong method
// is a JSON 404 like every other unknown route, never a 405 page.
func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	if s.catalog == nil {
		return
	}
	read := func(next http.HandlerFunc) http.Handler {
		return s.securityMiddleware(s.requireSession(next))
	}
	write := func(next http.HandlerFunc) http.Handler {
		return s.securityMiddleware(s.requireMutation(next))
	}
	mux.Handle("/api/v1/works", read(s.handleListWorks))
	mux.Handle("/api/v1/works/{id}", read(s.handleGetWork))
	mux.Handle("/api/v1/editions/{id}", read(s.handleGetEdition))
	// Every write route is registered with its method pattern, so a wrong
	// method on a shared path falls through to the read handler's JSON
	// 404 (or the catch-all 404) instead of reaching the CSRF check.
	mux.Handle("POST /api/v1/works", write(s.handleCreateWork))
	mux.Handle("POST /api/v1/additions/resolve", write(s.handleResolveAddition))
	mux.Handle("POST /api/v1/works/{id}/editions", write(s.handleAddEdition))
	mux.Handle("PATCH /api/v1/works/{id}/titles", write(s.handleCorrectWorkTitles))
	mux.Handle("PATCH /api/v1/editions/{id}", write(s.handleUpdateEdition))
	mux.Handle("POST /api/v1/editions/{id}/progress", write(s.handleRecordProgress))
	mux.Handle("POST /api/v1/editions/{id}/consumption", write(s.handleSetConsumption))
	mux.Handle("GET /api/v1/editions/{id}/history", read(s.handleGetHistory))
	mux.Handle("POST /api/v1/editions/{id}/history/{record_id}/retract", write(s.handleRetractProgress))
	mux.Handle("POST /api/v1/editions/{id}/intention", write(s.handleSetIntention))
	mux.Handle("POST /api/v1/editions/{id}/ratings", write(s.handleSetRating))
	mux.Handle("POST /api/v1/editions/{id}/references", write(s.handleAddReference))
	mux.Handle("POST /api/v1/editions/{id}/references/{reference_id}/retire", write(s.handleRetireReference))
	mux.Handle("POST /api/v1/notes", write(s.handleAddNote))
	mux.Handle("PATCH /api/v1/notes/{id}", write(s.handleReviseNote))
	mux.Handle("POST /api/v1/notes/{id}/retract", write(s.handleRetractNote))
	mux.Handle("POST /api/v1/tags/assignments", write(s.handleTagAssignment))
	mux.Handle("DELETE /api/v1/tags/assignments", write(s.handleTagAssignment))
	mux.Handle("GET /api/v1/works/{id}/relations", read(s.handleWorkRelations))
	mux.Handle("GET /api/v1/editions/{id}/relations", read(s.handleEditionRelations))
	mux.Handle("GET /api/v1/activity", read(s.handleListActivity))
	mux.Handle("POST /api/v1/relations", write(s.handleAddRelation))
	mux.Handle("POST /api/v1/relations/{id}/confirm", write(s.handleConfirmRelation))
	mux.Handle("POST /api/v1/relations/{id}/revoke", write(s.handleRevokeRelation))
	mux.Handle("POST /api/v1/origins", write(s.handleRecordOrigin))
}

// requireSession rejects a request without a live session before the
// handler runs; expired and unknown sessions are unauthorized.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, appErr := s.currentSession(r); appErr != nil {
			writeError(w, http.StatusUnauthorized, appErr)
			return
		}
		next(w, r)
	}
}

// requireMutation is the session plus CSRF boundary for catalog writes: a
// missing or stale session is unauthorized, and a missing or mismatched
// per-session CSRF token is forbidden.
func (s *Server) requireMutation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, appErr := s.currentSession(r)
		if appErr != nil {
			writeError(w, http.StatusUnauthorized, appErr)
			return
		}
		if appErr := requireCSRF(r, sess); appErr != nil {
			writeError(w, http.StatusForbidden, appErr)
			return
		}
		next(w, r)
	}
}

// catalogTyped maps a catalog service failure to a stable-coded application
// error; service failures already carry their code, anything else is a
// storage error.
func catalogTyped(err error) *app.AppError {
	var ae *app.AppError
	if errors.As(err, &ae) {
		return ae
	}
	return app.NewAppError(app.ErrStorageError, err.Error())
}
