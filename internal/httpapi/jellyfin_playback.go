package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
	"github.com/liyu1981/myvideostation/internal/playback"
	"github.com/liyu1981/myvideostation/internal/store"
)

// jfPlaybackInfoResponse is the PlaybackInfo payload.
type jfPlaybackInfoResponse struct {
	MediaSources  []jellyfin.MediaSourceInfo `json:"MediaSources"`
	PlaySessionID string                     `json:"PlaySessionId"`
}

// registerJellyfinPlaybackRoutes mounts the playback/progress surface (phase J2/J3).
func (s *Server) registerJellyfinPlaybackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /Items/{itemId}/PlaybackInfo", s.anyAuth(s.handleJFPlaybackInfo))
	mux.HandleFunc("POST /Items/{itemId}/PlaybackInfo", s.anyAuth(s.handleJFPlaybackInfo))
	// Direct static streaming is anonymous, like Jellyfin: ExoPlayer fetches the
	// URL without headers. Ids are opaque, so this does not allow enumeration.
	mux.HandleFunc("GET /Videos/{itemId}/stream", s.handleJFDirectStream)
	mux.HandleFunc("GET /Videos/{itemId}/{rest...}", s.anyAuth(s.handleJFVideo))

	mux.HandleFunc("POST /Sessions/Playing", s.anyAuth(s.handleJFPlaybackStart))
	mux.HandleFunc("POST /Sessions/Playing/Progress", s.anyAuth(s.handleJFPlaybackProgress))
	mux.HandleFunc("POST /Sessions/Playing/Stopped", s.anyAuth(s.handleJFPlaybackStopped))
	mux.HandleFunc("POST /Sessions/Playing/Ping", s.anyAuth(s.handleJFPing))

	mux.HandleFunc("POST /Users/{userId}/PlayedItems/{itemId}", s.anyAuth(s.handleJFMarkPlayed))
	mux.HandleFunc("DELETE /Users/{userId}/PlayedItems/{itemId}", s.anyAuth(s.handleJFMarkUnplayed))
	mux.HandleFunc("POST /Users/{userId}/FavoriteItems/{itemId}", s.anyAuth(s.handleJFFavorite))
	mux.HandleFunc("DELETE /Users/{userId}/FavoriteItems/{itemId}", s.anyAuth(s.handleJFUnfavorite))
	mux.HandleFunc("POST /UserFavoriteItems/{itemId}", s.anyAuth(s.handleJFFavorite))
	mux.HandleFunc("DELETE /UserFavoriteItems/{itemId}", s.anyAuth(s.handleJFUnfavorite))

	mux.HandleFunc("GET /Shows/{itemId}/Episodes", s.anyAuth(s.handleJFShowEpisodes))
}

// ---- playback info ---------------------------------------------------------

func (s *Server) handleJFPlaybackInfo(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	token := jellyfinToken(r)
	playSessionID := randomHex(16)
	sources := s.jfPlaybackSources(ctx, it, token, playSessionID)
	writeJSON(w, http.StatusOK, jfPlaybackInfoResponse{
		MediaSources:  sources,
		PlaySessionID: playSessionID,
	})
}

