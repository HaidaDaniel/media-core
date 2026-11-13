package httpapi

import (
	"errors"
	"net/http"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// registerCatalogRoutes wires the authenticated catalog read routes. They
// exist only when a catalog service was attached; every route goes through
// the same security boundary and requires a live session. Paths without a
// method prefix dispatch on the method inside the handler so that a wrong
// method is a JSON 404 like every other unknown route, never a 405 page.
func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	if s.catalog == nil {
		return
	}
	read := func(next http.HandlerFunc) http.Handler {
		return s.securityMiddleware(s.requireSession(next))
	}
	mux.Handle("/api/v1/works", read(s.handleListWorks))
	mux.Handle("/api/v1/works/{id}", read(s.handleGetWork))
	mux.Handle("/api/v1/editions/{id}", read(s.handleGetEdition))
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
