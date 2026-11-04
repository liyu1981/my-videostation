package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/liyu1981/myvideostation/internal/playback"
	"github.com/liyu1981/myvideostation/internal/store"
)

type transcodeInput struct {
	FileID           int64  `json:"fileId"`
	StartMS          int64  `json:"startMs"`
	AudioIndex       *int   `json:"audioIndex"`
	SubtitleIndex    *int   `json:"subtitleIndex"`      // embedded stream to burn in
	ExternalSubtitle int64  `json:"externalSubtitleId"` // sidecar to burn in
	MaxHeight        int    `json:"maxHeight"`
	Mode             string `json:"mode"` // "", "remux", "transcode"

	// BurnSubtitlePath is internal: an extracted/cached subtitle file to burn in
	// (used by the Jellyfin adapter for text tracks). Not part of the JSON API.
	BurnSubtitlePath string `json:"-"`
}

// handleStartTranscode resolves or starts an HLS session for one item.
func (s *Server) handleStartTranscode(w http.ResponseWriter, r *http.Request) {
	id, ok2 := parseInt64(r.PathValue("itemId"))
	if !ok2 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in transcodeInput
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
	}
	sess, file, err := s.startTranscodeSession(r.Context(), id, in)
	if err != nil {
		s.writeTranscodeError(w, id, file, err)
		return
	}
	// Not audited: seeks and track switches legitimately start many sessions.
	writeJSON(w, http.StatusOK, transcodeResponse(sess))
}

// startTranscodeSession resolves a file and starts (or reuses) an HLS session.
// Shared by the SPA API and the Jellyfin adapter.
func (s *Server) startTranscodeSession(ctx context.Context, itemID int64, in transcodeInput) (*playback.Session, *store.File, error) {
	// GetItem enforces per-library access; ItemFiles is not scoped on its own.
	if _, err := s.store.GetItem(ctx, itemID); err != nil {
		return nil, nil, err
	}
	files, err := s.store.ItemFiles(ctx, itemID)
	if err != nil {
		return nil, nil, err
	}
	var file *store.File
	for i := range files {
		if in.FileID != 0 && files[i].ID != in.FileID {
			continue
		}
		if !files[i].Missing {
			file = &files[i]
			break
		}
	}
	if file == nil {
		return nil, nil, errNoPlayableFile
	}
	if !s.transcoder.Available() {
		return nil, file, playback.ErrNoFFmpeg
	}

	audioIndex := -1
	if in.AudioIndex != nil {
		audioIndex = *in.AudioIndex
	} else {
		for _, a := range file.AudioStreams {
			if a.Default {
				audioIndex = a.Index
				break
			}
		}
		if audioIndex < 0 && len(file.AudioStreams) > 0 {
			audioIndex = file.AudioStreams[0].Index
		}
	}
	burnIndex := -1
	if in.SubtitleIndex != nil {
		burnIndex = *in.SubtitleIndex
	}
	burnPath := in.BurnSubtitlePath
	if in.ExternalSubtitle > 0 {
		sub, err := s.store.ExternalSubtitleByID(ctx, in.ExternalSubtitle)
		if err != nil || sub.FileID != file.ID {
			return nil, file, store.ErrNotFound
		}
		burnPath = sub.Path
	}
	if in.StartMS < 0 {
		in.StartMS = 0
	}
	if file.DurationMS > 0 && in.StartMS > file.DurationMS-2000 {
		in.StartMS = file.DurationMS - 2000
		if in.StartMS < 0 {
			in.StartMS = 0
		}
	}

	req := playback.Request{
		FileID:           file.ID,
		Path:             file.Path,
		Mtime:            file.Mtime,
		Container:        playback.ContainerFromPath(file.Path, file.Container),
		VideoCodec:       file.VideoCodec,
		Width:            file.Width,
		Height:           file.Height,
		HDR:              file.HDR,
		DurationMS:       file.DurationMS,
		Audio:            audioStreams(file.AudioStreams),
		Subtitles:        subtitleStreams(file.SubtitleStreams),
		AudioIndex:       audioIndex,
		BurnSubtitle:     burnIndex,
		BurnSubtitlePath: burnPath,
		StartMS:          in.StartMS,
		MaxHeight:        in.MaxHeight,
		Force:            strings.ToLower(strings.TrimSpace(in.Mode)),
	}
	sess, err := s.transcoder.Start(ctx, req)
	return sess, file, err
}

