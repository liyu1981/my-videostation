// Package httpapi exposes the versioned JSON API and the embedded web UI.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liyu1981/myvideostation/internal/buildinfo"
	"github.com/liyu1981/myvideostation/internal/enrich"
	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/store"
	"github.com/liyu1981/myvideostation/internal/web"
)

// Server wires the API handlers.
type Server struct {
	store    *store.Store
	jobs     *jobs.Manager
	log      *slog.Logger
	cors     []string
	version  string
	enricher *enrich.Service
	ffmpeg   string
	cacheDir string
	dataDir  string
	restart  func()

	castMu  sync.Mutex
	castKey []byte

	maintenance atomic.Bool
}

// Options configures the HTTP server.
type Options struct {
	Store      *store.Store
	Jobs       *jobs.Manager
	Logger     *slog.Logger
	CORSOrigin []string
	Version    string
	Enricher   *enrich.Service
	FFmpeg     string
	CacheDir   string
	DataDir    string
	Restart    func()
}

// New creates the API server.
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Version == "" {
		o.Version = buildinfo.Version
	}
	return &Server{store: o.Store, jobs: o.Jobs, log: o.Logger, cors: o.CORSOrigin, version: o.Version,
		enricher: o.Enricher, ffmpeg: o.FFmpeg, cacheDir: o.CacheDir, dataDir: o.DataDir, restart: o.Restart}
}

