package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/backup"
)

func (s *Server) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"maintenance":   s.maintenance.Load(),
		"pendingImport": s.dataDir != "" && backup.Pending(s.dataDir),
	})
}

func (s *Server) handleBackupExports(w http.ResponseWriter, r *http.Request) {
	list, err := backup.ListExports(s.dataDir)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": list})
}

func (s *Server) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	if s.dataDir == "" {
		writeError(w, http.StatusServiceUnavailable, "backup_unavailable", "data directory is not configured")
		return
	}
	archive, err := backup.Export(r.Context(), s.dataDir, s.version, s.store.Snapshot)
	if err != nil {
		s.log.Error("export failed", "err", err)
		writeError(w, http.StatusInternalServerError, "export_failed", err.Error())
		return
	}
	s.audit(r, "backup.export", "archive:"+archive.Name, "")
	writeJSON(w, http.StatusOK, archive)
}

func (s *Server) handleBackupImports(w http.ResponseWriter, r *http.Request) {
	list, err := backup.ListImports(s.dataDir)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imports": list})
}

func (s *Server) handleBackupImport(w http.ResponseWriter, r *http.Request) {
	if s.dataDir == "" {
		writeError(w, http.StatusServiceUnavailable, "backup_unavailable", "data directory is not configured")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		list, err := backup.ListImports(s.dataDir)
		if err != nil {
			s.fail(w, err)
			return
		}
		if len(list) == 0 {
			writeError(w, http.StatusBadRequest, "no_imports", "no import bundles found in the import folder")
			return
		}
		name = list[0].Name
	}

	// Safety first: never apply an import without a fresh export of current data.
	exported, err := backup.Export(r.Context(), s.dataDir, s.version, s.store.Snapshot)
	if err != nil {
		s.log.Error("pre-import export failed", "err", err)
		writeError(w, http.StatusInternalServerError, "export_failed", "pre-import export failed: "+err.Error())
		return
	}
	if err := backup.StageImport(r.Context(), s.dataDir, name); err != nil {
		s.log.Error("import staging failed", "archive", name, "err", err)
		writeError(w, http.StatusBadRequest, "import_invalid", err.Error())
		return
	}
	if err := backup.MarkPending(s.dataDir, name); err != nil {
		s.log.Error("import marker failed", "archive", name, "err", err)
		writeError(w, http.StatusInternalServerError, "import_failed", err.Error())
		return
	}
	s.audit(r, "backup.import", "archive:"+name, "safety export: "+exported.Name)
	s.maintenance.Store(true)
	if s.restart != nil {
		go func() {
			time.Sleep(time.Second)
			s.restart()
		}()
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "restarting",
		"name":   name,
		"export": exported.Name,
	})
}
