package jellyfin

// The structs here mirror the subset of the Jellyfin API that the mobile and TV
// clients actually read. Field names are JSON tags exactly as Jellyfin spells
// them (capitalised), because clients deserialize case-insensitively but some
// SDKs inspect raw maps.

// ServerVersion is the Jellyfin server version we claim. Pinned in ADR 0005;
// bump deliberately and re-run the client matrix.
const ServerVersion = "10.10.7"

// ProductName is reported to clients.
const ProductName = "MyVideoStation"

// UserDto is a Jellyfin user.
type UserDto struct {
	Name                  string        `json:"Name"`
	ServerID              string        `json:"ServerId,omitempty"`
	ID                    string        `json:"Id"`
	HasPassword           bool          `json:"HasPassword"`
	HasConfiguredPassword bool          `json:"HasConfiguredPassword"`
	LastLoginDate         string        `json:"LastLoginDate,omitempty"`
	LastActivityDate      string        `json:"LastActivityDate,omitempty"`
	PrimaryImageTag       string        `json:"PrimaryImageTag,omitempty"`
	Configuration         UserConfigDto `json:"Configuration"`
	Policy                UserPolicyDto `json:"Policy"`
}

// UserConfigDto carries playback defaults.
type UserConfigDto struct {
	PlayDefaultAudioTrack      bool     `json:"PlayDefaultAudioTrack"`
	SubtitleLanguagePreference string   `json:"SubtitleLanguagePreference"`
	DisplayMissingEpisodes     bool     `json:"DisplayMissingEpisodes"`
	GroupedFolders             []string `json:"GroupedFolders"`
	SubtitleMode               string   `json:"SubtitleMode"`
	DisplayCollectionsView     bool     `json:"DisplayCollectionsView"`
	EnableLocalPassword        bool     `json:"EnableLocalPassword"`
	OrderedViews               []string `json:"OrderedViews"`
	LatestItemsExcludes        []string `json:"LatestItemsExcludes"`
	MyMediaExcludes            []string `json:"MyMediaExcludes"`
	HidePlayedInLatest         bool     `json:"HidePlayedInLatest"`
	RememberAudioSelections    bool     `json:"RememberAudioSelections"`
	RememberSubtitleSelections bool     `json:"RememberSubtitleSelections"`
	EnableNextEpisodeAutoPlay  bool     `json:"EnableNextEpisodeAutoPlay"`
}

// UserPolicyDto is the subset of permissions clients read.
type UserPolicyDto struct {
	IsAdministrator                 bool     `json:"IsAdministrator"`
	IsHidden                        bool     `json:"IsHidden"`
	IsDisabled                      bool     `json:"IsDisabled"`
	EnableRemoteControlOfOtherUsers bool     `json:"EnableRemoteControlOfOtherUsers"`
	EnableSharedDeviceControl       bool     `json:"EnableSharedDeviceControl"`
	EnableRemoteAccess              bool     `json:"EnableRemoteAccess"`
	EnableLiveTvManagement          bool     `json:"EnableLiveTvManagement"`
	EnableLiveTvAccess              bool     `json:"EnableLiveTvAccess"`
	EnableMediaPlayback             bool     `json:"EnableMediaPlayback"`
	EnableAudioPlaybackTranscoding  bool     `json:"EnableAudioPlaybackTranscoding"`
	EnableVideoPlaybackTranscoding  bool     `json:"EnableVideoPlaybackTranscoding"`
	EnablePlaybackRemuxing          bool     `json:"EnablePlaybackRemuxing"`
	EnableContentDeletion           bool     `json:"EnableContentDeletion"`
	EnableContentDownloading        bool     `json:"EnableContentDownloading"`
	EnableSyncTranscoding           bool     `json:"EnableSyncTranscoding"`
	EnableMediaConversion           bool     `json:"EnableMediaConversion"`
	EnableAllDevices                bool     `json:"EnableAllDevices"`
	EnableAllChannels               bool     `json:"EnableAllChannels"`
	EnableAllFolders                bool     `json:"EnableAllFolders"`
	EnabledFolders                  []string `json:"EnabledFolders"`
	InvalidLoginAttemptCount        int      `json:"InvalidLoginAttemptCount"`
	LoginAttemptsBeforeLockout      int      `json:"LoginAttemptsBeforeLockout"`
	MaxActiveSessions               int      `json:"MaxActiveSessions"`
}

