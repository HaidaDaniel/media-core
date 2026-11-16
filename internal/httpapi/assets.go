package httpapi

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// Cache policy for the staged production build: hashed build assets are
// content-addressed and never change, so they are cached forever; the
// shell and every non-hashed file revalidate on each load.
const (
	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNoCache   = "no-cache"
)

// WithAssets attaches the filesystem served for every non-API path and
// finalizes the route table so the catch-all serves the SPA instead of the
// JSON 404 the session-only build serves. It must be called after
// WithCatalog (when used) because the catalog routes are only known then;
// the table is built once here and Serve uses it as-is. Without it the
// server keeps the session-only table and every non-API path is a JSON
// 404. Tests use it to inject a synthetic build.
func (s *Server) WithAssets(fsys fs.FS) *Server {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/session", s.securityMiddleware(http.HandlerFunc(s.sessionHandler)))
	s.registerCatalogRoutes(mux)
	serve := func(w http.ResponseWriter, r *http.Request) { serveAssets(fsys, w, r) }
	mux.Handle("/", s.securityMiddleware(http.HandlerFunc(serve)))
	s.mux = mux
	return s
}

// serveAssets serves the production UI same-origin. The SPA shell answers
// every non-API path that does not name a real file, which is what lets the
// client router own the UI routes; a miss under /api/ is always the JSON
// 404, never index.html. Only GET and HEAD read content; every other
// method on an unregistered route keeps the JSON 404 the session-only
// catch-all served, so the wire error contract is unchanged.
func serveAssets(fsys fs.FS, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	p := r.URL.Path
	// A path that changes under cleaning carries dot segments or doubled
	// slashes (including percent-encoded ones): it is a miss, never a
	// fetch, so a cleaned spelling cannot smuggle a file through.
	if path.Clean(p) != p {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	if p == "/api" || strings.HasPrefix(p, "/api/") {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	rel := strings.TrimPrefix(p, "/")
	if rel == "" {
		rel = "index.html"
	}
	if f, err := fsys.Open(rel); err == nil {
		defer f.Close()
		serveAssetFile(fsys, w, r, f, rel)
		return
	}
	serveIndexFile(fsys, w, r)
}

// serveAssetFile writes one staged file with the cache policy of its
// class. Directories are never listed: a path naming one is a miss.
func serveAssetFile(_ fs.FS, w http.ResponseWriter, r *http.Request, f fs.File, rel string) {
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	if strings.HasPrefix(rel, "assets/") {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoCache)
	}
	writeAssetContent(w, r, info.Name(), info, f)
}

// serveIndexFile writes the SPA shell for the root and every client-side
// route. Without a shell there is nothing to fall back to, so the miss
// stays a JSON 404.
func serveIndexFile(fsys fs.FS, w http.ResponseWriter, r *http.Request) {
	f, err := fsys.Open("index.html")
	if err != nil {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
		return
	}
	w.Header().Set("Cache-Control", cacheNoCache)
	writeAssetContent(w, r, "index.html", info, f)
}

// writeAssetContent streams one staged file through http.ServeContent,
// which supplies the content type, Last-Modified and Range support.
// Staged files that are seekable stream directly; the rare non-seekable
// source is read whole (static UI assets are small).
func writeAssetContent(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, f fs.File) {
	if seeker, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, info.ModTime(), seeker)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
}
