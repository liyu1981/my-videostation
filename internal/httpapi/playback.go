package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/playback"
	"github.com/liyu1981/myvideostation/internal/store"
	"github.com/liyu1981/myvideostation/internal/subtitle"
)

type subtitleTrackDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Language string `json:"language,omitempty"`
	URL      string `json:"url"`
	Source   string `json:"source"`           // external | embedded
	BurnIn   bool   `json:"burnIn,omitempty"` // image subtitles: only via transcode
	Default  bool   `json:"default,omitempty"`
}

type audioTrackDTO struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Direct   bool   `json:"direct"` // playable without transcoding
}

type nextUpDTO struct {
	ItemID  int64  `json:"itemId"`
	Title   string `json:"title"`
	Season  int    `json:"seasonNumber,omitempty"`
	Episode int    `json:"episodeNumber,omitempty"`
}

type playbackDTO struct {
	ItemID     int64              `json:"itemId"`
	FileID     int64              `json:"fileId"`
	StreamURL  string             `json:"streamUrl"`
	ResumeMS   int64              `json:"resumeMs"`
	DurationMS int64              `json:"durationMs"`
	Subtitles  []subtitleTrackDTO `json:"subtitles"`
	Audio      []audioTrackDTO    `json:"audio"`
	Next       *nextUpDTO         `json:"next,omitempty"`
	Cast       castInfoDTO        `json:"cast"`

	// Container/stream facts used by the player to pick direct play vs HLS.
	Container  string                   `json:"container,omitempty"`
	VideoCodec string                   `json:"videoCodec,omitempty"`
	Height     int                      `json:"height,omitempty"`
	Width      int                      `json:"width,omitempty"`
	DirectPlay bool                     `json:"directPlay"`
	HLS        bool                     `json:"hls"`
	Prepare    bool                     `json:"prepare"`
	Reasons    []string                 `json:"reasons,omitempty"`
	Qualities  []playback.QualityOption `json:"qualities"`
}

// handlePlayback resolves everything the player needs for one item.
func (s *Server) handlePlayback(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	files, err := s.store.ItemFiles(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	var file *store.File
	for i := range files {
		if !files[i].Missing {
			file = &files[i]
			break
		}
	}
	if file == nil {
		writeError(w, http.StatusNotFound, "no_playable_file", "no playable file for this item")
		return
	}

	out := playbackDTO{
		ItemID:     id,
		FileID:     file.ID,
		StreamURL:  "/api/v1/media/" + strconv.FormatInt(file.ID, 10),
		DurationMS: file.DurationMS,
		Subtitles:  []subtitleTrackDTO{},
		Audio:      []audioTrackDTO{},
	}
	if item.PositionMS > 0 && !item.Played {
		out.ResumeMS = item.PositionMS
	}
	out.Cast = s.castInfo(r, file)
	out.Container = playback.ContainerFromPath(file.Path, file.Container)
	out.VideoCodec = file.VideoCodec
	out.Height = file.Height
	out.Width = file.Width

	// Direct play only needs the default audio track to be browser-native; other
	// tracks can be switched later by restarting a transcode session.
	var defaultAudio *store.AudioStream
	for i := range file.AudioStreams {
		if file.AudioStreams[i].Default {
			defaultAudio = &file.AudioStreams[i]
			break
		}
	}
	if defaultAudio == nil && len(file.AudioStreams) > 0 {
		defaultAudio = &file.AudioStreams[0]
	}
	audioCodecs := []string{}
	if defaultAudio != nil {
		audioCodecs = append(audioCodecs, defaultAudio.Codec)
	}
	direct := playback.EvaluateDirectPlay(out.Container, file.VideoCodec, audioCodecs)
	out.DirectPlay = direct.OK
	out.Reasons = direct.Reasons
	out.HLS = s.transcoder.Available()
	out.Prepare = s.ffmpeg != ""
	out.Qualities = playback.QualityLadder(file.Height)

	exts, err := s.store.ExternalSubtitles(r.Context(), file.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, e := range exts {
		out.Subtitles = append(out.Subtitles, subtitleTrackDTO{
			ID:       fmt.Sprintf("ext-%d", e.ID),
			Label:    e.Label,
			Language: e.Language,
			URL:      fmt.Sprintf("/api/v1/media/%d/subtitle?ext=%d", file.ID, e.ID),
			Source:   "external",
		})
	}
	for _, a := range file.AudioStreams {
		out.Audio = append(out.Audio, audioTrackDTO{
			Index: a.Index, Codec: a.Codec, Channels: a.Channels,
			Language: a.Language, Title: a.Title, Default: a.Default,
			Direct: playback.EvaluateDirectPlay(out.Container, file.VideoCodec, []string{a.Codec}).OK,
		})
	}
	for _, st := range file.SubtitleStreams {
		label := st.Title
		if label == "" {
			label = st.Language
		}
		if label == "" {
			label = fmt.Sprintf("Track %d", st.Index)
		}
		track := subtitleTrackDTO{
			ID:       fmt.Sprintf("emb-%d", st.Index),
			Label:    label,
			Language: st.Language,
			Source:   "embedded",
		}
		if imageBasedSubtitle(st.Codec) {
			// Bitmap subtitles (PGS/VobSub) can only be burned in.
			track.BurnIn = true
			out.Subtitles = append(out.Subtitles, track)
			continue
		}
		track.URL = fmt.Sprintf("/api/v1/media/%d/subtitle?embedded=%d", file.ID, st.Index)
		out.Subtitles = append(out.Subtitles, track)
	}
	// Text tracks are auto-enabled; image tracks are only chosen explicitly
	// because enabling them forces a burn-in transcode.
	for i := range out.Subtitles {
		if out.Subtitles[i].BurnIn {
			continue
		}
		lang := strings.ToLower(out.Subtitles[i].Language)
		if lang == "en" || lang == "eng" || lang == "english" {
			out.Subtitles[i].Default = true
			break
		}
	}
	if !hasDefault(out.Subtitles) {
		for i := range out.Subtitles {
			if !out.Subtitles[i].BurnIn {
				out.Subtitles[i].Default = true
				break
			}
		}
	}

	if item.Kind == store.KindEpisode {
		if next, err := s.store.NextEpisode(r.Context(), item.ID); err == nil && next != nil {
			out.Next = &nextUpDTO{ItemID: next.ID, Title: next.Title, Season: next.SeasonNumber, Episode: next.EpisodeNumber}
		}
	}

	writeJSON(w, http.StatusOK, out)
}

func hasDefault(tracks []subtitleTrackDTO) bool {
	for _, t := range tracks {
		if t.Default {
			return true
		}
	}
	return false
}

func imageBasedSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "hdmv_pgs_subtitle", "pgs", "dvd_subtitle", "dvdsub", "dvb_subtitle", "xsub":
		return true
	}
	return false
}

