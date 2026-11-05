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
	mux.HandleFunc("GET /System/Ping", s.handleJFSystemPing)
	mux.HandleFunc("GET /Branding/Configuration", s.handleJFBranding)
	mux.HandleFunc("GET /QuickConnect/Enabled", s.handleJFQuickConnect)
	// Quick Connect is not implemented. Real Jellyfin answers 401 when it is
	// disabled, which makes clients (Moonfin, jellyfin-web) fall back to the
	// password form instead of retrying the code flow.
	mux.HandleFunc("POST /QuickConnect/Initiate", s.handleJFQuickConnectDisabled)
	mux.HandleFunc("GET /QuickConnect/Connect", s.handleJFQuickConnectDisabled)
	mux.HandleFunc("POST /QuickConnect/Authorize", s.handleJFQuickConnectDisabled)
	mux.HandleFunc("POST /Users/AuthenticateWithQuickConnect", s.handleJFQuickConnectDisabled)
	mux.HandleFunc("GET /Users/Public", s.handleJFUsersPublic)
	mux.HandleFunc("POST /Users/AuthenticateByName", s.handleJFAuthenticate)
	mux.HandleFunc("GET /Users/Me", s.anyAuth(s.handleJFMe))
	mux.HandleFunc("GET /Users/{userId}", s.anyAuth(s.handleJFUser))
	mux.HandleFunc("GET /Users/{userId}/Views", s.anyAuth(s.handleJFViews))
	mux.HandleFunc("GET /UserViews", s.anyAuth(s.handleJFViews))
	mux.HandleFunc("GET /DisplayPreferences/{name}", s.anyAuth(s.handleJFDisplayPreferences))
	mux.HandleFunc("GET /Users/{userId}/Items", s.anyAuth(s.handleJFItems))
	// Moonfin (and some other clients) read item detail through the
	// user-scoped path rather than /Items/{id}.
	mux.HandleFunc("GET /Users/{userId}/Items/{itemId}", s.anyAuth(s.handleJFItem))
	mux.HandleFunc("GET /Users/{userId}/Items/{itemId}/LocalTrailers", s.anyAuth(s.handleJFLocalTrailers))
	mux.HandleFunc("GET /Users/{userId}/Items/{itemId}/Intros", s.anyAuth(s.handleJFEmptyQuery))
	mux.HandleFunc("GET /Users/{userId}/Images/{imageType}", s.handleJFUserImage)
	mux.HandleFunc("GET /UserItems/{itemId}/UserData", s.anyAuth(s.handleJFUserData))
	mux.HandleFunc("POST /UserItems/{itemId}/UserData", s.anyAuth(s.handleJFUpdateUserData))
	// Likes are separate from favourites in Jellyfin; we accept the call so
	// thumb up/down does not error, without conflating it with the favourite flag.
	mux.HandleFunc("POST /UserItems/{itemId}/Rating", s.anyAuth(s.handleJFRating))
	mux.HandleFunc("DELETE /UserItems/{itemId}/Rating", s.anyAuth(s.handleJFRating))
	mux.HandleFunc("POST /UserPlayedItems/{itemId}", s.anyAuth(s.handleJFMarkPlayed))
	mux.HandleFunc("DELETE /UserPlayedItems/{itemId}", s.anyAuth(s.handleJFMarkUnplayed))
	mux.HandleFunc("GET /MediaSegments/{itemId}", s.anyAuth(s.handleJFMediaSegments))
	mux.HandleFunc("POST /Devices/Options", s.anyAuth(s.handleJFSaveDeviceOptions))
	mux.HandleFunc("GET /Items", s.anyAuth(s.handleJFItems))
	mux.HandleFunc("GET /Items/{itemId}", s.anyAuth(s.handleJFItem))
	// Item artwork is anonymous, like Jellyfin: clients load it straight from an
	// <img>/Coil URL without auth headers. Ids are opaque, so guessing is not
	// practical.
	mux.HandleFunc("GET /Items/{itemId}/Images/{imageType}", s.handleJFImage)
	mux.HandleFunc("GET /Items/{itemId}/Images/{imageType}/{index}", s.handleJFImage)
	// Findroid builds lowercase image URLs ("items/<id>/Images/Primary?tag=…"),
	// so mirror the item-image routes case-insensitively for that prefix.
	mux.HandleFunc("GET /items/{itemId}/Images/{imageType}", s.handleJFImage)
	mux.HandleFunc("GET /items/{itemId}/Images/{imageType}/{index}", s.handleJFImage)
	mux.HandleFunc("GET /Search/Hints", s.anyAuth(s.handleJFSearchHints))
	mux.HandleFunc("GET /Sessions", s.anyAuth(s.handleJFSessions))
	mux.HandleFunc("POST /Sessions/Logout", s.anyAuth(s.handleJFLogout))
	mux.HandleFunc("POST /Users/Configuration", s.anyAuth(s.handleJFSaveUserConfiguration))
	mux.HandleFunc("POST /Sessions/Capabilities", s.anyAuth(s.handleJFCapabilities))
	mux.HandleFunc("POST /Sessions/Capabilities/Full", s.anyAuth(s.handleJFCapabilities))
	s.registerJellyfinPlaybackRoutes(mux)
	s.registerJellyfinBrowseRoutes(mux)
}

