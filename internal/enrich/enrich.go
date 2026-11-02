// Package enrich matches library items against a metadata provider and caches artwork.
package enrich

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/metadata"
	"github.com/liyu1981/myvideostation/internal/store"
	"github.com/liyu1981/myvideostation/internal/textutil"
)

// MinConfidence is the score below which an automatic search result is rejected.
// Manual picks always win; the lower threshold is the first-pass best effort.
const MinConfidence = 0.35

// Service enriches items and caches artwork under the data dir.
type Service struct {
	store     *store.Store
	providers *metadata.Registry
	cacheDir  string
	log       *slog.Logger
	client    *http.Client
}

// New creates the service with a single TMDb-compatible provider factory.
func New(st *store.Store, factory metadata.Factory, cacheDir string, log *slog.Logger) *Service {
	reg := metadata.NewRegistry()
	reg.Register("tmdb", factory)
	return NewWithRegistry(st, reg, cacheDir, log)
}

// NewWithRegistry creates the service with several metadata providers/plugins.
func NewWithRegistry(st *store.Store, providers *metadata.Registry, cacheDir string, log *slog.Logger) *Service {
	return &Service{
		store:     st,
		providers: providers,
		cacheDir:  cacheDir,
		log:       log,
		client:    &http.Client{Timeout: 45 * time.Second},
	}
}