// handleSubtitle serves an external or ffmpeg-extracted subtitle as WebVTT.
func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	fileID, ok := parseInt64(r.PathValue("fileId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid file id")
		return
	}
	file, err := s.store.FileByID(r.Context(), fileID)
	if err != nil {
		s.fail(w, err)
		return
	}

	q := r.URL.Query()
	delay := intQuery(r, "delayMs", 0)
	if delay < -60000 {
		delay = -60000
	} else if delay > 60000 {
		delay = 60000
	}
	var (
		data []byte
		etag string
	)
	switch {
	case q.Get("ext") != "":
		subID, ok := parseInt64(q.Get("ext"))
		if !ok {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid subtitle id")
			return
		}
		sub, err := s.store.ExternalSubtitleByID(r.Context(), subID)
		if err != nil || sub.FileID != fileID {
			http.NotFound(w, r)
			return
		}
		raw, err := os.ReadFile(sub.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data, err = subtitle.ToWebVTT(sub.Format, raw)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "subtitle_unsupported", err.Error())
			return
		}
		if st, err := os.Stat(sub.Path); err == nil {
			etag = fmt.Sprintf(`"ext-%d-%d"`, subID, st.ModTime().Unix())
		}
	case q.Get("embedded") != "":
		index, err := strconv.Atoi(q.Get("embedded"))
		if err != nil || index < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid stream index")
			return
		}
		if s.ffmpeg == "" {
			writeError(w, http.StatusServiceUnavailable, "ffmpeg_missing", "ffmpeg is required to extract embedded subtitles")
			return
		}
		data, etag, err = s.extractEmbeddedSubtitle(r.Context(), file, index)
		if err != nil {
			s.log.Error("embedded subtitle extraction failed", "file", fileID, "index", index, "err", err)
			writeError(w, http.StatusBadGateway, "subtitle_extract_failed", "could not extract subtitle track")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "specify ext or embedded")
		return
	}

	if delay != 0 {
		data = subtitle.ShiftWebVTT(data, delay)
		if etag != "" {
			etag = strings.TrimSuffix(etag, `"`) + fmt.Sprintf("-d%d\"", delay)
		}
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// extractEmbeddedSubtitle runs ffmpeg once and caches the WebVTT output.
func (s *Server) extractEmbeddedSubtitle(ctx context.Context, file *store.File, index int) ([]byte, string, error) {
	path, etag, err := s.embeddedSubtitleFile(ctx, file, index)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, etag, nil
}

// embeddedSubtitleFile extracts one embedded text subtitle to the WebVTT cache
// and returns its path (used for on-demand extraction and burn-in).
func (s *Server) embeddedSubtitleFile(ctx context.Context, file *store.File, index int) (string, string, error) {
	if s.ffmpeg == "" {
		return "", "", errors.New("ffmpeg is required to extract embedded subtitles")
	}
	dir := filepath.Join(s.cacheDir, "subs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d-%d.vtt", file.ID, index))

	// Reuse the cache while it is newer than the media file.
	if st, err := os.Stat(path); err == nil && st.ModTime().Unix() >= file.Mtime {
		return path, fmt.Sprintf(`"emb-%d-%d"`, file.ID, st.ModTime().Unix()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.ffmpeg,
		"-v", "error",
		"-i", file.Path,
		"-map", fmt.Sprintf("0:%d", index),
		"-f", "webvtt",
		"pipe:1",
	)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", "", fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", "", err
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return "", "", errors.New("empty subtitle stream")
	}
	if !strings.HasPrefix(string(out), "WEBVTT") {
		out = append([]byte("WEBVTT\n\n"), out...)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		s.log.Warn("embedded subtitle cache write failed", "file", file.ID, "index", index, "err", err)
	}
	return path, fmt.Sprintf(`"emb-%d-%d"`, file.ID, time.Now().Unix()), nil
}
