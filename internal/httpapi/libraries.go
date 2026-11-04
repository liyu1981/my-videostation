package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/scanrules"
	"github.com/liyu1981/myvideostation/internal/store"
)

type libraryInput struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Language        string   `json:"language"`
	Paths           []string `json:"paths"`
	AutoScanMinutes *int     `json:"autoScanMinutes"`
	WatchEnabled    *bool    `json:"watchEnabled"`

	// Indexing rules (GAP-005). Pointers distinguish "not sent" (PATCH keeps
	// the current value) from an explicit empty/zero value.
	ExcludeGlobs  *[]string `json:"excludeGlobs"`
	IncludeGlobs  *[]string `json:"includeGlobs"`
	MinFileSizeMB *int64    `json:"minFileSizeMb"`
	SkipExtras    *bool     `json:"skipExtras"`
}

const maxLibraryRules = 100

// normalizeRules validates glob lists and the size floor from a request.
func normalizeRules(in libraryInput) (exclude, include []string, minSize int64, skipExtras bool, err error) {
	if in.ExcludeGlobs != nil {
		exclude, err = scanrules.Validate(*in.ExcludeGlobs)
		if err != nil {
			return nil, nil, 0, false, store.NewInvalid("%s", err.Error())
		}
		if len(exclude) > maxLibraryRules {
			return nil, nil, 0, false, store.NewInvalid("at most %d exclude patterns are supported", maxLibraryRules)
		}
	}
	if in.IncludeGlobs != nil {
		include, err = scanrules.Validate(*in.IncludeGlobs)
		if err != nil {
			return nil, nil, 0, false, store.NewInvalid("%s", err.Error())
		}
		if len(include) > maxLibraryRules {
			return nil, nil, 0, false, store.NewInvalid("at most %d include patterns are supported", maxLibraryRules)
		}
	}
	if in.MinFileSizeMB != nil {
		minSize = *in.MinFileSizeMB
		if minSize < 0 {
			return nil, nil, 0, false, store.NewInvalid("minFileSizeMb must be zero or greater")
		}
		if minSize > 1_000_000 {
			return nil, nil, 0, false, store.NewInvalid("minFileSizeMb is unreasonably large")
		}
	}
	if in.SkipExtras != nil {
		skipExtras = *in.SkipExtras
	}
	return exclude, include, minSize, skipExtras, nil
}

type libraryDTO struct {
	store.Library
	UnmatchedCount int `json:"unmatchedCount"`
}

func (s *Server) libraryDTO(r *http.Request, lib store.Library) (libraryDTO, error) {
	n, err := s.store.CountUnmatched(r.Context(), lib.ID)
	if err != nil {
		return libraryDTO{}, err
	}
	return libraryDTO{Library: lib, UnmatchedCount: n}, nil
}

func (s *Server) handleListLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.store.ListLibraries(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]libraryDTO, 0, len(libs))
	for _, lib := range libs {
		dto, err := s.libraryDTO(r, lib)
		if err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": out})
}