// Stats summarises a library enrichment run.
type Stats struct {
	Considered int `json:"considered"`
	Matched    int `json:"matched"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
}

func (s Stats) String() string {
	return fmt.Sprintf("%d items: %d matched, %d failed, %d skipped", s.Considered, s.Matched, s.Failed, s.Skipped)
}

// EnrichLibrary matches every currently-unmatched item in a library (0 = all libraries).
func (s *Service) EnrichLibrary(ctx context.Context, libraryID int64, report func(float64, string)) (Stats, error) {
	var stats Stats
	if report == nil {
		report = func(float64, string) {}
	}
	provider, err := s.providers.Get(ctx, "")
	if err != nil {
		return stats, err
	}
	ids, err := s.store.UnmatchedItemIDs(ctx, libraryID)
	if err != nil {
		return stats, err
	}
	stats.Considered = len(ids)
	if len(ids) == 0 {
		report(100, "nothing to match")
		return stats, nil
	}

	for i, id := range ids {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		if err := s.enrichOne(ctx, provider, id, true, ""); err != nil {
			if err == metadata.ErrNoMatch {
				stats.Failed++
			} else {
				stats.Failed++
				s.log.Warn("enrich failed", "item", id, "err", err)
			}
		} else {
			stats.Matched++
		}
		report(float64(i+1)/float64(len(ids))*100, fmt.Sprintf("matching %d/%d", i+1, len(ids)))
	}
	report(100, stats.String())
	return stats, nil
}

// Refresh re-fetches one item from its stored provider id. providerName selects the
// provider ("" = the item's applied provider, then the default).
func (s *Service) Refresh(ctx context.Context, itemID int64, language, providerName string) error {
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if providerName == "" && item.MetadataStatus == "local" && item.ProviderID == "" {
		return nil
	}
	name := providerName
	if name == "" {
		name = item.Provider
	}
	if !s.providers.Has(name) {
		name = s.providers.DefaultName()
	}
	provider, err := s.providers.Get(ctx, name)
	if err != nil {
		return err
	}
	id := ""
	if item.Provider == provider.Name() {
		id = item.ProviderID
	}
	if id == "" {
		var lookupErr error
		id, lookupErr = s.store.ItemProviderID(ctx, itemID, provider.Name())
		if lookupErr != nil {
			return lookupErr
		}
	}
	if id == "" {
		return metadata.ErrNoMatch
	}
	var d *metadata.Details
	switch item.Kind {
	case store.KindEpisode:
		d, err = s.episodeDetails(ctx, provider, item, id, language, item.SeasonNumber, item.EpisodeNumber)
	case store.KindSeason:
		d, err = s.seasonDetails(ctx, provider, item, id, language, item.SeasonNumber)
	default:
		d, err = s.detailsLang(ctx, provider, item, id, language)
	}
	if err != nil {
		return err
	}
	status := item.MetadataStatus
	if status == "" || status == "unmatched" || status == "local" || status == "failed" {
		status = "matched"
	}
	return s.apply(ctx, item, provider, d, status, item.MatchConfidence, nil)
}

// ProviderInfos reports the configured providers for the UI selector.
func (s *Service) ProviderInfos(ctx context.Context) []ProviderInfo {
	var out []ProviderInfo
	for _, name := range s.providers.Names() {
		info := ProviderInfo{Name: name}
		if _, err := s.providers.Get(ctx, name); err == nil {
			info.Configured = true
		}
		info.Kinds = []string{"movie", "show", "season", "episode"}
		if provider, err := s.providers.Get(ctx, name); err == nil {
			if kp, ok := provider.(metadata.KindsProvider); ok {
				info.Kinds = kp.Kinds()
			}
		}
		out = append(out, info)
	}
	return out
}

// ProviderInfo describes one provider.
type ProviderInfo struct {
	Name       string   `json:"name"`
	Configured bool     `json:"configured"`
	Kinds      []string `json:"kinds"`
}

// SetArtFromURL downloads an image and assigns it as the poster or backdrop.
func (s *Service) SetArtFromURL(ctx context.Context, itemID int64, kind, url string) error {
	if url == "" {
		return fmt.Errorf("image url is required")
	}
	if _, err := s.store.GetItem(ctx, itemID); err != nil {
		return err
	}
	path, err := s.cacheImage(ctx, url)
	if err != nil {
		return err
	}
	return s.store.SetArt(ctx, itemID, kind, path)
}

// Search returns provider candidates for an item, best first.
func (s *Service) Search(ctx context.Context, itemID int64, query string, year int, language, providerName string) ([]metadata.Candidate, error) {
	provider, err := s.providers.Get(ctx, providerName)
	if err != nil {
		return nil, err
	}
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	searchTV := item.Kind == store.KindShow
	if item.Kind == store.KindSeason {
		show, err := s.showForSeason(ctx, item)
		if err != nil {
			return nil, err
		}
		if show == nil {
			return nil, metadata.ErrNoMatch
		}
		searchTV = true
		if query == "" {
			query = show.Title
		}
	} else if item.Kind == store.KindEpisode {
		show, err := s.showForEpisode(ctx, item)
		if err != nil {
			return nil, err
		}
		if show == nil {
			return nil, metadata.ErrNoMatch
		}
		searchTV = true
		if query == "" {
			query = show.Title
		}
	} else if item.Kind != store.KindMovie && item.Kind != store.KindShow {
		return nil, metadata.ErrUnsupportedKind
	}
	if query == "" {
		query = item.Title
	}
	// Manual searches are deliberately broad: do not filter by the item's stored
	// year, otherwise re-searching a matched item returns only that year's results.
	var candidates []metadata.Candidate
	if lp, ok := provider.(metadata.LanguageProvider); ok && language != "" {
		if searchTV {
			candidates, err = lp.SearchTVLang(ctx, query, year, language)
		} else {
			candidates, err = lp.SearchMoviesLang(ctx, query, year, language)
		}
	} else if searchTV {
		candidates, err = provider.SearchTV(ctx, query, year)
	} else {
		candidates, err = provider.SearchMovies(ctx, query, year)
	}
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		candidates[i].MediaType = mediaTypeFor(item.Kind)
		if candidates[i].Provider == "" {
			candidates[i].Provider = provider.Name()
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return metadata.Score(candidates[i], query, year) > metadata.Score(candidates[j], query, year)
	})
	return candidates, nil
}

// Match applies a user-chosen provider id (manual identification). language
// selects the provider details language ("" = configured default).
func (s *Service) Match(ctx context.Context, itemID int64, providerID string, language string, season, episode int, providerName string) error {
	provider, err := s.providers.Get(ctx, providerName)
	if err != nil {
		return err
	}
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.Kind == store.KindSeason {
		show, err := s.showForSeason(ctx, item)
		if err != nil {
			return err
		}
		if show == nil || item.SeasonNumber == 0 {
			return metadata.ErrNoMatch
		}
		sn := item.SeasonNumber
		if season > 0 {
			sn = season
		}
		d, err := s.seasonDetails(ctx, provider, item, providerID, language, sn)
		if err != nil {
			return err
		}
		return s.apply(ctx, item, provider, d, "manual", 1, nil)
	}
	if item.Kind == store.KindEpisode {
		show, err := s.showForEpisode(ctx, item)
		if err != nil {
			return err
		}
		if show == nil {
			return metadata.ErrNoMatch
		}
		if season <= 0 {
			season = item.SeasonNumber
		}
		if episode <= 0 {
			episode = item.EpisodeNumber
		}
		d, err := s.episodeDetails(ctx, provider, item, providerID, language, season, episode)
		if err != nil {
			return err
		}
		return s.apply(ctx, item, provider, d, "manual", 1, nil)
	}
	if item.Kind != store.KindMovie && item.Kind != store.KindShow {
		return metadata.ErrUnsupportedKind
	}
	details, err := s.detailsLang(ctx, provider, item, providerID, language)
	if err != nil {
		return err
	}
	// Explicit user choice overrides locks.
	return s.apply(ctx, item, provider, details, "manual", 1, nil)
}

func (s *Service) enrichOne(ctx context.Context, provider metadata.Provider, itemID int64, _ bool, language string) error {
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.Kind == store.KindSeason {
		if item.ProviderID == "" {
			return metadata.ErrNoMatch
		}
		d, err := s.seasonDetails(ctx, provider, item, item.ProviderID, language, item.SeasonNumber)
		if err != nil {
			return err
		}
		return s.apply(ctx, item, provider, d, item.MetadataStatus, item.MatchConfidence, nil)
	}
	if item.Kind == store.KindEpisode {
		if item.ProviderID == "" {
			return metadata.ErrNoMatch
		}
		d, err := s.episodeDetails(ctx, provider, item, item.ProviderID, language, item.SeasonNumber, item.EpisodeNumber)
		if err != nil {
			return err
		}
		return s.apply(ctx, item, provider, d, item.MetadataStatus, item.MatchConfidence, nil)
	}
	if item.Kind != store.KindMovie && item.Kind != store.KindShow {
		return metadata.ErrNoMatch
	}
	// Local .nfo/.vsmeta metadata wins over automatic matching; manual Identify
	// still overrides it explicitly.
	if item.MetadataStatus == "local" {
		return nil
	}

	locks, err := s.lockSet(ctx, item)
	if err != nil {
		return err
	}

	// Refresh path: we already know the provider id and the match is trusted.
	if item.ProviderID != "" && item.Provider != "" && (item.MetadataStatus == "matched" || item.MetadataStatus == "manual") {
		details, err := s.detailsLang(ctx, provider, item, item.ProviderID, language)
		if err != nil {
			return err
		}
		return s.apply(ctx, item, provider, details, item.MetadataStatus, item.MatchConfidence, locks)
	}

	var candidates []metadata.Candidate
	if lp, ok := provider.(metadata.LanguageProvider); ok && language != "" {
		if item.Kind == store.KindShow {
			candidates, err = lp.SearchTVLang(ctx, item.Title, item.Year, language)
		} else {
			candidates, err = lp.SearchMoviesLang(ctx, item.Title, item.Year, language)
		}
	} else if item.Kind == store.KindShow {
		candidates, err = provider.SearchTV(ctx, item.Title, item.Year)
	} else {
		candidates, err = provider.SearchMovies(ctx, item.Title, item.Year)
	}
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return metadata.ErrNoMatch
	}

	best := candidates[0]
	bestScore := metadata.Score(best, item.Title, item.Year)
	for _, c := range candidates[1:] {
		if sc := metadata.Score(c, item.Title, item.Year); sc > bestScore {
			best, bestScore = c, sc
		}
	}
	if bestScore < MinConfidence {
		return metadata.ErrNoMatch
	}

	details, err := s.detailsLang(ctx, provider, item, best.ProviderID, language)
	if err != nil {
		return err
	}
	return s.apply(ctx, item, provider, details, "matched", bestScore, locks)
}

// episodeDetails fetches one episode from the provider (showID is the TMDb show id).
func (s *Service) episodeDetails(ctx context.Context, provider metadata.Provider, item *store.Item, showID, language string, season, episode int) (*metadata.Details, error) {
	ep, ok := provider.(metadata.EpisodeProvider)
	if !ok {
		return nil, metadata.ErrUnsupportedKind
	}
	ed, err := ep.EpisodeLang(ctx, showID, season, episode, language)
	if err != nil {
		return nil, err
	}
	return &metadata.Details{
		Candidate: metadata.Candidate{
			ProviderID:  showID,
			Title:       ed.Title,
			Overview:    ed.Overview,
			Rating:      ed.Rating,
			BackdropURL: ed.StillURL,
			Year:        yearOfDate(ed.AirDate),
			MediaType:   "tv",
		},
		ReleaseDate: ed.AirDate,
		RuntimeMS:   ed.RuntimeMS,
	}, nil
}

// seasonDetails fetches one season from the provider (showID is the TMDb show id).
func (s *Service) seasonDetails(ctx context.Context, provider metadata.Provider, item *store.Item, showID, language string, season int) (*metadata.Details, error) {
	sp, ok := provider.(metadata.SeasonProvider)
	if !ok {
		return nil, metadata.ErrUnsupportedKind
	}
	sd, err := sp.SeasonLang(ctx, showID, season, language)
	if err != nil {
		return nil, err
	}
	return &metadata.Details{
		Candidate: metadata.Candidate{
			ProviderID: showID,
			Title:      sd.Title,
			Overview:   sd.Overview,
			Rating:     sd.Rating,
			PosterURL:  sd.PosterURL,
			Year:       yearOfDate(sd.AirDate),
			MediaType:  "tv",
		},
		ReleaseDate: sd.AirDate,
	}, nil
}

// showForSeason returns the parent show of a season.
func (s *Service) showForSeason(ctx context.Context, item *store.Item) (*store.Item, error) {
	if item.Kind != store.KindSeason || item.ParentID == 0 {
		return nil, nil
	}
	return s.store.GetItem(ctx, item.ParentID)
}

// showForEpisode walks episode -> season -> show.
func (s *Service) showForEpisode(ctx context.Context, item *store.Item) (*store.Item, error) {
	if item.Kind != store.KindEpisode || item.ParentID == 0 {
		return nil, nil
	}
	season, err := s.store.GetItem(ctx, item.ParentID)
	if err != nil {
		return nil, err
	}
	if season.Kind != store.KindSeason || season.ParentID == 0 {
		return nil, nil
	}
	return s.store.GetItem(ctx, season.ParentID)
}

func yearOfDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil || y < 1800 {
		return 0
	}
	return y
}

func (s *Service) detailsLang(ctx context.Context, provider metadata.Provider, item *store.Item, providerID, language string) (*metadata.Details, error) {
	if language != "" {
		if lp, ok := provider.(metadata.LanguageDetailsProvider); ok {
			if item.Kind == store.KindShow {
				return lp.TVLang(ctx, providerID, language)
			}
			return lp.MovieLang(ctx, providerID, language)
		}
	}
	if item.Kind == store.KindShow {
		return provider.TV(ctx, providerID)
	}
	return provider.Movie(ctx, providerID)
}

func (s *Service) lockSet(ctx context.Context, item *store.Item) (map[string]bool, error) {
	locks := map[string]bool{}
	if item.Locked {
		for _, f := range []string{"title", "original_title", "year", "overview", "tagline", "rating", "content_rating", "genres", "poster", "backdrop", "cast", "crew"} {
			locks[f] = true
		}
		return locks, nil
	}
	fields, err := s.store.ItemLocks(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		locks[f] = true
	}
	return locks, nil
}

// apply turns provider details into an Enrichment, honouring locks.
func (s *Service) apply(ctx context.Context, item *store.Item, provider metadata.Provider,
	d *metadata.Details, status string, confidence float64, locks map[string]bool) error {

	e := store.Enrichment{
		Provider:   provider.Name(),
		ProviderID: d.ProviderID,
		Status:     status,
		Confidence: confidence,
	}
	locked := func(field string) bool { return locks != nil && locks[field] }

	if !locked("title") && strings.TrimSpace(d.Title) != "" {
		e.Title = strPtr(d.Title)
	}
	if !locked("original_title") && strings.TrimSpace(d.OriginalTitle) != "" {
		e.OriginalTitle = strPtr(d.OriginalTitle)
	}
	if !locked("year") && d.Year > 0 {
		e.Year = intPtr(d.Year)
	}
	if !locked("overview") && strings.TrimSpace(d.Overview) != "" {
		e.Overview = strPtr(textutil.NormalizeOverview(d.Overview))
	}
	if !locked("tagline") && strings.TrimSpace(d.Tagline) != "" {
		e.Tagline = strPtr(d.Tagline)
	}
	if !locked("rating") && d.Rating > 0 {
		e.Rating = floatPtr(d.Rating)
	}
	if !locked("content_rating") && d.ContentRating != "" {
		e.ContentRating = strPtr(d.ContentRating)
	}
	if !locked("release_date") && d.ReleaseDate != "" {
		e.ReleaseDate = strPtr(d.ReleaseDate)
	}
	if d.RuntimeMS > 0 {
		e.RuntimeMS = int64Ptr(d.RuntimeMS)
	}
	if !locked("genres") && len(d.Genres) > 0 {
		e.Genres = d.Genres
	}
	if !locked("cast") && d.Cast != nil {
		e.Cast = make([]store.Person, 0, len(d.Cast))
		for i, c := range d.Cast {
			e.Cast = append(e.Cast, store.Person{Name: c.Name, Role: "cast", Character: c.Character, Ordering: i})
		}
	}
	if !locked("crew") && d.Crew != nil {
		e.Crew = make([]store.Person, 0, len(d.Crew))
		for i, c := range d.Crew {
			role := "director"
			switch strings.ToLower(c.Job) {
			case "writer", "screenplay", "creator", "story":
				role = "writer"
			}
			e.Crew = append(e.Crew, store.Person{Name: c.Name, Role: role, Ordering: i})
		}
	}

	if !locked("poster") && d.PosterURL != "" {
		if p, err := s.cacheImage(ctx, d.PosterURL); err == nil {
			e.PosterPath = strPtr(p)
		} else {
			s.log.Warn("poster download failed", "item", item.ID, "err", err)
		}
	}
	if !locked("backdrop") && d.BackdropURL != "" {
		if p, err := s.cacheImage(ctx, d.BackdropURL); err == nil {
			e.BackdropPath = strPtr(p)
		} else {
			s.log.Warn("backdrop download failed", "item", item.ID, "err", err)
		}
	}

	if err := s.store.ApplyEnrichment(ctx, item.ID, e); err != nil {
		return err
	}
	s.log.Info("metadata applied", "item", item.ID, "title", d.Title, "provider", provider.Name(), "status", status)
	return nil
}

// cacheImage downloads an image into <cache>/images and returns its path.
func (s *Service) cacheImage(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("unsupported image url %q", url)
	}
	dir := filepath.Join(s.cacheDir, "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(url))
	base := hex.EncodeToString(sum[:])

	// Reuse an existing file with any known extension.
	for _, ext := range []string{".jpg", ".png", ".webp"} {
		existing := filepath.Join(dir, base+ext)
		if st, err := os.Stat(existing); err == nil && st.Size() > 0 {
			return existing, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image status %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("unexpected content type %q", ct)
	}
	ext := ".jpg"
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "webp"):
		ext = ".webp"
	}
	final := filepath.Join(dir, base+ext)
	tmp, err := os.CreateTemp(dir, "img-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(res.Body, 20<<20)); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}

func mediaTypeFor(kind string) string {
	if kind == store.KindShow {
		return "tv"
	}
	return "movie"
}

func strPtr(s string) *string     { return &s }
func intPtr(i int) *int           { return &i }
func int64Ptr(i int64) *int64     { return &i }
func floatPtr(f float64) *float64 { return &f }
