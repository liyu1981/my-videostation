package httpapi

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/liyu1981/myvideostation/internal/artwork"
)

// artworkOptions resolves the effective artwork settings for a request.
func (s *Server) artworkOptions(r *http.Request) artwork.Options {
	all, err := s.store.Settings(r.Context())
	if err != nil {
		s.log.Warn("artwork settings read failed", "err", err)
		return artwork.DefaultOptions()
	}
	return artwork.FromSettings(all)
}

// serveArt writes an artwork file, transparently replacing it with a cached
// thumbnail variant unless `?original=1` is requested or thumbnails are off.
func (s *Server) serveArt(w http.ResponseWriter, r *http.Request, kind, path string) {
	opts := s.artworkOptions(r)
	if opts.Thumbnails && r.URL.Query().Get("original") == "" && artwork.Supported(path) {
		width := artwork.WidthFor(opts, kind)
		if thumb, err := artwork.Variant(s.cacheDir, path, width, opts.Quality); err == nil {
			path = thumb
		} else {
			s.log.Debug("thumbnail generation failed", "path", filepath.Base(path), "err", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if ct := mimeByExt[filepath.Ext(path)]; ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

// handleThumbnailStats reports the disposable thumbnail cache size.
func (s *Server) handleThumbnailStats(w http.ResponseWriter, r *http.Request) {
	files, bytes := artwork.Stats(s.cacheDir)
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "bytes": bytes})
}

// handleClearThumbnails deletes every cached variant.
func (s *Server) handleClearThumbnails(w http.ResponseWriter, r *http.Request) {
	files, bytes, err := artwork.Clear(s.cacheDir)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("thumbnail cache cleared", "files", files, "bytes", bytes)
	s.audit(r, "artwork.thumbnails.clear", "cache", "")
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "bytes": bytes})
}

// handleRebuildThumbnails clears the cache and pre-generates variants for every
// item that has artwork.
func (s *Server) handleRebuildThumbnails(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		writeError(w, http.StatusServiceUnavailable, "jobs_unavailable", "job queue is not configured")
		return
	}
	if _, _, err := artwork.Clear(s.cacheDir); err != nil {
		s.fail(w, err)
		return
	}
	job, err := s.jobs.Enqueue(r.Context(), "thumbnails", map[string]any{})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "artwork.thumbnails.rebuild", "cache", "")
	writeJSON(w, http.StatusAccepted, job)
}
