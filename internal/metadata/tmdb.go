package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TMDB is a The Movie Database client (v3 API key or v4 bearer token).
type TMDB struct {
	client    *http.Client
	apiKey    string
	baseURL   string
	imageBase string
	language  string
	region    string
}

// NewTMDB builds a client. language defaults to en-US, region to US.
func NewTMDB(apiKey, language, region string) *TMDB {
	if language == "" {
		language = "en-US"
	}
	if region == "" {
		region = "US"
	}
	return &TMDB{
		client:    &http.Client{Timeout: 20 * time.Second},
		apiKey:    strings.TrimSpace(apiKey),
		baseURL:   "https://api.themoviedb.org/3",
		imageBase: "https://image.tmdb.org/t/p",
		language:  language,
		region:    region,
	}
}

// Name implements Provider.
func (t *TMDB) Name() string { return "tmdb" }

// WithBaseURLs overrides endpoints (used by tests).
func (t *TMDB) WithBaseURLs(apiBase, imageBase string) *TMDB {
	if apiBase != "" {
		t.baseURL = strings.TrimRight(apiBase, "/")
	}
	if imageBase != "" {
		t.imageBase = strings.TrimRight(imageBase, "/")
	}
	return t
}

func (t *TMDB) image(path, size string) string {
	if path == "" {
		return ""
	}
	return t.imageBase + "/" + size + path
}

func (t *TMDB) get(ctx context.Context, path string, params url.Values, dst any) error {
	if t.apiKey == "" {
		return ErrNotConfigured
	}
	if params == nil {
		params = url.Values{}
	}
	if params.Get("language") == "" {
		params.Set("language", t.language)
	}
	u := t.baseURL + path + "?" + params.Encode()

	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		// v4 tokens are JWTs; v3 keys go in the query string.
		if strings.Count(t.apiKey, ".") == 2 {
			req.Header.Set("Authorization", "Bearer "+t.apiKey)
		} else {
			q := req.URL.Query()
			q.Set("api_key", t.apiKey)
			req.URL.RawQuery = q.Encode()
		}
		return req, nil
	}

	// Retry transient TMDb failures (429/5xx/network) with a short backoff; a
	// flaky provider response should not surface as a 502 in the edit dialog.
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		req, err := build()
		if err != nil {
			return err
		}
		res, err := t.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("tmdb %s: %w", path, err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			if err := json.Unmarshal(body, dst); err != nil {
				return fmt.Errorf("tmdb %s: decode: %w", path, err)
			}
			return nil
		}
		lastErr = fmt.Errorf("tmdb %s: status %d: %s", path, res.StatusCode, strings.TrimSpace(string(body[:min(len(body), 512)])))
		if res.StatusCode != http.StatusTooManyRequests && res.StatusCode < 500 {
			break
		}
	}
	return lastErr
}