// jfPlaybackSources decorates data-only media sources with stream URLs.
func (s *Server) jfPlaybackSources(ctx context.Context, it *store.Item, token, playSessionID string) []jellyfin.MediaSourceInfo {
	sources := s.jfMediaSources(ctx, it)
	itemID := jellyfin.ItemID(it.ID)
	tokenQuery := ""
	if token != "" {
		tokenQuery = "&api_key=" + url.QueryEscape(token)
	}
	for i := range sources {
		if _, err := jellyfin.DecodeKind(sources[i].ID, jellyfin.KindFile); err != nil {
			continue
		}
		// Subtitle side-load URLs must authenticate on their own, because ExoPlayer
		// fetches them as separate requests.
		if token != "" {
			for j := range sources[i].MediaStreams {
				ms := &sources[i].MediaStreams[j]
				if ms.Type != "Subtitle" || ms.DeliveryURL == "" || strings.Contains(ms.DeliveryURL, "api_key=") {
					continue
				}
				sep := "?"
				if strings.Contains(ms.DeliveryURL, "?") {
					sep = "&"
				}
				ms.DeliveryURL += sep + "api_key=" + url.QueryEscape(token)
			}
		}
		sources[i].DirectStreamURL = fmt.Sprintf("/Videos/%s/stream?static=true&MediaSourceId=%s%s",
			itemID, sources[i].ID, tokenQuery)
		if sources[i].SupportsTranscoding {
			// No StartTimeTicks/AudioStreamIndex/SubtitleStreamIndex here: Jellyfin
			// clients append their own values, and duplicate query keys would make
			// the first (ours) win. The manifest handler defaults them.
			sources[i].TranscodingURL = fmt.Sprintf(
				"/Videos/%s/master.m3u8?MediaSourceId=%s&PlaySessionId=%s%s",
				itemID, sources[i].ID, url.QueryEscape(playSessionID), tokenQuery)
			sources[i].TranscodingSubProtocol = "hls"
			sources[i].TranscodingContainer = "mp4"
			sources[i].HasSegments = true
		}
	}
	return sources
}

// ---- video stream / manifest ----------------------------------------------

