package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/artwork"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// Artwork routes are session-gated. Reads use the session, writes add CSRF.
// Cache clearing is a write because it removes files, even though it never
// touches a user record.

type artworkSearchBody struct {
	Title        string   `json:"title"`
	Kind         string   `json:"kind"`
	Alternatives []string `json:"alternatives"`
	References   []string `json:"references"`
}

type artworkSelectBody struct {
	WorkID       string `json:"work_id"`
	EditionID    string `json:"edition_id"`
	PreviewToken string `json:"preview_token"`
	Replace      bool   `json:"replace"`
}

type artworkFindBody struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func artworkError(err error) *app.AppError {
	switch {
	case errors.Is(err, artwork.ErrDisabled):
		return app.NewAppError(app.ErrConfigError, "artwork discovery is disabled")
	case errors.Is(err, artwork.ErrInvalidInput):
		return app.NewAppError(app.ErrInvalidArgument, err.Error())
	case errors.Is(err, artwork.ErrUnknownPreview), errors.Is(err, store.ErrArtworkNotFound):
		return app.NewAppError(app.ErrNotFound, "artwork candidate or selection not found")
	case errors.Is(err, store.ErrArtworkExists):
		return app.NewAppError(app.ErrConflict, "a user-selected cover already exists; pass replace to change it")
	case errors.Is(err, artwork.ErrUpstream):
		return app.NewAppError(app.ErrInternalError, "cover providers are unavailable")
	default:
		return app.NewAppError(app.ErrStorageError, "artwork operation failed")
	}
}

func (s *Server) requireArtwork() *app.AppError {
	if s.artwork == nil {
		return app.NewAppError(app.ErrConfigError, "artwork discovery is not configured")
	}
	if !s.cfg.Artwork {
		return app.NewAppError(app.ErrConfigError, "artwork discovery is disabled")
	}
	return nil
}

func (s *Server) handleArtworkProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeSuccess(w, map[string]any{"providers": s.artwork.Providers()})
}

func (s *Server) handleArtworkSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var body artworkSearchBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, err.Error()))
		return
	}
	result, err := s.artwork.Search(r.Context(), artwork.SearchInput{
		Title: body.Title, Kind: artwork.Kind(body.Kind),
		Alternatives: body.Alternatives, References: body.References,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, artwork.ErrInvalidInput) {
			status = http.StatusBadRequest
		}
		writeError(w, status, artworkError(err))
		return
	}
	writeSuccess(w, map[string]any{"candidates": result})
}

func (s *Server) handleArtworkSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var body artworkSelectBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, err.Error()))
		return
	}
	record, err := s.artwork.Select(r.Context(), body.WorkID, body.EditionID, body.PreviewToken, body.Replace)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrArtworkExists) {
			status = http.StatusConflict
		} else if errors.Is(err, artwork.ErrUnknownPreview) || errors.Is(err, store.ErrArtworkNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, artworkError(err))
		return
	}
	writeSuccess(w, map[string]any{"artwork": app.ArtworkViewFromStore(record)})
}

func (s *Server) handleArtworkFindMissing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var body artworkFindBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, err.Error()))
		return
	}
	if body.Limit == 0 {
		body.Limit = 5
	}
	items, total, err := s.artwork.FindMissing(r.Context(), body.Offset, body.Limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, artworkError(err))
		return
	}
	next := body.Offset + len(items)
	if next > total {
		next = total
	}
	writeSuccess(w, map[string]any{"items": items, "next_offset": next, "total_missing": total})
}

func (s *Server) handleArtworkPreviewImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	data, mime, ok := s.artwork.Preview(r.PathValue("token"))
	if !ok {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "preview not found"))
		return
	}
	writeArtworkImage(w, data, mime)
}

func (s *Server) handleArtworkImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	id := r.PathValue("id")
	if _, err := strconv.Atoi(id); err == nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "invalid artwork id"))
		return
	}
	data, mime, err := s.artwork.ArtworkImage(r.Context(), id)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, artwork.ErrCacheRef) {
			status = http.StatusBadRequest
		}
		writeError(w, status, artworkError(err))
		return
	}
	writeArtworkImage(w, data, mime)
}

func (s *Server) handleArtworkClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.notFoundHandler(w, r)
		return
	}
	if err := s.requireArtwork(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	removed, bytes, err := s.artwork.ClearCache()
	if err != nil {
		writeError(w, http.StatusInternalServerError, artworkError(err))
		return
	}
	writeSuccess(w, map[string]any{"removed": removed, "bytes": bytes})
}

func writeArtworkImage(w http.ResponseWriter, data []byte, mime string) {
	if mime == "" {
		mime = "image/jpeg"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
