package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
)

// decodeListFilter maps the /works query parameters onto the shared library
// filter. It only decodes and type-maps: a present but unparsable numeric
// parameter is an invalid argument (never silently dropped), and every
// bound, value and cursor rule is enforced by the shared service after
// decoding, so the HTTP layer owns no search rule of its own.
func decodeListFilter(q url.Values) (app.LibraryFilter, *app.AppError) {
	invalid := func(param, value string) *app.AppError {
		return app.NewAppError(app.ErrInvalidArgument, param+" is not a number: "+value)
	}
	f := app.LibraryFilter{}
	if v := q.Get("query"); v != "" {
		f.Query = &v
	}
	for _, v := range q["kind"] {
		f.Kinds = append(f.Kinds, domain.EditionKind(v))
	}
	for _, v := range q["publication"] {
		f.Publications = append(f.Publications, domain.Publication(v))
	}
	for _, v := range q["consumption"] {
		if v == "unknown" {
			f.UnknownConsumption = true
		} else {
			f.Consumptions = append(f.Consumptions, domain.Consumption(v))
		}
	}
	for _, v := range q["intention"] {
		f.Intentions = append(f.Intentions, domain.Intention(v))
	}
	for _, v := range q["tag"] {
		f.Tags = append(f.Tags, v)
	}
	if v := q.Get("min-rating"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, invalid("min-rating", v)
		}
		f.MinRating = &n
	}
	if v := q.Get("sort"); v != "" {
		f.Sort = app.LibrarySort(v)
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, invalid("limit", v)
		}
		f.Limit = n
	}
	if v := q.Get("cursor"); v != "" {
		f.Cursor = &v
	}
	return f, nil
}

// handleListWorks is GET /api/v1/works: the shared catalog search as work
// cards with the standard list payload. A non-GET method is an unknown
// route, like every other wrong method on a known path.
func (s *Server) handleListWorks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	filter, appErr := decodeListFilter(r.URL.Query())
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	payload, err := s.catalog.SearchLibrary(r.Context(), filter)
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, payload)
}

// handleGetWork is GET /api/v1/works/{id}: one work detail view.
func (s *Server) handleGetWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	view, err := s.catalog.GetWork(r.Context(), r.PathValue("id"))
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}

// handleGetEdition is GET /api/v1/editions/{id}: one edition detail view.
func (s *Server) handleGetEdition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	view, err := s.catalog.GetEdition(r.Context(), r.PathValue("id"))
	if err != nil {
		ae := catalogTyped(err)
		writeError(w, ae.Code.HTTPStatus(), ae)
		return
	}
	writeSuccess(w, view)
}