// SessionInfoDto is the session half of an authentication result.
type SessionInfoDto struct {
	ID                  string         `json:"Id"`
	UserID              string         `json:"UserId"`
	UserName            string         `json:"UserName"`
	Client              string         `json:"Client,omitempty"`
	DeviceName          string         `json:"DeviceName,omitempty"`
	DeviceID            string         `json:"DeviceId,omitempty"`
	ApplicationVersion  string         `json:"ApplicationVersion,omitempty"`
	LastActivityDate    string         `json:"LastActivityDate,omitempty"`
	IsActive            bool           `json:"IsActive"`
	PlayableMediaTypes  []string       `json:"PlayableMediaTypes"`
	SupportedCommands   []string       `json:"SupportedCommands"`
	NowPlayingQueue     []any          `json:"NowPlayingQueue"`
	PlayState           map[string]any `json:"PlayState"`
	PlaylistItems       []any          `json:"PlaylistItems"`
	HasCustomDeviceName bool           `json:"HasCustomDeviceName"`
}

// AuthenticationResult is returned by /Users/AuthenticateByName.
type AuthenticationResult struct {
	User        UserDto        `json:"User"`
	SessionInfo SessionInfoDto `json:"SessionInfo"`
	AccessToken string         `json:"AccessToken"`
	ServerID    string         `json:"ServerId"`
}

// PublicSystemInfo is returned by /System/Info/Public.
type PublicSystemInfo struct {
	ServerName               string `json:"ServerName"`
	Version                  string `json:"Version"`
	ProductName              string `json:"ProductName"`
	OperatingSystem          string `json:"OperatingSystem"`
	ID                       string `json:"Id"`
	StartupWizardCompleted   bool   `json:"StartupWizardCompleted"`
	HasPendingRestart        bool   `json:"HasPendingRestart"`
	IsShuttingDown           bool   `json:"IsShuttingDown"`
	SupportsLibraryPlaylists bool   `json:"SupportsLibraryPlaylists"`
	LocalAddress             string `json:"LocalAddress,omitempty"`
}

// SystemInfo is returned by /System/Info.
type SystemInfo struct {
	PublicSystemInfo
	OperatingSystemDisplayName    string `json:"OperatingSystemDisplayName"`
	HasUpdateAvailable            bool   `json:"HasUpdateAvailable"`
	CanSelfRestart                bool   `json:"CanSelfRestart"`
	CanSelfUpdate                 bool   `json:"CanSelfUpdate"`
	CanLaunchWebBrowser           bool   `json:"CanLaunchWebBrowser"`
	WebSocketPortNumber           int    `json:"WebSocketPortNumber"`
	LibraryScanRunning            bool   `json:"LibraryScanRunning"`
	EncoderLocation               string `json:"EncoderLocation"`
	SystemUpdateLevel             string `json:"SystemUpdateLevel"`
	ProgramDataPath               string `json:"ProgramDataPath,omitempty"`
	DataFolder                    string `json:"DataFolder,omitempty"`
	CachePath                     string `json:"CachePath,omitempty"`
	LogPath                       string `json:"LogPath,omitempty"`
	InternalMetadataPath          string `json:"InternalMetadataPath,omitempty"`
	TranscodingTempPath           string `json:"TranscodingTempPath,omitempty"`
	ItemsByNamePath               string `json:"ItemsByNamePath,omitempty"`
	CompletedInstallScheduledTask bool   `json:"CompletedInstallScheduledTask"`
}

// QueryResult is a paged item list.
type QueryResult struct {
	Items            []BaseItemDto `json:"Items"`
	TotalRecordCount int           `json:"TotalRecordCount"`
	StartIndex       int           `json:"StartIndex"`
}

