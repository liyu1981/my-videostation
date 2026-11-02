package httpapi

import (
	"net/http"

	"github.com/liyu1981/myvideostation/internal/redact"
	"github.com/liyu1981/myvideostation/internal/store"
)

func (s *Server) audit(r *http.Request, action, target, detail string) {
	u := userFrom(r)
	if u == nil {
		return
	}
	s.auditAs(r, u, action, target, detail)
}

func (s *Server) auditAs(r *http.Request, u *store.User, action, target, detail string) {
	if u == nil {
		return
	}
	if err := s.store.RecordAudit(r.Context(), u.ID, u.Username, action, target, redact.String(detail), clientIP(r)); err != nil {
		s.log.Warn("audit write failed", "action", action, "err", err)
	}
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListAuditEvents(r.Context(), intQuery(r, "limit", 100))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
