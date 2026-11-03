package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/dlna"
	"github.com/liyu1981/myvideostation/internal/store"
)

const (
	castSecretKey = "security.cast_secret"
	castTokenTTL  = 12 * time.Hour
)

type castInfoDTO struct {
	URL        string `json:"url,omitempty"`
	Compatible bool   `json:"compatible"`
	Reachable  bool   `json:"reachable"`
	Reason     string `json:"reason,omitempty"`
}

// mediaAuth accepts either the normal session cookie or a signed cast URL token.
func (s *Server) mediaAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := r.URL.Query().Get("cast"); token != "" {
			if id, ok := parseInt64(r.PathValue("fileId")); ok && s.validCastToken(r.Context(), token, id) {
				next(w, r)
				return
			}
		}
		s.auth(next)(w, r)
	}
}

func (s *Server) castSecret(ctx context.Context) ([]byte, error) {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if len(s.castKey) > 0 {
		return s.castKey, nil
	}
	value, ok, err := s.store.Setting(ctx, castSecretKey)
	if err != nil {
		return nil, err
	}
	if ok && value != "" {
		if key, err := base64.RawURLEncoding.DecodeString(value); err == nil && len(key) >= 32 {
			s.castKey = key
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := s.store.PutSettings(ctx, map[string]string{castSecretKey: base64.RawURLEncoding.EncodeToString(key)}); err != nil {
		return nil, err
	}
	s.castKey = key
	return key, nil
}

func (s *Server) mediaCastURL(r *http.Request, fileID int64) (string, bool, error) {
	key, err := s.castSecret(r.Context())
	if err != nil {
		return "", false, err
	}
	expiry := time.Now().Add(castTokenTTL).Unix()
	payload := fmt.Sprintf("%d.%d", fileID, expiry)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	token := base64.RawURLEncoding.EncodeToString([]byte(payload + "." + hex.EncodeToString(mac.Sum(nil))))
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	raw := fmt.Sprintf("%s://%s/api/v1/media/%d?cast=%s", scheme, host, fileID, url.QueryEscape(token))
	return raw, castHostReachable(host), nil
}

func (s *Server) validCastToken(ctx context.Context, token string, fileID int64) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	// Reject non-canonical base64 encodings so a tampered token cannot
	// decode to the same bytes as the original.
	if base64.RawURLEncoding.EncodeToString(raw) != token {
		return false
	}
	lastDot := strings.LastIndexByte(string(raw), '.')
	if lastDot <= 0 {
		return false
	}
	payload := string(raw[:lastDot])
	sig, err := hex.DecodeString(string(raw[lastDot+1:]))
	if err != nil {
		return false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 2 {
		return false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id != fileID {
		return false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	key, err := s.castSecret(ctx)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(mac.Sum(nil), sig)
}

func castHostReachable(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return false
	}
	ip := net.ParseIP(hostname)
	return ip == nil || !ip.IsLoopback()
}

// castCompatible is deliberately conservative: MP4/MOV with H.264 video and
// AAC/MP3 audio. Everything else is a job for M5 transcoding.
func castCompatible(file *store.File) (bool, string) {
	container := strings.ToLower(file.Container)
	if !(strings.Contains(container, "mp4") || strings.Contains(container, "m4v") || strings.Contains(container, "mov")) {
		if container == "" {
			return false, "media format has not been probed; transcoding is needed for casting"
		}
		return false, "container " + file.Container + " needs transcoding for casting"
	}
	video := strings.ToLower(file.VideoCodec)
	if !(strings.Contains(video, "h264") || strings.Contains(video, "avc")) {
		if video == "" {
			return false, "video codec has not been probed; transcoding is needed for casting"
		}
		return false, "video codec " + file.VideoCodec + " needs transcoding for casting"
	}
	for _, audio := range file.AudioStreams {
		codec := strings.ToLower(audio.Codec)
		if codec == "" {
			continue
		}
		if !(strings.Contains(codec, "aac") || strings.Contains(codec, "mp3")) {
			return false, "audio codec " + audio.Codec + " needs transcoding for casting"
		}
	}
	return true, ""
}

func (s *Server) castInfo(r *http.Request, file *store.File) castInfoDTO {
	compatible, reason := castCompatible(file)
	streamURL, reachable, err := s.mediaCastURL(r, file.ID)
	if err != nil {
		return castInfoDTO{Compatible: false, Reason: "could not create a cast URL"}
	}
	if !reachable {
		return castInfoDTO{URL: streamURL, Compatible: compatible, Reachable: false, Reason: "open MyVideoStation via the NAS LAN IP (not localhost) to cast"}
	}
	return castInfoDTO{URL: streamURL, Compatible: compatible, Reachable: true, Reason: reason}
}

type dlnaPlayInput struct {
	FileID     int64  `json:"fileId"`
	ControlURL string `json:"controlUrl"`
	Title      string `json:"title"`
}

func (s *Server) handleCastDLNADiscover(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	devices, err := dlna.Discover(ctx, 3*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, "dlna_discovery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *Server) handleCastDLNAPlay(w http.ResponseWriter, r *http.Request) {
	var in dlnaPlayInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.FileID <= 0 || strings.TrimSpace(in.ControlURL) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "fileId and controlUrl are required")
		return
	}
	file, err := s.store.FileByID(r.Context(), in.FileID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if ok, reason := castCompatible(file); !ok {
		writeError(w, http.StatusUnprocessableEntity, "needs_transcoding", reason)
		return
	}
	mediaURL, reachable, err := s.mediaCastURL(r, file.ID)
	if err != nil || !reachable {
		writeError(w, http.StatusBadRequest, "cast_url_unavailable", "open MyVideoStation via the NAS LAN IP (not localhost) to cast")
		return
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "MyVideoStation"
	}
	if err := dlna.Play(r.Context(), in.ControlURL, mediaURL, title); err != nil {
		writeError(w, http.StatusBadGateway, "dlna_play_failed", err.Error())
		return
	}
	s.audit(r, "cast.dlna", "file:"+strconv.FormatInt(file.ID, 10), title)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