// Handler builds the root HTTP handler (API + UI).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/config", s.handleConfig)
	mux.HandleFunc("GET /api/v1/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/v1/auth/setup", s.handleAuthSetup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleAuthLogin)
	mux.HandleFunc("GET /api/v1/auth/me", s.auth(s.handleAuthMe))
	mux.HandleFunc("POST /api/v1/auth/logout", s.auth(s.handleAuthLogout))
	mux.HandleFunc("POST /api/v1/auth/password", s.auth(s.handleChangePassword))

	mux.HandleFunc("GET /api/v1/settings", s.admin(s.handleGetSettings))
	mux.HandleFunc("PUT /api/v1/settings", s.admin(s.handlePutSettings))
	mux.HandleFunc("GET /api/v1/providers", s.admin(s.handleProviders))
	mux.HandleFunc("GET /api/v1/users", s.admin(s.handleListUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.handleCreateUser))
	mux.HandleFunc("PATCH /api/v1/users/{id}", s.admin(s.handleUpdateUser))
	mux.HandleFunc("DELETE /api/v1/users/{id}", s.admin(s.handleDeleteUser))
	mux.HandleFunc("GET /api/v1/users/{id}/libraries", s.admin(s.handleUserLibraries))
	mux.HandleFunc("PUT /api/v1/users/{id}/libraries", s.admin(s.handleUserLibraries))
	mux.HandleFunc("GET /api/v1/audit", s.admin(s.handleAudit))
	mux.HandleFunc("POST /api/v1/cast/dlna/discover", s.auth(s.handleCastDLNADiscover))
	mux.HandleFunc("POST /api/v1/cast/dlna/play", s.auth(s.handleCastDLNAPlay))
	mux.HandleFunc("GET /api/v1/backup/status", s.handleBackupStatus)
	mux.HandleFunc("GET /api/v1/backup/exports", s.admin(s.handleBackupExports))
	mux.HandleFunc("POST /api/v1/backup/export", s.admin(s.handleBackupExport))
	mux.HandleFunc("GET /api/v1/backup/imports", s.admin(s.handleBackupImports))
	mux.HandleFunc("POST /api/v1/backup/import", s.admin(s.handleBackupImport))

	mux.HandleFunc("GET /api/v1/libraries", s.auth(s.handleListLibraries))
	mux.HandleFunc("POST /api/v1/libraries", s.admin(s.handleCreateLibrary))
	mux.HandleFunc("GET /api/v1/libraries/{id}", s.auth(s.handleGetLibrary))
	mux.HandleFunc("PATCH /api/v1/libraries/{id}", s.admin(s.handleUpdateLibrary))
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.admin(s.handleDeleteLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.admin(s.handleScanLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/enrich", s.admin(s.handleEnrichLibrary))
	mux.HandleFunc("GET /api/v1/libraries/{id}/facets", s.auth(s.handleFacets))
	mux.HandleFunc("GET /api/v1/libraries/{id}/folders", s.auth(s.handleFolders))

	mux.HandleFunc("GET /api/v1/jobs", s.admin(s.handleListJobs))
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.admin(s.handleGetJob))
	mux.HandleFunc("GET /api/v1/jobs/{id}/logs", s.admin(s.handleJobLogs))
	mux.HandleFunc("DELETE /api/v1/jobs/{id}", s.admin(s.handleDeleteJob))
	mux.HandleFunc("DELETE /api/v1/jobs", s.admin(s.handleClearFinishedJobs))

	mux.HandleFunc("GET /api/v1/collections", s.auth(s.handleListCollections))
	mux.HandleFunc("POST /api/v1/collections", s.auth(s.handleCreateCollection))
	mux.HandleFunc("PATCH /api/v1/collections/{id}", s.auth(s.handleRenameCollection))
	mux.HandleFunc("DELETE /api/v1/collections/{id}", s.auth(s.handleDeleteCollection))
	mux.HandleFunc("GET /api/v1/collections/{id}/items", s.auth(s.handleCollectionItems))
	mux.HandleFunc("POST /api/v1/collections/{id}/items", s.auth(s.handleAddCollectionItems))
	mux.HandleFunc("DELETE /api/v1/collections/{id}/items/{itemId}", s.auth(s.handleRemoveCollectionItem))
	mux.HandleFunc("POST /api/v1/items/batch", s.auth(s.handleBatch))

	mux.HandleFunc("GET /api/v1/fs", s.admin(s.handleFSList))

	mux.HandleFunc("GET /api/v1/home", s.auth(s.handleHome))
	mux.HandleFunc("GET /api/v1/browse", s.auth(s.handleBrowse))
	mux.HandleFunc("GET /api/v1/search", s.auth(s.handleSearch))

	mux.HandleFunc("GET /api/v1/items/{id}", s.auth(s.handleGetItem))
	mux.HandleFunc("PATCH /api/v1/items/{id}", s.admin(s.handlePatchItem))
	mux.HandleFunc("GET /api/v1/items/{id}/children", s.auth(s.handleItemChildren))
	mux.HandleFunc("GET /api/v1/items/{id}/neighbors", s.auth(s.handleItemNeighbors))
	mux.HandleFunc("GET /api/v1/items/{id}/matches", s.admin(s.handleMatches))
	mux.HandleFunc("GET /api/v1/items/{id}/duplicate", s.admin(s.handleItemDuplicate))
	mux.HandleFunc("POST /api/v1/items/{id}/merge", s.admin(s.handleItemMerge))
	mux.HandleFunc("POST /api/v1/items/{id}/keep-separate", s.admin(s.handleKeepSeparate))
	mux.HandleFunc("POST /api/v1/items/{id}/apply-local", s.admin(s.handleApplyLocal))
	mux.HandleFunc("POST /api/v1/items/{id}/match", s.admin(s.handleMatch))
	mux.HandleFunc("POST /api/v1/items/{id}/refresh", s.admin(s.handleRefreshMetadata))
	mux.HandleFunc("POST /api/v1/items/{id}/art", s.admin(s.handleSetArt))
	mux.HandleFunc("POST /api/v1/items/{id}/art/upload", s.admin(s.handleUploadArt))
	mux.HandleFunc("POST /api/v1/items/{id}/progress", s.auth(s.handleProgress))
	mux.HandleFunc("GET /api/v1/items/{id}/poster", s.auth(s.handleArt("poster")))
	mux.HandleFunc("GET /api/v1/items/{id}/backdrop", s.auth(s.handleArt("backdrop")))

	mux.HandleFunc("GET /api/v1/media/{fileId}", s.mediaAuth(s.handleStream))
	mux.HandleFunc("GET /api/v1/media/{fileId}/subtitle", s.auth(s.handleSubtitle))
	mux.HandleFunc("GET /api/v1/items/{id}/playback", s.auth(s.handlePlayback))

	mux.Handle("/", web.Handler())

	return s.withMiddleware(mux)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if origin := r.Header.Get("Origin"); origin != "" && s.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")

		if s.maintenance.Load() && strings.HasPrefix(r.URL.Path, "/api/") &&
			r.URL.Path != "/api/v1/health" && r.URL.Path != "/api/v1/config" &&
			r.URL.Path != "/api/v1/backup/status" {
			writeError(w, http.StatusServiceUnavailable, "maintenance", "import in progress; the server is restarting")
			return
		}

		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "path", r.URL.Path, "recover", rec)
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.log.Debug("request", "method", r.Method, "path", r.URL.Path,
				"duration", time.Since(start).Round(time.Millisecond).String())
		}
	})
}

func (s *Server) originAllowed(origin string) bool {
	for _, o := range s.cors {
		if o == "*" || strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

// ---- handlers -------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.version,
		"time":    time.Now().UTC(),
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": buildinfo.Version,
		"commit":  buildinfo.Commit,
		"built":   buildinfo.Date,
		"name":    "MyVideoStation",
	})
}