// handleJFVideo dispatches the Jellyfin video URL shapes:
//
//	/Videos/{id}/stream[.ext]          direct play with range support
//	/Videos/{id}/master.m3u8           HLS manifest (starts/reuses a session)
//	/Videos/{id}/{source}/Subtitles/{index}/Stream.{format}
func (s *Server) handleJFVideo(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(r.PathValue("rest"), "/")
	switch {
	case rest == "stream" || strings.HasPrefix(rest, "stream."):
		s.handleJFDirectStream(w, r)
	case rest == "master.m3u8" || rest == "main.m3u8":
		s.handleJFManifest(w, r)
	case strings.Contains(rest, "/Subtitles/"):
		s.handleJFSubtitleRedirect(w, r, rest)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) handleJFDirectStream(w http.ResponseWriter, r *http.Request) {
	// No auth on purpose (see the route comment); jfSourceFile resolves the
	// opaque item id and the media source id.
	ctx := r.Context()
	if me, ok := s.jfAuthenticatedUser(r); ok {
		ctx = s.userContext(ctx, me, "")
	}
	file, err := s.jfSourceFile(ctx, r)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.serveMediaFile(w, r, file)
}

// jfSourceFile resolves the requested media source (or the first available file).
func (s *Server) jfSourceFile(ctx context.Context, r *http.Request) (*store.File, error) {
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetItem(ctx, itemID); err != nil { // access check
		return nil, err
	}
	files, err := s.store.ItemFiles(ctx, itemID)
	if err != nil {
		return nil, err
	}
	wanted := int64(0)
	if raw := firstValue(r.URL.Query(), "MediaSourceId"); raw != "" {
		if id, err := jellyfin.DecodeKind(raw, jellyfin.KindFile); err == nil {
			wanted = id
		}
	}
	for i := range files {
		if files[i].Missing {
			continue
		}
		if wanted != 0 && files[i].ID != wanted {
			continue
		}
		return &files[i], nil
	}
	return nil, errNoPlayableFile
}

// handleJFManifest starts (or reuses) an HLS session at the requested offset and
// returns the playlist with absolute, token-bearing segment URLs.
func (s *Server) handleJFManifest(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := s.store.GetItem(ctx, itemID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	file, err := s.jfSourceFile(ctx, r)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	in := transcodeInput{
		FileID:    file.ID,
		StartMS:   int64(jfInt(q, "StartTimeTicks", 0) / 10000),
		MaxHeight: jfInt(q, "MaxHeight", 0),
	}
	if v := jfInt(q, "AudioStreamIndex", -1); v >= 0 {
		in.AudioIndex = &v
	}
	if v := jfInt(q, "SubtitleStreamIndex", -1); v >= 0 {
		burnIndex, burnPath, ok := s.jfBurnInTarget(ctx, file, v)
		if !ok {
			// Unknown subtitle track: play without subtitles instead of failing.
			burnIndex = -1
		}
		if burnIndex >= 0 {
			in.SubtitleIndex = &burnIndex
		}
		in.BurnSubtitlePath = burnPath
	}

	// A new manifest for the same PlaySessionId means the client seeked: stop the
	// previous job for that session so builds do not pile up.
	playSession := firstValue(q, "PlaySessionId")
	var sess *playback.Session
	if playSession != "" {
		if previous := s.jfTakeSession(playSession); previous != "" {
			s.transcoder.Stop(previous)
		}
	}
	sess, _, err = s.startTranscodeSession(ctx, itemID, in)
	if err != nil {
		s.writeTranscodeError(w, itemID, file, err)
		return
	}
	if playSession != "" {
		s.jfRememberSession(playSession, sess.ID)
	}

	raw, err := os.ReadFile(sess.Playlist)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	token := jellyfinToken(r)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(rewriteHLSPlaylist(raw, sess.ID, token))
}

// rewriteHLSPlaylist makes segment/init URIs absolute under /hls/{session} and
// carries the API token so segment requests authenticate.
func rewriteHLSPlaylist(raw []byte, sessionID, token string) []byte {
	tokenQuery := ""
	if token != "" {
		tokenQuery = "?api_key=" + url.QueryEscape(token)
	}
	base := "/hls/" + sessionID + "/"
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "#EXT-X-MAP:"):
			start := strings.Index(trimmed, `URI="`)
			if start < 0 {
				continue
			}
			rest := trimmed[start+len(`URI="`):]
			end := strings.Index(rest, `"`)
			if end <= 0 {
				continue
			}
			lines[i] = trimmed[:start] + `URI="` + base + rest[:end] + tokenQuery + `"` + rest[end+1:]
		case strings.HasPrefix(trimmed, "#"):
			// other tags stay untouched
		default:
			lines[i] = base + trimmed + tokenQuery
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// jfBurnInTarget resolves a Jellyfin subtitle stream index to a burn-in request.
// Text subtitles (embedded or sidecar) are extracted to WebVTT and burned in via
// the subtitles filter; bitmap streams use the overlay path.
func (s *Server) jfBurnInTarget(ctx context.Context, file *store.File, index int) (int, string, bool) {
	if index >= 100000 {
		extID := int64(index - 100000)
		sub, err := s.store.ExternalSubtitleByID(ctx, extID)
		if err != nil || sub.FileID != file.ID {
			return -1, "", false
		}
		return -1, sub.Path, true
	}
	for _, st := range file.SubtitleStreams {
		if st.Index != index {
			continue
		}
		if imageBasedSubtitle(st.Codec) {
			return st.Index, "", true
		}
		path, _, err := s.embeddedSubtitleFile(ctx, file, st.Index)
		if err != nil {
			s.log.Warn("subtitle extraction for burn-in failed", "file", file.ID, "index", st.Index, "err", err)
			return -1, "", false
		}
		return -1, path, true
	}
	return -1, "", false
}

// handleJFSubtitleRedirect serves Jellyfin-shaped subtitle URLs by redirecting to
// our WebVTT endpoint (the token is preserved in the query).
func (s *Server) handleJFSubtitleRedirect(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	// <source>/Subtitles/<index>/Stream.<format>
	if len(parts) < 4 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	fileID, err := jellyfin.DecodeKind(parts[0], jellyfin.KindFile)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	target := fmt.Sprintf("/api/v1/media/%d/subtitle?", fileID)
	if index >= 100000 {
		target += "ext=" + strconv.Itoa(index-100000)
	} else {
		target += "embedded=" + strconv.Itoa(index)
	}
	if token := jellyfinToken(r); token != "" {
		target += "&api_key=" + url.QueryEscape(token)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// ---- progress (J3) ---------------------------------------------------------

type jfPlaybackReport struct {
	ItemID        string `json:"ItemId"`
	MediaSourceID string `json:"MediaSourceId"`
	PlaySessionID string `json:"PlaySessionId"`
	PositionTicks int64  `json:"PositionTicks"`
	IsPaused      bool   `json:"IsPaused"`
	IsMuted       bool   `json:"IsMuted"`
	PlayMethod    string `json:"PlayMethod"`
}

// decodeJFBody decodes permissively: Jellyfin clients send many extra fields.
func decodeJFBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(dst)
}

func (s *Server) jfApplyPlaybackReport(r *http.Request, report jfPlaybackReport, stopped bool) {
	ctx := r.Context()
	itemID, err := jellyfin.DecodeKind(report.ItemID, jellyfin.KindItem)
	if err != nil {
		return
	}
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return
	}
	positionMS := report.PositionTicks / 10000
	durationMS := it.DurationMS
	if durationMS == 0 {
		durationMS = it.RuntimeMS
	}
	played := false
	if stopped && durationMS > 0 && positionMS >= durationMS*95/100 {
		played = true
	}
	if err := s.store.SetProgress(ctx, itemID, positionMS, durationMS, played); err != nil {
		s.log.Warn("jellyfin progress write failed", "item", itemID, "err", err)
	}
	if stopped {
		if playSession := report.PlaySessionID; playSession != "" {
			if id := s.jfTakeSession(playSession); id != "" {
				s.transcoder.Stop(id)
			}
		}
	}
}

func (s *Server) handleJFPlaybackStart(w http.ResponseWriter, r *http.Request) {
	var report jfPlaybackReport
	_ = decodeJFBody(r, &report)
	s.jfApplyPlaybackReport(r, report, false)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJFPlaybackProgress(w http.ResponseWriter, r *http.Request) {
	var report jfPlaybackReport
	_ = decodeJFBody(r, &report)
	s.jfApplyPlaybackReport(r, report, false)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJFPlaybackStopped(w http.ResponseWriter, r *http.Request) {
	var report jfPlaybackReport
	_ = decodeJFBody(r, &report)
	s.jfApplyPlaybackReport(r, report, true)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJFPing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// ---- user data / media segments / device options ---------------------------

// handleJFUserData answers GET /UserItems/{itemId}/UserData.
func (s *Server) handleJFUserData(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	itemID, ok := s.jfResolveItemID(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	favorites, _ := s.store.FavoriteItemIDs(ctx, []int64{itemID})
	playCount := 0
	if it.Played {
		playCount = 1
	}
	itemEnc := jellyfin.ItemID(itemID)
	writeJSON(w, http.StatusOK, jellyfin.UserItemDataDto{
		Key:                   itemEnc,
		ItemID:                itemEnc,
		Played:                it.Played,
		PlaybackPositionTicks: jfTicks(it.PositionMS),
		PlayCount:             playCount,
		IsFavorite:            favorites[itemID],
	})
}

// handleJFUpdateUserData answers POST /UserItems/{itemId}/UserData.
func (s *Server) handleJFUpdateUserData(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	itemID, ok := s.jfResolveItemID(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var in struct {
		Played                *bool  `json:"Played"`
		PlaybackPositionTicks *int64 `json:"PlaybackPositionTicks"`
		IsFavorite            *bool  `json:"IsFavorite"`
	}
	_ = decodeJFBody(r, &in)
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	position := it.PositionMS
	played := it.Played
	if in.PlaybackPositionTicks != nil {
		position = *in.PlaybackPositionTicks / 10000
	}
	if in.Played != nil {
		played = *in.Played
		if !played && in.PlaybackPositionTicks == nil {
			position = 0
		}
	}
	if in.PlaybackPositionTicks != nil || in.Played != nil {
		if err := s.store.SetProgress(ctx, itemID, position, it.DurationMS, played); err != nil {
			s.fail(w, err)
			return
		}
	}
	if in.IsFavorite != nil {
		collection, err := s.store.SystemCollection(ctx, store.SystemFavorites)
		if err == nil {
			if *in.IsFavorite {
				err = s.store.AddToCollection(ctx, collection, []int64{itemID})
			} else {
				err = s.store.RemoveFromCollection(ctx, collection, itemID)
			}
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				s.fail(w, err)
				return
			}
		}
	}
	s.handleJFUserData(w, r)
}

// handleJFMediaSegments answers GET /MediaSegments/{itemId}. We do not detect
// intro/outro segments (the client then simply hides the skip button).
func (s *Server) handleJFMediaSegments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Items": []any{}, "TotalRecordCount": 0, "StartIndex": 0,
	})
}

// handleJFSaveDeviceOptions accepts a client device-options update.
func (s *Server) handleJFSaveDeviceOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- played / favorites ----------------------------------------------------

func (s *Server) jfResolveItemID(r *http.Request) (int64, bool) {
	id, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		return 0, false
	}
	if _, err := s.store.GetItem(r.Context(), id); err != nil {
		return 0, false
	}
	return id, true
}

func (s *Server) handleJFMarkPlayed(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	itemID, ok := s.jfResolveItemID(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	duration := it.DurationMS
	if duration == 0 {
		duration = it.RuntimeMS
	}
	if err := s.store.SetProgress(ctx, itemID, duration, duration, true); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJFMarkUnplayed(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	itemID, ok := s.jfResolveItemID(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := s.store.SetProgress(ctx, itemID, 0, 0, false); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) jfFavoriteTarget(r *http.Request) (int64, int64, bool) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		return 0, 0, false
	}
	ctx := s.userContext(r.Context(), me, "")
	itemID, ok := s.jfResolveItemID(r.WithContext(ctx))
	if !ok {
		return 0, 0, false
	}
	collection, err := s.store.SystemCollection(ctx, store.SystemFavorites)
	if err != nil {
		return 0, 0, false
	}
	return collection, itemID, true
}

func (s *Server) handleJFFavorite(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	collection, itemID, ok := s.jfFavoriteTarget(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := s.store.AddToCollection(ctx, collection, []int64{itemID}); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJFUnfavorite(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	collection, itemID, ok := s.jfFavoriteTarget(r.WithContext(ctx))
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := s.store.RemoveFromCollection(ctx, collection, itemID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- shows ----------------------------------------------------------------

func (s *Server) handleJFShowEpisodes(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	showID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := s.store.GetItem(ctx, showID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	serverID := s.jellyfinServerID(ctx)
	var items []store.Item
	if seasonRaw := firstValue(q, "seasonId"); seasonRaw != "" {
		seasonID, err := jellyfin.DecodeKind(seasonRaw, jellyfin.KindItem)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		items, _, err = s.store.ListItems(ctx, store.ItemFilter{
			ParentID: seasonID, Kind: store.KindEpisode, Page: 1, PerPage: 200,
		})
		if err != nil {
			s.fail(w, err)
			return
		}
		jfSortEpisodes(items, false)
	} else {
		items, _, err = s.jfSeriesEpisodes(ctx, showID)
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, serverID, items, false), TotalRecordCount: len(items),
	})
}

// ---- helpers ---------------------------------------------------------------

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// jfTakeSession removes and returns the HLS session bound to a PlaySessionId.
func (s *Server) jfTakeSession(playSessionID string) string {
	s.jfPlayMu.Lock()
	defer s.jfPlayMu.Unlock()
	id := s.jfPlaySessions[playSessionID]
	delete(s.jfPlaySessions, playSessionID)
	return id
}

// jfRememberSession binds an HLS session to a client PlaySessionId.
func (s *Server) jfRememberSession(playSessionID, sessionID string) {
	s.jfPlayMu.Lock()
	defer s.jfPlayMu.Unlock()
	if s.jfPlaySessions == nil {
		s.jfPlaySessions = map[string]string{}
	}
	if len(s.jfPlaySessions) > 64 {
		// Bounded: drop entries whose sessions are already gone.
		for k, v := range s.jfPlaySessions {
			if _, ok := s.transcoder.Get(v); !ok {
				delete(s.jfPlaySessions, k)
			}
		}
	}
	s.jfPlaySessions[playSessionID] = sessionID
}
