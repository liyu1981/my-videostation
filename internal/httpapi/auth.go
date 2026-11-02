package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/auth"
	"github.com/liyu1981/myvideostation/internal/store"
)

const (
	sessionCookieName = "mvs_session"
	sessionTTL        = 30 * 24 * time.Hour
)

type authUserKey struct{}
type authTokenKey struct{}

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(authUserKey{}).(*store.User)
	return u
}

func tokenFrom(r *http.Request) string {
	v, _ := r.Context().Value(authTokenKey{}).(string)
	return v
}

// auth requires a valid session cookie and puts the user (and user ID for
// per-user progress/collections) into the request context.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
			return
		}
		u, err := s.store.SessionUser(r.Context(), auth.HashToken(cookie.Value))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "session expired")
			} else {
				s.fail(w, err)
			}
			return
		}
		ctx := context.WithValue(r.Context(), authUserKey{}, u)
		ctx = context.WithValue(ctx, authTokenKey{}, cookie.Value)
		ctx = store.WithUserID(ctx, u.ID)
		next(w, r.WithContext(ctx))
	}
}

// admin is auth plus the admin role.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r); u == nil || u.Role != store.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "admin permission required")
			return
		}
		next(w, r)
	})
}

// ---- handlers -------------------------------------------------------------

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	needs, err := s.store.NeedsSetup(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"needsSetup": needs})
}

type setupInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	needs, err := s.store.NeedsSetup(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if !needs {
		writeError(w, http.StatusConflict, "already_setup", "the admin account is already set up")
		return
	}
	var in setupInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if err := auth.ValidateUsername(in.Username); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.store.SetupAdmin(r.Context(), in.Username, in.DisplayName, hash)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(in.Username))
	if err != nil || u == nil || !u.Active || u.PasswordHash == "" || !auth.VerifyPassword(u.PasswordHash, in.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, in.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "invalid_current_password", "current password is incorrect")
		return
	}
	if err := auth.ValidatePassword(in.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.UpdatePassword(r.Context(), u.ID, hash); err != nil {
		s.fail(w, err)
		return
	}
	if token := tokenFrom(r); token != "" {
		if err := s.store.DeleteOtherSessions(r.Context(), u.ID, auth.HashToken(token)); err != nil {
			s.fail(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if token := tokenFrom(r); token != "" {
		if err := s.store.DeleteSession(r.Context(), auth.HashToken(token)); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.store.CreateSession(r.Context(), u.ID, hash, r.UserAgent(), clientIP(r), expires); err != nil {
		return err
	}
	_ = s.store.CleanupSessions(r.Context())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