func (s *Server) handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	lib, err := s.store.GetLibrary(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	dto, err := s.libraryDTO(r, *lib)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleCreateLibrary(w http.ResponseWriter, r *http.Request) {
	var in libraryInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if err := validatePaths(in.Paths); err != nil {
		s.fail(w, err)
		return
	}
	autoScan := 360
	if in.AutoScanMinutes != nil {
		autoScan = *in.AutoScanMinutes
	}
	watch := true
	if in.WatchEnabled != nil {
		watch = *in.WatchEnabled
	}
	exclude, include, minSize, skipExtras, err := normalizeRules(in)
	if err != nil {
		s.fail(w, err)
		return
	}
	lib, err := s.store.CreateLibraryWithOptions(r.Context(), in.Name, in.Type, in.Language, in.Paths, store.LibraryOptions{
		AutoScanMinutes: autoScan,
		WatchEnabled:    watch,
		ExcludeGlobs:    exclude,
		IncludeGlobs:    include,
		MinFileSizeMB:   minSize,
		SkipExtras:      skipExtras,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("library created", "id", lib.ID, "name", lib.Name, "type", lib.Type,
		"autoScanMinutes", autoScan, "watch", watch, "exclude", len(exclude), "include", len(include),
		"minFileSizeMb", minSize, "skipExtras", skipExtras)
	s.audit(r, "library.create", "library:"+strconv.FormatInt(lib.ID, 10), lib.Name)
	if s.jobs != nil {
		if _, err := s.jobs.Enqueue(r.Context(), "scan", map[string]any{"libraryId": lib.ID}); err != nil {
			s.log.Warn("initial scan enqueue failed", "library", lib.ID, "err", err)
		} else {
			s.log.Info("initial scan queued", "library", lib.ID)
		}
	}
	dto, err := s.libraryDTO(r, *lib)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) handleUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	var in libraryInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.Paths != nil {
		if err := validatePaths(in.Paths); err != nil {
			s.fail(w, err)
			return
		}
	}
	before, err := s.store.GetLibrary(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	exclude, include, minSize, skipExtras, err := normalizeRules(in)
	if err != nil {
		s.fail(w, err)
		return
	}
	upd := store.LibraryUpdate{
		Name:            in.Name,
		Language:        in.Language,
		Paths:           in.Paths,
		AutoScanMinutes: in.AutoScanMinutes,
		WatchEnabled:    in.WatchEnabled,
		MinFileSizeMB:   in.MinFileSizeMB,
		SkipExtras:      in.SkipExtras,
	}
	if in.ExcludeGlobs != nil {
		upd.ExcludeGlobs = &exclude
	}
	if in.IncludeGlobs != nil {
		upd.IncludeGlobs = &include
	}
	if in.MinFileSizeMB != nil {
		v := minSize
		upd.MinFileSizeMB = &v
	}
	if in.SkipExtras != nil {
		v := skipExtras
		upd.SkipExtras = &v
	}
	if err := s.store.UpdateLibrary(r.Context(), id, upd); err != nil {
		s.fail(w, err)
		return
	}
	lib, err := s.store.GetLibrary(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	dto, err := s.libraryDTO(r, *lib)
	if err != nil {
		s.fail(w, err)
		return
	}
	rulesChanged := in.Paths != nil ||
		before.MinFileSizeMB != lib.MinFileSizeMB ||
		before.SkipExtras != lib.SkipExtras ||
		strings.Join(before.ExcludeGlobs, "\n") != strings.Join(lib.ExcludeGlobs, "\n") ||
		strings.Join(before.IncludeGlobs, "\n") != strings.Join(lib.IncludeGlobs, "\n")
	if rulesChanged && s.jobs != nil {
		// Apply the new rules right away instead of waiting for the next scheduled scan.
		if _, err := s.jobs.Enqueue(r.Context(), "scan", map[string]any{"libraryId": id}); err != nil {
			s.log.Warn("post-edit scan enqueue failed", "library", id, "err", err)
		}
	}
	s.audit(r, "library.update", "library:"+strconv.FormatInt(id, 10), lib.Name)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleDeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	if _, err := s.store.GetLibrary(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.DeleteLibrary(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("library deleted", "id", id)
	s.audit(r, "library.delete", "library:"+strconv.FormatInt(id, 10), "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	if _, err := s.store.GetLibrary(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	force := false
	if r.ContentLength > 0 {
		var in struct {
			Force bool `json:"force"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
		force = in.Force
	}
	job, err := s.jobs.Enqueue(r.Context(), "scan", map[string]any{"libraryId": id, "force": force})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "library.scan", "library:"+strconv.FormatInt(id, 10), jobAction(force))
	writeJSON(w, http.StatusAccepted, job)
}

func validatePaths(paths []string) error {
	if len(paths) == 0 {
		return store.Invalid("at least one path is required")
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return store.Invalid("path must be absolute: " + p)
		}
		st, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return store.Invalid("path does not exist: " + p)
			}
			return store.Invalid("cannot read path: " + p)
		}
		if !st.IsDir() {
			return store.Invalid("path is not a directory: " + p)
		}
	}
	return nil
}

func jobAction(force bool) string {
	if force {
		return "force rescan"
	}
	return "scan"
}
