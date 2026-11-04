package httpapi

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/liyu1981/myvideostation/internal/auth"
	"github.com/liyu1981/myvideostation/internal/store"
)

// jfTokenPattern extracts Token="..." from a MediaBrowser authorization header.
var jfTokenPattern = regexp.MustCompile(`(?i)\btoken\s*=\s*"([^"]*)"`)

// jellyfinToken returns the client token from any of the places Jellyfin
// clients use: X-Emby-Token, X-MediaBrowser-Token, the MediaBrowser
// authorization header, or an api_key query parameter.
func jellyfinToken(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Emby-Token")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-MediaBrowser-Token")); v != "" {
		return v
	}
	for _, h := range []string{"X-Emby-Authorization", "Authorization"} {
		raw := r.Header.Get(h)
		if raw == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "mediabrowser") {
			continue
		}
		if m := jfTokenPattern.FindStringSubmatch(raw); len(m) == 2 {
			if tok := strings.TrimSpace(m[1]); tok != "" {
				return tok
			}
		}
	}
	for _, key := range []string{"api_key", "ApiKey", "apikey"} {
		if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
			return v
		}
	}
	return ""
}

// tokenUser resolves a Jellyfin token to an active user.
func (s *Server) tokenUser(r *http.Request) (*store.User, string, error) {
	raw := jellyfinToken(r)
	if raw == "" {
		return nil, "", store.ErrNotFound
	}
	u, err := s.store.SessionUser(r.Context(), auth.HashToken(raw))
	if err != nil {
		return nil, "", err
	}
	return u, raw, nil
}

// isJellyfinRequest reports whether a request comes from a Jellyfin client
// (used to pick the error format).
func isJellyfinRequest(r *http.Request) bool {
	if r.Header.Get("X-Emby-Authorization") != "" || r.Header.Get("X-Emby-Token") != "" {
		return true
	}
	if r.Header.Get("X-MediaBrowser-Token") != "" {
		return true
	}
	if r.Header.Get("User-Agent") != "" && strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "jellyfin") {
		return true
	}
	return r.URL.Query().Get("api_key") != "" || r.URL.Query().Get("ApiKey") != ""
}
