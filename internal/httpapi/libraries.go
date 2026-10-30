package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/liyu1981/myvideostation/internal/store"
)

type libraryInput struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Language string   `json:"language"`
	Paths    []string `json:"paths"`
}

func (s *Server) handleListLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.store.ListLibraries(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": libs})
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
	writeJSON(w, http.StatusOK, lib)
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
	lib, err := s.store.CreateLibrary(r.Context(), in.Name, in.Type, in.Language, in.Paths)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("library created", "id", lib.ID, "name", lib.Name, "type", lib.Type)
	writeJSON(w, http.StatusCreated, lib)
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
	if err := s.store.UpdateLibrary(r.Context(), id, in.Name, in.Language, in.Paths); err != nil {
		s.fail(w, err)
		return
	}
	lib, err := s.store.GetLibrary(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
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
	job, err := s.jobs.Enqueue(r.Context(), "scan", map[string]int64{"libraryId": id})
	if err != nil {
		s.fail(w, err)
		return
	}
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
