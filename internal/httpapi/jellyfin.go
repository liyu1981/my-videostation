package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/auth"
	"github.com/liyu1981/myvideostation/internal/jellyfin"
	"github.com/liyu1981/myvideostation/internal/store"
)

// jfServerIDSetting persists the stable server id clients remember.
const jfServerIDSetting = "jellyfin.server_id"

// registerJellyfinRoutes mounts the Jellyfin compatibility adapter at the root,
// where the official clients expect it. Our own API stays under /api/v1.
func (s *Server) registerJellyfinRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /System/Info/Public", s.handleJFPublicInfo)
	mux.HandleFunc("GET /System/Info", s.anyAuth(s.handleJFSystemInfo))
	mux.HandleFunc("GET /Branding/Configuration", s.handleJFBranding)
	mux.HandleFunc("GET /QuickConnect/Enabled", s.handleJFQuickConnect)
	mux.HandleFunc("GET /Users/Public", s.handleJFUsersPublic)
	mux.HandleFunc("POST /Users/AuthenticateByName", s.handleJFAuthenticate)
	mux.HandleFunc("GET /Users/Me", s.anyAuth(s.handleJFMe))
	mux.HandleFunc("GET /Users/{userId}", s.anyAuth(s.handleJFUser))
	mux.HandleFunc("GET /Users/{userId}/Views", s.anyAuth(s.handleJFViews))
	mux.HandleFunc("GET /UserViews", s.anyAuth(s.handleJFViews))
	mux.HandleFunc("GET /DisplayPreferences/{name}", s.anyAuth(s.handleJFDisplayPreferences))
	mux.HandleFunc("GET /Users/{userId}/Items", s.anyAuth(s.handleJFItems))
	mux.HandleFunc("GET /Items", s.anyAuth(s.handleJFItems))
	mux.HandleFunc("GET /Items/{itemId}", s.anyAuth(s.handleJFItem))
	mux.HandleFunc("GET /Items/{itemId}/Images/{imageType}", s.anyAuth(s.handleJFImage))
	mux.HandleFunc("GET /Items/{itemId}/Images/{imageType}/{index}", s.anyAuth(s.handleJFImage))
	mux.HandleFunc("GET /Search/Hints", s.anyAuth(s.handleJFSearchHints))
	mux.HandleFunc("GET /Sessions", s.anyAuth(s.handleJFSessions))
	mux.HandleFunc("POST /Sessions/Capabilities/Full", s.anyAuth(s.handleJFCapabilities))
	s.registerJellyfinPlaybackRoutes(mux)
}

// jellyfinServerID returns (and persists on first use) the server id reported
// to clients.
func (s *Server) jellyfinServerID(ctx context.Context) string {
	s.jfMu.Lock()
	defer s.jfMu.Unlock()
	if s.jfServerID != "" {
		return s.jfServerID
	}
	if v, ok, err := s.store.Setting(ctx, jfServerIDSetting); err == nil && ok && len(v) == 32 {
		s.jfServerID = v
		return v
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	if err := s.store.PutSettings(ctx, map[string]string{jfServerIDSetting: id}); err != nil {
		s.log.Warn("could not persist jellyfin server id", "err", err)
	}
	s.jfServerID = id
	return id
}

func (s *Server) handleJFPublicInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jellyfin.PublicSystemInfo{
		ServerName:               "MyVideoStation",
		Version:                  jellyfin.ServerVersion,
		ProductName:              jellyfin.ProductName,
		OperatingSystem:          runtime.GOOS,
		ID:                       s.jellyfinServerID(r.Context()),
		StartupWizardCompleted:   true,
		SupportsLibraryPlaylists: false,
	})
}