// errNoPlayableFile signals an item without a usable media file.
var errNoPlayableFile = errors.New("no playable file")

// writeTranscodeError maps transcode failures to HTTP responses.
func (s *Server) writeTranscodeError(w http.ResponseWriter, itemID int64, file *store.File, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, errNoPlayableFile):
		writeError(w, http.StatusNotFound, "no_playable_file", "no playable file for this item")
	case errors.Is(err, playback.ErrNoFFmpeg):
		writeError(w, http.StatusServiceUnavailable, "ffmpeg_missing", "ffmpeg is required for transcoding")
	case errors.Is(err, playback.ErrTooManySessions):
		writeError(w, http.StatusTooManyRequests, "too_many_sessions", "too many concurrent playback sessions; close another player and retry")
	default:
		fileID := int64(0)
		if file != nil {
			fileID = file.ID
		}
		s.log.Error("transcode start failed", "item", itemID, "file", fileID, "err", err)
		writeError(w, http.StatusUnprocessableEntity, "transcode_failed", err.Error())
	}
}

func transcodeResponse(sess *playback.Session) map[string]any {
	st := sess.Status()
	return map[string]any{
		"sessionId":   st.ID,
		"playlistUrl": st.PlaylistURL,
		"startMs":     st.StartMS,
		"durationMs":  st.DurationMS,
		"mode":        st.Mode,
		"height":      st.Height,
		"videoCodec":  st.VideoCodec,
		"audioCodec":  st.AudioCodec,
		"audioIndex":  st.AudioIndex,
		"reasons":     st.Reasons,
	}
}

func (s *Server) handleTranscodeStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session")
	sess, ok := s.transcoder.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess.Status())
}

func (s *Server) handleStopTranscode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session")
	if _, ok := s.transcoder.Get(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	s.transcoder.Stop(id)
	w.WriteHeader(http.StatusNoContent)
}

// handlePlaybackCapabilities tells the client what the server can do.
func (s *Server) handlePlaybackCapabilities(w http.ResponseWriter, r *http.Request) {
	encoders := []map[string]string{}
	for _, e := range playback.EncoderProfiles() {
		encoders = append(encoders, map[string]string{"name": e.Name, "label": e.Label})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"directPlay": map[string]any{
			"containers":  []string{"mp4", "m4v", "mov", "webm", "ogv", "ogg"},
			"videoCodecs": []string{"h264", "vp8", "vp9", "av1"},
			"audioCodecs": []string{"aac", "mp3", "opus", "vorbis", "flac"},
		},
		"transcoding": map[string]any{
			"available": s.transcoder.Available(),
			"encoder":   s.transcoder.Encoder(),
			"encoders":  encoders,
			"qualities": playback.QualityLadder(2160),
		},
	})
}

var hlsFilePattern = regexp.MustCompile(`^(index\.m3u8|init\.mp4|seg\d{5}\.m4s|seg\d{5}\.ts)$`)

// handleHLS serves a session's playlist, init segment and media segments.
func (s *Server) handleHLS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session")
	name := filepath.Base(r.PathValue("file"))
	sess, ok := s.transcoder.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if !hlsFilePattern.MatchString(name) {
		writeError(w, http.StatusNotFound, "not_found", "unknown segment")
		return
	}
	path := filepath.Join(sess.Dir, name)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "segment not yet available")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.fail(w, err)
		return
	}
	switch {
	case strings.HasSuffix(name, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache, no-store")
	case strings.HasSuffix(name, ".mp4"):
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "private, max-age=86400")
	default:
		w.Header().Set("Content-Type", "video/iso.segment")
		w.Header().Set("Cache-Control", "private, max-age=86400")
	}
	// Range support keeps MSE/AVFoundation happy on partial fetches.
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func audioStreams(in []store.AudioStream) []playback.AudioStream {
	out := make([]playback.AudioStream, 0, len(in))
	for _, a := range in {
		out = append(out, playback.AudioStream{
			Index: a.Index, Codec: a.Codec, Channels: a.Channels,
			Language: a.Language, Title: a.Title, Default: a.Default,
		})
	}
	return out
}

func subtitleStreams(in []store.SubtitleStream) []playback.SubtitleStream {
	out := make([]playback.SubtitleStream, 0, len(in))
	for _, s := range in {
		out = append(out, playback.SubtitleStream{
			Index: s.Index, Codec: s.Codec, Language: s.Language, Title: s.Title, Forced: s.Forced,
		})
	}
	return out
}