// ---- helpers --------------------------------------------------------------

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Response is already committed; nothing sensible to do.
		return
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = msg
	writeJSON(w, status, body)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case store.IsInvalid(err):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		s.log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	v := r.PathValue("id")
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func int64Query(r *http.Request, key string) int64 {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// itemDTO is the API representation of a media item.
type itemDTO struct {
	store.Item
	PosterURL   string `json:"posterUrl,omitempty"`
	BackdropURL string `json:"backdropUrl,omitempty"`
}

type fileDTO struct {
	store.File
	StreamURL string `json:"streamUrl"`
}

type itemDetailDTO struct {
	itemDTO
	Genres          []string       `json:"genres"`
	People          []store.Person `json:"people"`
	Files           []fileDTO      `json:"files"`
	Locks           []string       `json:"locks"`
	Children        []itemDTO      `json:"children"`
	Extras          []itemDTO      `json:"extras"`
	CollectionIDs   []int64        `json:"collectionIds"`
	LibraryName     string         `json:"libraryName,omitempty"`
	ShowTitle       string         `json:"showTitle,omitempty"`
	ShowPosterURL   string         `json:"showPosterUrl,omitempty"`
	ShowBackdropURL string         `json:"showBackdropUrl,omitempty"`
}

func (s *Server) itemDTO(it store.Item) itemDTO {
	d := itemDTO{Item: it}
	if it.PosterPath != "" {
		d.PosterURL = "/api/v1/items/" + strconv.FormatInt(it.ID, 10) + "/poster?v=" + strconv.FormatInt(it.UpdatedAt.Unix(), 10)
	}
	if it.BackdropPath != "" {
		d.BackdropURL = "/api/v1/items/" + strconv.FormatInt(it.ID, 10) + "/backdrop?v=" + strconv.FormatInt(it.UpdatedAt.Unix(), 10)
	}
	return d
}

func (s *Server) itemDTOs(items []store.Item) []itemDTO {
	out := make([]itemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, s.itemDTO(it))
	}
	return out
}

func (s *Server) itemDetail(ctx context.Context, it *store.Item) (*itemDetailDTO, error) {
	d := &itemDetailDTO{itemDTO: s.itemDTO(*it), Genres: []string{}, People: []store.Person{}, Files: []fileDTO{},
		Locks: []string{}, Children: []itemDTO{}, Extras: []itemDTO{}, CollectionIDs: []int64{}}
	var err error
	if d.Genres, err = s.store.ItemGenres(ctx, it.ID); err != nil {
		return nil, err
	}
	if d.People, err = s.store.ItemPeople(ctx, it.ID); err != nil {
		return nil, err
	}
	if d.Locks, err = s.store.ItemLocks(ctx, it.ID); err != nil {
		return nil, err
	}
	files, err := s.store.ItemFiles(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		d.Files = append(d.Files, fileDTO{File: f, StreamURL: "/api/v1/media/" + strconv.FormatInt(f.ID, 10)})
	}
	if lib, err := s.store.GetLibrary(ctx, it.LibraryID); err == nil {
		d.LibraryName = lib.Name
	}
	if ids, err := s.store.ItemCollectionIDs(ctx, it.ID); err == nil {
		d.CollectionIDs = ids
	}
	if extras, err := s.store.ListExtras(ctx, it.ID); err == nil && len(extras) > 0 {
		d.Extras = s.itemDTOs(extras)
	}
	if it.Kind == store.KindSeason && it.ParentID > 0 {
		if show, err := s.store.GetItem(ctx, it.ParentID); err == nil {
			d.ShowTitle = show.Title
			showDTO := s.itemDTO(*show)
			d.ShowPosterURL = showDTO.PosterURL
			d.ShowBackdropURL = showDTO.BackdropURL
		}
	}
	if it.Kind == store.KindEpisode && it.ParentID > 0 {
		if season, err := s.store.GetItem(ctx, it.ParentID); err == nil && season.ParentID > 0 {
			if show, err := s.store.GetItem(ctx, season.ParentID); err == nil {
				d.ShowTitle = show.Title
				showDTO := s.itemDTO(*show)
				d.ShowPosterURL = showDTO.PosterURL
				d.ShowBackdropURL = showDTO.BackdropURL
			}
		}
	}
	if it.Kind == store.KindShow || it.Kind == store.KindSeason {
		kind := store.KindSeason
		if it.Kind == store.KindSeason {
			kind = store.KindEpisode
		}
		children, _, err := s.store.ListItems(ctx, store.ItemFilter{ParentID: it.ID, Kind: kind, Page: 1, PerPage: 200, Sort: "title"})
		if err != nil {
			return nil, err
		}
		d.Children = s.itemDTOs(children)
	}
	return d, nil
}
