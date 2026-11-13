package httpapi

import (
	"net/http"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// handleListActivity projects the bounded recent personal activity over
// every history source; the types, limit and cursor come from the query
// string and are validated by the shared service, so the transport owns no
// feed rule of its own.
func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	q := app.ActivityQuery{}
	for _, v := range r.URL.Query()["types"] {
		q.Types = append(q.Types, app.ActivityType(v))
	}
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
	payload, err := s.catalog.ListActivity(r.Context(), q)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, payload)
}