// BaseItemDto covers movies, series, seasons, episodes, extras and library views.
type BaseItemDto struct {
	Name                    string            `json:"Name"`
	ServerID                string            `json:"ServerId,omitempty"`
	ID                      string            `json:"Id"`
	Type                    string            `json:"Type,omitempty"`
	CollectionType          string            `json:"CollectionType,omitempty"`
	MediaType               string            `json:"MediaType,omitempty"`
	IsFolder                bool              `json:"IsFolder"`
	ParentID                string            `json:"ParentId,omitempty"`
	SeriesID                string            `json:"SeriesId,omitempty"`
	SeasonID                string            `json:"SeasonId,omitempty"`
	SeriesName              string            `json:"SeriesName,omitempty"`
	SeasonName              string            `json:"SeasonName,omitempty"`
	SeriesPrimaryImageTag   string            `json:"SeriesPrimaryImageTag,omitempty"`
	ParentBackdropItemID    string            `json:"ParentBackdropItemId,omitempty"`
	ParentBackdropImageTags []string          `json:"ParentBackdropImageTags,omitempty"`
	IndexNumber             *int              `json:"IndexNumber,omitempty"`
	ParentIndexNumber       *int              `json:"ParentIndexNumber,omitempty"`
	ProductionYear          int               `json:"ProductionYear,omitempty"`
	Overview                string            `json:"Overview,omitempty"`
	OfficialRating          string            `json:"OfficialRating,omitempty"`
	CommunityRating         float64           `json:"CommunityRating,omitempty"`
	RunTimeTicks            int64             `json:"RunTimeTicks,omitempty"`
	DateCreated             string            `json:"DateCreated,omitempty"`
	PremiereDate            string            `json:"PremiereDate,omitempty"`
	SortName                string            `json:"SortName,omitempty"`
	Path                    string            `json:"Path,omitempty"`
	Genres                  []string          `json:"Genres,omitempty"`
	People                  []PersonDto       `json:"People,omitempty"`
	ProviderIds             map[string]string `json:"ProviderIds,omitempty"`
	ImageTags               map[string]string `json:"ImageTags,omitempty"`
	BackdropImageTags       []string          `json:"BackdropImageTags"`
	PrimaryImageAspectRatio float64           `json:"PrimaryImageAspectRatio,omitempty"`
	ChildCount              int               `json:"ChildCount,omitempty"`
	RecursiveItemCount      int               `json:"RecursiveItemCount,omitempty"`
	LocationType            string            `json:"LocationType,omitempty"`
	UserData                *UserItemDataDto  `json:"UserData,omitempty"`
	MediaSources            []MediaSourceInfo `json:"MediaSources,omitempty"`
	CanDownload             bool              `json:"CanDownload"`
	CanDelete               bool              `json:"CanDelete"`
	PlayAccess              string            `json:"PlayAccess,omitempty"`
	IsHD                    bool              `json:"IsHD,omitempty"`
	VideoType               string            `json:"VideoType,omitempty"`
}

// PersonDto is one cast/crew credit.
type PersonDto struct {
	Name string `json:"Name"`
	ID   string `json:"Id,omitempty"`
	Role string `json:"Role,omitempty"`
	Type string `json:"Type,omitempty"`
}

// UserItemDataDto is the per-user watch state.
type UserItemDataDto struct {
	Played                bool    `json:"Played"`
	PlaybackPositionTicks int64   `json:"PlaybackPositionTicks"`
	PlayCount             int     `json:"PlayCount"`
	IsFavorite            bool    `json:"IsFavorite"`
	PlayedPercentage      float64 `json:"PlayedPercentage,omitempty"`
	UnplayedItemCount     int     `json:"UnplayedItemCount,omitempty"`
}

// MediaSourceInfo describes one playable file.
type MediaSourceInfo struct {
	ID                         string        `json:"Id"`
	Name                       string        `json:"Name,omitempty"`
	Path                       string        `json:"Path,omitempty"`
	Container                  string        `json:"Container,omitempty"`
	Size                       int64         `json:"Size,omitempty"`
	RunTimeTicks               int64         `json:"RunTimeTicks,omitempty"`
	Bitrate                    int64         `json:"Bitrate,omitempty"`
	SupportsDirectPlay         bool          `json:"SupportsDirectPlay"`
	SupportsDirectStream       bool          `json:"SupportsDirectStream"`
	SupportsTranscoding        bool          `json:"SupportsTranscoding"`
	TranscodingURL             string        `json:"TranscodingUrl,omitempty"`
	TranscodingSubProtocol     string        `json:"TranscodingSubProtocol,omitempty"`
	TranscodingContainer       string        `json:"TranscodingContainer,omitempty"`
	DirectStreamURL            string        `json:"DirectStreamUrl,omitempty"`
	Protocol                   string        `json:"Protocol"`
	VideoType                  string        `json:"VideoType,omitempty"`
	IsRemote                   bool          `json:"IsRemote"`
	ETag                       string        `json:"ETag,omitempty"`
	MediaStreams               []MediaStream `json:"MediaStreams"`
	DefaultAudioStreamIndex    *int          `json:"DefaultAudioStreamIndex,omitempty"`
	DefaultSubtitleStreamIndex *int          `json:"DefaultSubtitleStreamIndex,omitempty"`
}