// initJellyfin loads (or creates) the persistent server id and derives the
// opaque id codec from it. Called once from the composition root, before any
// request can encode or decode an id.
func (s *Server) initJellyfin() {
	if s.store == nil {
		return
	}
	ctx := context.Background()
	id, ok, err := s.store.Setting(ctx, jfServerIDSetting)
	if err != nil || !ok || len(id) != 32 {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		id = hex.EncodeToString(b)
		if err := s.store.PutSettings(ctx, map[string]string{jfServerIDSetting: id}); err != nil {
			s.log.Warn("could not persist jellyfin server id", "err", err)
		}
	}
	s.jfServerID = id
	jellyfin.SetCodec(jellyfin.NewCodec(id))
}

// jellyfinServerID returns the server id reported to clients.
func (s *Server) jellyfinServerID(ctx context.Context) string {
	s.jfMu.Lock()
	defer s.jfMu.Unlock()
	return s.jfServerID
}

func (s *Server) handleJFPublicInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jellyfin.PublicSystemInfo{
		ServerName:               jellyfin.ServerDisplayName,
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
			ServerName:               jellyfin.ServerDisplayName,
			Version:                  jellyfin.ServerVersion,
			ProductName:              jellyfin.ProductName,
			OperatingSystem:          runtime.GOOS,
			ID:                       s.jellyfinServerID(r.Context()),
			StartupWizardCompleted:   true,
			SupportsLibraryPlaylists: false,
		},
		SupportsLibraryMonitor:        true,
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

// handleJFQuickConnectDisabled answers the Quick Connect flow with the same 401
// a Jellyfin server with the feature turned off returns.
func (s *Server) handleJFQuickConnectDisabled(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
}

// handleJFSystemPing is the unauthenticated reachability probe clients use
// before signing in.
func (s *Server) handleJFSystemPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("Jellyfin Server"))
}

