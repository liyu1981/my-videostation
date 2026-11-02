package httpapi

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/liyu1981/myvideostation/internal/metadata"
	"github.com/liyu1981/myvideostation/internal/redact"
)

// settingsDTO is the redacted settings payload.
type settingsDTO struct {
	Metadata struct {
		APIKeySet bool   `json:"apiKeySet"`
		Language  string `json:"language"`
		Region    string `json:"region"`
	} `json:"metadata"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.store.Settings(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	var out settingsDTO
	out.Metadata.APIKeySet = strings.TrimSpace(all[metadata.SettingAPIKey]) != ""
	out.Metadata.Language = valueOr(all[metadata.SettingLanguage], "en-US")
	out.Metadata.Region = valueOr(all[metadata.SettingRegion], "US")
	writeJSON(w, http.StatusOK, out)
}

type settingsInput struct {
	TMDBAPIKey *string `json:"tmdbApiKey"`
	Language   *string `json:"language"`
	Region     *string `json:"region"`
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	values := map[string]string{}
	if in.TMDBAPIKey != nil {
		values[metadata.SettingAPIKey] = strings.TrimSpace(*in.TMDBAPIKey)
	}
	if in.Language != nil {
		values[metadata.SettingLanguage] = strings.TrimSpace(*in.Language)
	}
	if in.Region != nil {
		values[metadata.SettingRegion] = strings.TrimSpace(*in.Region)
	}
	if len(values) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "nothing to update")
		return
	}
	if err := s.store.PutSettings(r.Context(), values); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("settings updated", "keys", len(values))
	s.handleGetSettings(w, r)
}

func (s *Server) handleEnrichLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	if _, err := s.store.GetLibrary(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	job, err := s.jobs.Enqueue(r.Context(), "enrich", map[string]int64{"libraryId": id})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleMatches(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	if s.enricher == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata_not_configured", "metadata provider is not configured")
		return
	}
	query := r.URL.Query().Get("q")
	candidates, err := s.enricher.Search(r.Context(), id, query, intQuery(r, "year", 0), r.URL.Query().Get("language"), r.URL.Query().Get("provider"))
	if err != nil {
		s.failMetadata(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

type matchInput struct {
	ProviderID string `json:"providerId"`
	Provider   string `json:"provider"`
	Language   string `json:"language"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in matchInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(in.ProviderID) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "providerId is required")
		return
	}
	if s.enricher == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata_not_configured", "metadata provider is not configured")
		return
	}
	if err := s.enricher.Match(r.Context(), id, in.ProviderID, in.Language, in.Season, in.Episode, in.Provider); err != nil {
		s.failMetadata(w, err)
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleRefreshMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	if s.enricher == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata_not_configured", "metadata provider is not configured")
		return
	}
	if err := s.enricher.Refresh(r.Context(), id, r.URL.Query().Get("language"), r.URL.Query().Get("provider")); err != nil {
		s.failMetadata(w, err)
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type artInput struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

func (s *Server) handleSetArt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in artInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.Kind != "poster" && in.Kind != "backdrop" {
		writeError(w, http.StatusBadRequest, "bad_request", "kind must be poster or backdrop")
		return
	}
	url := strings.TrimSpace(in.URL)
	if url == "" {
		// Clearing the artwork does not need a metadata provider.
		if err := s.store.SetArt(r.Context(), id, in.Kind, ""); err != nil {
			s.fail(w, err)
			return
		}
	} else {
		if s.enricher == nil {
			writeError(w, http.StatusServiceUnavailable, "metadata_not_configured", "metadata provider is not configured")
			return
		}
		if err := s.enricher.SetArtFromURL(r.Context(), id, in.Kind, url); err != nil {
			s.fail(w, err)
			return
		}
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) failMetadata(w http.ResponseWriter, err error) {
	msg := redact.String(err.Error())
	switch {
	case errors.Is(err, metadata.ErrNotConfigured):
		writeError(w, http.StatusBadRequest, "metadata_not_configured", "set a TMDB API key in Settings first")
	case errors.Is(err, metadata.ErrNoMatch):
		writeError(w, http.StatusNotFound, "no_match", "no confident metadata match found")
	case errors.Is(err, metadata.ErrUnsupportedKind):
		writeError(w, http.StatusBadRequest, "unsupported_kind", "metadata search is only available for movies and TV shows")
	case strings.Contains(msg, "status 401"), strings.Contains(strings.ToLower(msg), "invalid api key"):
		writeError(w, http.StatusBadGateway, "metadata_auth_failed", "The metadata provider rejected the API key — check Settings")
	case strings.Contains(msg, "status 429"):
		writeError(w, http.StatusTooManyRequests, "metadata_rate_limited", "TMDB rate limit reached — try again shortly")
	case strings.Contains(msg, "status 404"):
		writeError(w, http.StatusNotFound, "not_found", "the provider has no record with that id")
	default:
		s.log.Error("metadata request failed", "err", msg)
		detail := msg
		if len(detail) > 200 {
			detail = detail[:200]
		}
		writeError(w, http.StatusBadGateway, "metadata_unavailable", "metadata provider request failed: "+detail)
	}
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if s.enricher == nil {
		writeJSON(w, http.StatusOK, map[string]any{"providers": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.enricher.ProviderInfos(r.Context())})
}

// handleUploadArt accepts a multipart image upload and stores it in the artwork
// cache. kind is poster|backdrop; the file must be JPEG, PNG, WebP or GIF.
func (s *Server) handleUploadArt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	if _, err := s.store.GetItem(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart form: "+err.Error())
		return
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind != "poster" && kind != "backdrop" {
		writeError(w, http.StatusBadRequest, "bad_request", "kind must be poster or backdrop")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 20<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read upload: "+err.Error())
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "uploaded file is empty")
		return
	}
	var ext string
	switch http.DetectContentType(data) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	default:
		writeError(w, http.StatusBadRequest, "unsupported_image", "only JPEG, PNG, WebP or GIF images are supported")
		return
	}
	dir := filepath.Join(s.cacheDir, "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.fail(w, err)
		return
	}
	sum := sha1.Sum(data)
	path := filepath.Join(dir, "upload-"+hex.EncodeToString(sum[:])+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.SetArt(r.Context(), id, kind, path); err != nil {
		s.fail(w, err)
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