// MediaStream is a video/audio/subtitle stream.
type MediaStream struct {
	Type                 string  `json:"Type"`
	Index                int     `json:"Index"`
	Codec                string  `json:"Codec,omitempty"`
	Language             string  `json:"Language,omitempty"`
	Title                string  `json:"Title,omitempty"`
	DisplayTitle         string  `json:"DisplayTitle,omitempty"`
	IsDefault            bool    `json:"IsDefault"`
	IsForced             bool    `json:"IsForced"`
	IsExternal           bool    `json:"IsExternal"`
	IsTextSubtitleStream bool    `json:"IsTextSubtitleStream"`
	DeliveryMethod       string  `json:"DeliveryMethod,omitempty"`
	DeliveryURL          string  `json:"DeliveryUrl,omitempty"`
	Width                int     `json:"Width,omitempty"`
	Height               int     `json:"Height,omitempty"`
	Channels             int     `json:"Channels,omitempty"`
	ChannelLayout        string  `json:"ChannelLayout,omitempty"`
	SampleRate           int     `json:"SampleRate,omitempty"`
	BitRate              int64   `json:"BitRate,omitempty"`
	PixelFormat          string  `json:"PixelFormat,omitempty"`
	Profile              string  `json:"Profile,omitempty"`
	Level                float64 `json:"Level,omitempty"`
}

// SearchHint is one search result.
type SearchHint struct {
	ItemID          string `json:"ItemId"`
	ID              string `json:"Id"`
	Name            string `json:"Name"`
	MatchedTerm     string `json:"MatchedTerm,omitempty"`
	IndexNumber     *int   `json:"IndexNumber,omitempty"`
	ProductionYear  int    `json:"ProductionYear,omitempty"`
	Type            string `json:"Type,omitempty"`
	MediaType       string `json:"MediaType,omitempty"`
	RunTimeTicks    int64  `json:"RunTimeTicks,omitempty"`
	ParentID        string `json:"ParentId,omitempty"`
	SeriesID        string `json:"SeriesId,omitempty"`
	PrimaryImageTag string `json:"PrimaryImageTag,omitempty"`
}

// NameGuidPair is a filter value with a server-generated id (genres, persons).
type NameGuidPair struct {
	Name string `json:"Name"`
	ID   string `json:"Id"`
}

// QueryFilters is returned by /Items/Filters2.
type QueryFilters struct {
	Genres          []NameGuidPair `json:"Genres"`
	Tags            []string       `json:"Tags"`
	OfficialRatings []string       `json:"OfficialRatings"`
	Years           []int          `json:"Years"`
	StudioIDs       []string       `json:"StudioIds"`
}

// LegacyQueryFilters is returned by /Items/Filters.
type LegacyQueryFilters struct {
	Genres          []string `json:"Genres"`
	Tags            []string `json:"Tags"`
	OfficialRatings []string `json:"OfficialRatings"`
	Years           []int    `json:"Years"`
}

// ImageInfo describes one image served for an item.
type ImageInfo struct {
	ImageType  string `json:"ImageType"`
	ImageIndex int    `json:"ImageIndex"`
	Width      int    `json:"Width"`
	Height     int    `json:"Height"`
}

// SearchHintResult is returned by /Search/Hints.
type SearchHintResult struct {
	SearchHints      []SearchHint `json:"SearchHints"`
	TotalRecordCount int          `json:"TotalRecordCount"`
}

// BrandingConfiguration is the login-screen branding payload.
type BrandingConfiguration struct {
	LoginDisclaimer     string `json:"LoginDisclaimer"`
	CustomCSS           string `json:"CustomCss"`
	SplashscreenEnabled bool   `json:"SplashscreenEnabled"`
}
