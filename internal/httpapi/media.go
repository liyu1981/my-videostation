package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/liyu1981/myvideostation/internal/store"
)

var mimeByExt = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".flv":  "video/x-flv",
	".ogv":  "video/ogg",
	".wmv":  "video/x-ms-wmv",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
}

// handleStream serves a file with byte-range support (direct play).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("fileId")
	id, ok := parseInt64(raw)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid file id")
		return
	}
	file, err := s.store.FileByID(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	// ADR 0006: serve a completed background variant instead of the source file.
	if raw := r.URL.Query().Get("variant"); raw != "" {
		variantID, ok := parseInt64(raw)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid variant id")
			return
		}
		v, err := s.store.VariantByID(r.Context(), variantID)
		if err != nil || v.FileID != file.ID || v.State != "ready" {
			writeError(w, http.StatusNotFound, "not_found", "transcoded variant not found")
			return
		}
		if st, err := os.Stat(v.Path); err != nil || st.IsDir() {
			writeError(w, http.StatusGone, "variant_missing", "transcoded variant is no longer available")
			return
		}
		_ = s.store.TouchVariant(r.Context(), v.ID)
		file.Path = v.Path
		file.Container = "mp4"
	}
	s.serveMediaFile(w, r, file)
}

// serveMediaFile writes one physical file with byte-range support. Shared by the
// SPA API and the Jellyfin adapter.
func (s *Server) serveMediaFile(w http.ResponseWriter, r *http.Request, file *store.File) {
	f, err := os.Open(file.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusGone, "file_missing", "file is no longer available")
			return
		}
		s.fail(w, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Path))
	if ct := mimeByExt[ext]; ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if r.URL.Query().Get("cast") != "" {
		// Cast receivers fetch media without the session cookie.
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, filepath.Base(file.Path), st.ModTime(), f)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context(), intQuery(r, "limit", 20))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid job id")
		return
	}
	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid job id")
		return
	}
	if _, err := s.store.GetJob(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	logs, err := s.store.JobLogs(r.Context(), id, intQuery(r, "limit", 5000))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid job id")
		return
	}
	if _, err := s.store.GetJob(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.DeleteJob(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearFinishedJobs(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.ClearFinishedJobs(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("finished jobs cleared", "count", n)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

func parseInt64(s string) (int64, bool) {
	var n int64
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n > 1<<62 {
			return 0, false
		}
	}
	return n, n > 0
}