func (s *Server) handleJFSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := jellyfin.SystemInfo{
		PublicSystemInfo: jellyfin.PublicSystemInfo{
			ServerName:               "MyVideoStation",
			Version:                  jellyfin.ServerVersion,
			ProductName:              jellyfin.ProductName,
			OperatingSystem:          runtime.GOOS,
			ID:                       s.jellyfinServerID(r.Context()),
			StartupWizardCompleted:   true,
			SupportsLibraryPlaylists: false,
		},
		OperatingSystemDisplayName:    strings.Title(runtime.GOOS), //nolint:staticcheck
		CanSelfRestart:                true,
		CanLaunchWebBrowser:           false,
		WebSocketPortNumber:           -1, // no websocket API is exposed
		EncoderLocation:               "System",
		SystemUpdateLevel:             "Release",
		CompletedInstallScheduledTask: true,
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleJFBranding(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jellyfin.BrandingConfiguration{})
}

func (s *Server) handleJFQuickConnect(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, false)
}

func (s *Server) handleJFUsersPublic(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]jellyfin.UserDto, 0, len(users))
	for i := range users {
		if !users[i].Active {
			continue
		}
		out = append(out, s.jfUser(r.Context(), &users[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleJFAuthenticate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"Username"`
		Pw       string `json:"Pw"`
	}
	if err := decodeJSON(r, &in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(in.Username))
	if err != nil || u == nil || !u.Active || u.PasswordHash == "" || !auth.VerifyPassword(u.PasswordHash, in.Pw) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.CreateSession(r.Context(), u.ID, hash, r.UserAgent(), clientIP(r), time.Now().Add(sessionTTL)); err != nil {
		s.fail(w, err)
		return
	}
	_ = s.store.CleanupSessions(r.Context())
	s.auditAs(r, u, "auth.login", "user:"+strconv.FormatInt(u.ID, 10), "signed in (Jellyfin client)")
	serverID := s.jellyfinServerID(r.Context())
	writeJSON(w, http.StatusOK, jellyfin.AuthenticationResult{
		User:        s.jfUser(r.Context(), u),
		SessionInfo: s.jfSession(u, r),
		AccessToken: raw,
		ServerID:    serverID,
	})
}

// jfAuthenticatedUser resolves the userId path parameter and checks that the
// token holder may read it.
func (s *Server) jfAuthenticatedUser(r *http.Request) (*store.User, bool) {
	me := userFrom(r)
	if me == nil {
		return nil, false
	}
	raw := r.PathValue("userId")
	if raw == "" {
		return me, true
	}
	id, err := jellyfin.DecodeKind(raw, jellyfin.KindUser)
	if err != nil {
		return nil, false
	}
	if id != me.ID && !store.IsAdmin(r.Context()) {
		return nil, false
	}
	return me, true
}

func (s *Server) handleJFMe(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.jfUser(r.Context(), me))
}

func (s *Server) handleJFUser(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("userId")
	id, err := jellyfin.DecodeKind(raw, jellyfin.KindUser)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	u, err := s.store.GetUserByID(r.Context(), id)
	if err != nil || !u.Active {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.jfUser(r.Context(), u))
}

func (s *Server) handleJFViews(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, jellyfin.QueryResult{Items: []jellyfin.BaseItemDto{}})
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	libs, err := s.store.ListLibraries(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	serverID := s.jellyfinServerID(ctx)
	items := make([]jellyfin.BaseItemDto, 0, len(libs))
	for i := range libs {
		items = append(items, jellyfin.BaseItemDto{
			Name:           libs[i].Name,
			ServerID:       serverID,
			ID:             jellyfin.LibraryID(libs[i].ID),
			Type:           "CollectionFolder",
			CollectionType: jfCollectionType(libs[i].Type),
			IsFolder:       true,
			ChildCount:     int(libs[i].ItemCount),
			CanDownload:    false,
			PlayAccess:     "Full",
		})
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{Items: items, TotalRecordCount: len(items)})
}

// handleJFDisplayPreferences returns default list preferences; clients use it to
// remember sort/view settings, which we do not persist server-side.
func (s *Server) handleJFDisplayPreferences(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	userID := ""
	if me != nil {
		userID = jellyfin.UserID(me.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Id":               r.PathValue("name"),
		"UserId":           userID,
		"Client":           firstValue(r.URL.Query(), "client"),
		"SortBy":           "SortName",
		"SortOrder":        "Ascending",
		"RememberIndexing": false,
		"RememberSorting":  false,
		"ShowSidebar":      false,
		"ShowBackdrop":     true,
		"ScrollDirection":  "Horizontal",
		"IndexBy":          "None",
		"CustomPrefs":      map[string]string{},
	})
}

// handleJFSessions reports no controllable sessions (casting/remote control is
// not implemented through the Jellyfin API).
func (s *Server) handleJFSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handleJFCapabilities accepts a client capability report and ignores it.
func (s *Server) handleJFCapabilities(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func jfCollectionType(libraryType string) string {
	switch libraryType {
	case "movie":
		return "movies"
	case "tv":
		return "tvshows"
	case "home_video", "tv_recording":
		return "homevideos"
	}
	return ""
}

// jfUser renders our user as a Jellyfin user.
func (s *Server) jfUser(ctx context.Context, u *store.User) jellyfin.UserDto {
	enabledFolders := []string{}
	allFolders := true
	if u.LibraryRestricted {
		allFolders = false
		if access, err := s.store.UserLibraryAccess(ctx, u.ID); err == nil {
			for _, a := range access {
				if a.Allowed {
					enabledFolders = append(enabledFolders, jellyfin.LibraryID(a.ID))
				}
			}
		}
	}
	return jellyfin.UserDto{
		Name:                  u.DisplayName,
		ServerID:              s.jellyfinServerID(ctx),
		ID:                    jellyfin.UserID(u.ID),
		HasPassword:           u.PasswordHash != "",
		HasConfiguredPassword: u.PasswordHash != "",
		LastLoginDate:         u.UpdatedAt.UTC().Format(time.RFC3339),
		LastActivityDate:      u.UpdatedAt.UTC().Format(time.RFC3339),
		Configuration: jellyfin.UserConfigDto{
			PlayDefaultAudioTrack:      true,
			GroupedFolders:             []string{},
			OrderedViews:               []string{},
			LatestItemsExcludes:        []string{},
			MyMediaExcludes:            []string{},
			HidePlayedInLatest:         true,
			RememberAudioSelections:    true,
			RememberSubtitleSelections: true,
			EnableNextEpisodeAutoPlay:  true,
			SubtitleMode:               "Default",
		},
		Policy: jellyfin.UserPolicyDto{
			IsAdministrator:                u.Role == store.RoleAdmin,
			IsDisabled:                     !u.Active,
			EnableRemoteAccess:             true,
			EnableMediaPlayback:            true,
			EnableAudioPlaybackTranscoding: true,
			EnableVideoPlaybackTranscoding: true,
			EnablePlaybackRemuxing:         true,
			EnableContentDownloading:       true,
			EnableAllDevices:               true,
			EnableAllChannels:              true,
			EnableAllFolders:               allFolders,
			EnabledFolders:                 enabledFolders,
			LoginAttemptsBeforeLockout:     -1,
		},
	}
}

func (s *Server) jfSession(u *store.User, r *http.Request) jellyfin.SessionInfoDto {
	client, deviceName, deviceID, version := jellyfinClientInfo(r)
	return jellyfin.SessionInfoDto{
		ID:                 jellyfin.UserID(u.ID),
		UserID:             jellyfin.UserID(u.ID),
		UserName:           u.DisplayName,
		Client:             client,
		DeviceName:         deviceName,
		DeviceID:           deviceID,
		ApplicationVersion: version,
		LastActivityDate:   time.Now().UTC().Format(time.RFC3339),
		IsActive:           true,
		PlayableMediaTypes: []string{"Video"},
		SupportedCommands:  []string{},
		NowPlayingQueue:    []any{},
		PlaylistItems:      []any{},
		PlayState:          map[string]any{},
	}
}

// jellyfinClientInfo parses the MediaBrowser authorization header.
func jellyfinClientInfo(r *http.Request) (client, deviceName, deviceID, version string) {
	raw := r.Header.Get("X-Emby-Authorization")
	if raw == "" {
		raw = r.Header.Get("Authorization")
	}
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "client":
			client = value
		case "device":
			deviceName = value
		case "deviceid":
			deviceID = value
		case "version":
			version = value
		}
	}
	return client, deviceName, deviceID, version
}