// handleJFLogout revokes the caller's token.
func (s *Server) handleJFLogout(w http.ResponseWriter, r *http.Request) {
	if tok := jellyfinToken(r); tok != "" {
		_ = s.store.DeleteSession(r.Context(), auth.HashToken(tok))
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleJFSaveUserConfiguration accepts user configuration changes. Our own
// settings live under /api/v1, so this is a compatibility no-op.
func (s *Server) handleJFSaveUserConfiguration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	res, err := s.jfViewsResult(s.userContext(r.Context(), me, ""))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// jfViewsResult returns the user's libraries as Jellyfin CollectionFolders.
// Jellyfin's bare GET /Items returns these root views, which is what clients use
// to build the "My media" library list.
func (s *Server) jfViewsResult(ctx context.Context) (jellyfin.QueryResult, error) {
	libs, err := s.store.ListLibraries(ctx)
	if err != nil {
		return jellyfin.QueryResult{}, err
	}
	serverID := s.jellyfinServerID(ctx)
	items := make([]jellyfin.BaseItemDto, 0, len(libs))
	for i := range libs {
		d := jellyfin.BaseItemDto{
			Name:              libs[i].Name,
			ServerID:          serverID,
			ID:                jellyfin.LibraryID(libs[i].ID),
			Type:              "CollectionFolder",
			CollectionType:    jfCollectionType(libs[i].Type),
			IsFolder:          true,
			ChildCount:        int(libs[i].ItemCount),
			CanDownload:       false,
			PlayAccess:        "Full",
			BackdropImageTags: []string{},
		}
		if poster, backdrop, err := s.store.LibraryArtwork(ctx, libs[i].ID); err == nil {
			if tag := jfImageTag(poster); tag != "" {
				d.ImageTags = map[string]string{"Primary": tag}
				d.PrimaryImageAspectRatio = 0.6667
			}
			if tag := jfImageTag(backdrop); tag != "" {
				d.BackdropImageTags = []string{tag}
			}
		}
		items = append(items, d)
	}
	return jellyfin.QueryResult{Items: items, TotalRecordCount: len(items)}, nil
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
		"Id":                 r.PathValue("name"),
		"UserId":             userID,
		"Client":             firstValue(r.URL.Query(), "client"),
		"SortBy":             "SortName",
		"SortOrder":          "Ascending",
		"RememberIndexing":   false,
		"RememberSorting":    false,
		"ShowSidebar":        false,
		"ShowBackdrop":       true,
		"ScrollDirection":    "Horizontal",
		"IndexBy":            "None",
		"CustomPrefs":        map[string]string{},
		"PrimaryImageHeight": 150,
		"PrimaryImageWidth":  100,
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
		Name:                      u.DisplayName,
		ServerID:                  s.jellyfinServerID(ctx),
		ID:                        jellyfin.UserID(u.ID),
		HasPassword:               u.PasswordHash != "",
		HasConfiguredPassword:     u.PasswordHash != "",
		HasConfiguredEasyPassword: false,
		LastLoginDate:             u.UpdatedAt.UTC().Format(time.RFC3339),
		LastActivityDate:          u.UpdatedAt.UTC().Format(time.RFC3339),
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
			IsAdministrator:                 u.Role == store.RoleAdmin,
			IsDisabled:                      !u.Active,
			EnableCollectionManagement:      u.Role == store.RoleAdmin,
			EnableSubtitleManagement:        true,
			EnableLyricManagement:           false,
			EnableUserPreferenceAccess:      true,
			EnableRemoteControlOfOtherUsers: u.Role == store.RoleAdmin,
			EnableSharedDeviceControl:       true,
			EnableRemoteAccess:              true,
			EnableLiveTvManagement:          false,
			EnableLiveTvAccess:              false,
			EnableMediaPlayback:             true,
			EnableAudioPlaybackTranscoding:  true,
			EnableVideoPlaybackTranscoding:  true,
			EnablePlaybackRemuxing:          true,
			ForceRemoteSourceTranscoding:    false,
			EnableContentDeletion:           false,
			EnableContentDownloading:        true,
			EnableSyncTranscoding:           false,
			EnableMediaConversion:           false,
			EnableAllDevices:                true,
			EnableAllChannels:               true,
			EnableAllFolders:                allFolders,
			EnabledFolders:                  enabledFolders,
			InvalidLoginAttemptCount:        0,
			LoginAttemptsBeforeLockout:      -1,
			MaxActiveSessions:               0,
			EnablePublicSharing:             false,
			RemoteClientBitrateLimit:        0,
			AuthenticationProviderID:        "Default",
			PasswordResetProviderID:         "Default",
			SyncPlayAccess:                  "None",
		},
	}
}

func (s *Server) jfSession(u *store.User, r *http.Request) jellyfin.SessionInfoDto {
	client, deviceName, deviceID, version := jellyfinClientInfo(r)
	now := time.Now().UTC().Format(time.RFC3339)
	return jellyfin.SessionInfoDto{
		ID:                    jellyfin.UserID(u.ID),
		UserID:                jellyfin.UserID(u.ID),
		UserName:              u.DisplayName,
		Client:                client,
		DeviceName:            deviceName,
		DeviceID:              deviceID,
		ApplicationVersion:    version,
		LastActivityDate:      now,
		LastPlaybackCheckIn:   now,
		IsActive:              true,
		SupportsMediaControl:  true,
		SupportsRemoteControl: true,
		PlayableMediaTypes:    []string{"Video"},
		SupportedCommands:     []string{},
		NowPlayingQueue:       []any{},
		PlaylistItems:         []any{},
		PlayState: map[string]any{
			// The SDK marks these as required even for a fresh session.
			"CanSeek":       true,
			"IsPaused":      false,
			"IsMuted":       false,
			"RepeatMode":    "RepeatNone",
			"PlaybackOrder": "Default",
			"VolumeLevel":   100,
		},
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