type tmdbSearch struct {
	Results []struct {
		ID            int64   `json:"id"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		Overview      string  `json:"overview"`
		PosterPath    string  `json:"poster_path"`
		BackdropPath  string  `json:"backdrop_path"`
		VoteAverage   float64 `json:"vote_average"`
		Popularity    float64 `json:"popularity"`
	} `json:"results"`
}

// SearchMovies implements Provider.
func (t *TMDB) SearchMovies(ctx context.Context, query string, year int) ([]Candidate, error) {
	return t.SearchMoviesLang(ctx, query, year, "")
}

// SearchMoviesLang searches movies in a specific language ("" = configured default).
func (t *TMDB) SearchMoviesLang(ctx context.Context, query string, year int, language string) ([]Candidate, error) {
	params := url.Values{}
	params.Set("query", query)
	if language != "" {
		params.Set("language", language)
	}
	params.Set("include_adult", "false")
	if year > 0 {
		params.Set("year", strconv.Itoa(year))
	}
	var out tmdbSearch
	if err := t.get(ctx, "/search/movie", params, &out); err != nil {
		return nil, err
	}
	res := make([]Candidate, 0, len(out.Results))
	for _, r := range out.Results {
		res = append(res, Candidate{
			ProviderID:    strconv.FormatInt(r.ID, 10),
			Title:         r.Title,
			OriginalTitle: r.OriginalTitle,
			Year:          yearOf(r.ReleaseDate),
			Overview:      r.Overview,
			PosterURL:     t.image(r.PosterPath, "w500"),
			BackdropURL:   t.image(r.BackdropPath, "w1280"),
			Rating:        r.VoteAverage,
			Popularity:    r.Popularity,
			MediaType:     "movie",
			Provider:      "tmdb",
		})
	}
	return res, nil
}

// SearchTV implements Provider.
func (t *TMDB) SearchTV(ctx context.Context, query string, year int) ([]Candidate, error) {
	return t.SearchTVLang(ctx, query, year, "")
}

// SearchTVLang searches shows in a specific language ("" = configured default).
func (t *TMDB) SearchTVLang(ctx context.Context, query string, year int, language string) ([]Candidate, error) {
	params := url.Values{}
	params.Set("query", query)
	if language != "" {
		params.Set("language", language)
	}
	params.Set("include_adult", "false")
	if year > 0 {
		params.Set("first_air_date_year", strconv.Itoa(year))
	}
	var out tmdbSearch
	if err := t.get(ctx, "/search/tv", params, &out); err != nil {
		return nil, err
	}
	res := make([]Candidate, 0, len(out.Results))
	for _, r := range out.Results {
		res = append(res, Candidate{
			ProviderID:    strconv.FormatInt(r.ID, 10),
			Title:         r.Name,
			OriginalTitle: r.OriginalName,
			Year:          yearOf(r.FirstAirDate),
			Overview:      r.Overview,
			PosterURL:     t.image(r.PosterPath, "w500"),
			BackdropURL:   t.image(r.BackdropPath, "w1280"),
			Rating:        r.VoteAverage,
			Popularity:    r.Popularity,
			MediaType:     "tv",
			Provider:      "tmdb",
		})
	}
	return res, nil
}

type tmdbDetails struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Name          string  `json:"name"`
	OriginalTitle string  `json:"original_title"`
	OriginalName  string  `json:"original_name"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	Overview      string  `json:"overview"`
	Tagline       string  `json:"tagline"`
	Runtime       int     `json:"runtime"`
	EpisodeRuns   []int   `json:"episode_run_time"`
	VoteAverage   float64 `json:"vote_average"`
	Popularity    float64 `json:"popularity"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	Genres        []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Credits struct {
		Cast []struct {
			Name      string `json:"name"`
			Character string `json:"character"`
			Order     int    `json:"order"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	ReleaseDates struct {
		Results []struct {
			ISO      string `json:"iso_3166_1"`
			Releases []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Results []struct {
			ISO   string `json:"iso_3166_1"`
			Value string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

func (t *TMDB) details(ctx context.Context, path string, mediaType string) (*Details, error) {
	return t.detailsLang(ctx, path, mediaType, "")
}

func (t *TMDB) detailsLang(ctx context.Context, path string, mediaType string, language string) (*Details, error) {
	var d tmdbDetails
	params := url.Values{}
	appendTo := "credits,release_dates,content_ratings"
	if mediaType == "tv" {
		appendTo = "credits,content_ratings"
	}
	params.Set("append_to_response", appendTo)
	if language != "" {
		params.Set("language", language)
	}
	if err := t.get(ctx, path, params, &d); err != nil {
		return nil, err
	}

	title := d.Title
	original := d.OriginalTitle
	date := d.ReleaseDate
	if mediaType == "tv" {
		title, original, date = d.Name, d.OriginalName, d.FirstAirDate
	}
	out := &Details{
		Candidate: Candidate{
			ProviderID:    strconv.FormatInt(d.ID, 10),
			Title:         title,
			OriginalTitle: original,
			Year:          yearOf(date),
			Overview:      d.Overview,
			PosterURL:     t.image(d.PosterPath, "w500"),
			BackdropURL:   t.image(d.BackdropPath, "w1280"),
			Rating:        d.VoteAverage,
			Popularity:    d.Popularity,
			MediaType:     mediaType,
			Provider:      "tmdb",
		},
		Tagline:     d.Tagline,
		ReleaseDate: date,
	}
	out.RuntimeMS = int64(d.Runtime) * 60_000
	if mediaType == "tv" && len(d.EpisodeRuns) > 0 {
		out.RuntimeMS = int64(d.EpisodeRuns[0]) * 60_000
	}
	for _, g := range d.Genres {
		out.Genres = append(out.Genres, g.Name)
	}
	for i, c := range d.Credits.Cast {
		if i >= 15 {
			break
		}
		out.Cast = append(out.Cast, Credit{Name: c.Name, Character: c.Character, Ordering: c.Order})
	}
	for _, c := range d.Credits.Crew {
		switch strings.ToLower(c.Job) {
		case "director", "writer", "screenplay", "creator":
			out.Crew = append(out.Crew, Credit{Name: c.Name, Job: c.Job})
		}
	}
	out.ContentRating = certification(mediaType, d, t.region)
	return out, nil
}

// Movie implements Provider.
func (t *TMDB) Movie(ctx context.Context, id string) (*Details, error) {
	return t.details(ctx, "/movie/"+url.PathEscape(id), "movie")
}

// TV implements Provider.
func (t *TMDB) TV(ctx context.Context, id string) (*Details, error) {
	return t.details(ctx, "/tv/"+url.PathEscape(id), "tv")
}

type tmdbEpisode struct {
	Name        string  `json:"name"`
	Overview    string  `json:"overview"`
	AirDate     string  `json:"air_date"`
	VoteAverage float64 `json:"vote_average"`
	Runtime     int     `json:"runtime"`
	StillPath   string  `json:"still_path"`
}

// EpisodeLang fetches one episode of a show in a specific language.
func (t *TMDB) EpisodeLang(ctx context.Context, showID string, season, episode int, language string) (*EpisodeDetails, error) {
	params := url.Values{}
	if language != "" {
		params.Set("language", language)
	}
	path := "/tv/" + url.PathEscape(showID) + "/season/" + strconv.Itoa(season) + "/episode/" + strconv.Itoa(episode)
	var e tmdbEpisode
	if err := t.get(ctx, path, params, &e); err != nil {
		return nil, err
	}
	return &EpisodeDetails{
		Title:     e.Name,
		Overview:  e.Overview,
		AirDate:   e.AirDate,
		Rating:    e.VoteAverage,
		RuntimeMS: int64(e.Runtime) * 60_000,
		StillURL:  t.image(e.StillPath, "w1280"),
	}, nil
}

type tmdbSeason struct {
	Name        string  `json:"name"`
	Overview    string  `json:"overview"`
	AirDate     string  `json:"air_date"`
	VoteAverage float64 `json:"vote_average"`
	PosterPath  string  `json:"poster_path"`
}

// SeasonLang fetches one season of a show in a specific language.
func (t *TMDB) SeasonLang(ctx context.Context, showID string, season int, language string) (*SeasonDetails, error) {
	params := url.Values{}
	if language != "" {
		params.Set("language", language)
	}
	path := "/tv/" + url.PathEscape(showID) + "/season/" + strconv.Itoa(season)
	var s tmdbSeason
	if err := t.get(ctx, path, params, &s); err != nil {
		return nil, err
	}
	return &SeasonDetails{
		Title:     s.Name,
		Overview:  s.Overview,
		AirDate:   s.AirDate,
		Rating:    s.VoteAverage,
		PosterURL: t.image(s.PosterPath, "w500"),
	}, nil
}

// MovieLang fetches movie details in a specific language ("" = configured default).
func (t *TMDB) MovieLang(ctx context.Context, id string, language string) (*Details, error) {
	return t.detailsLang(ctx, "/movie/"+url.PathEscape(id), "movie", language)
}

// TVLang fetches show details in a specific language ("" = configured default).
func (t *TMDB) TVLang(ctx context.Context, id string, language string) (*Details, error) {
	return t.detailsLang(ctx, "/tv/"+url.PathEscape(id), "tv", language)
}

func certification(mediaType string, d tmdbDetails, region string) string {
	if mediaType == "tv" {
		for _, r := range d.ContentRatings.Results {
			if r.ISO == region && r.Value != "" {
				return r.Value
			}
		}
		for _, r := range d.ContentRatings.Results {
			if r.Value != "" {
				return r.Value
			}
		}
		return ""
	}
	for _, r := range d.ReleaseDates.Results {
		if r.ISO != region {
			continue
		}
		for _, rel := range r.Releases {
			if rel.Certification != "" {
				return rel.Certification
			}
		}
	}
	for _, r := range d.ReleaseDates.Results {
		for _, rel := range r.Releases {
			if rel.Certification != "" {
				return rel.Certification
			}
		}
	}
	return ""
}

func yearOf(date string) int {
	if len(date) >= 4 {
		if y, err := strconv.Atoi(date[:4]); err == nil {
			return y
		}
	}
	return 0
}
