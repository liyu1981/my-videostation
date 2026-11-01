// Package metadata defines the provider interface for online metadata (TMDB in M2).
package metadata

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// Settings keys stored in the settings table.
const (
	SettingAPIKey   = "metadata.tmdb_api_key"
	SettingLanguage = "metadata.language"
	SettingRegion   = "metadata.region"
)

// Errors returned by providers and the enrichment service.
var (
	ErrNotConfigured   = errors.New("metadata provider is not configured")
	ErrNoMatch         = errors.New("no confident metadata match")
	ErrUnsupportedKind = errors.New("metadata matching is only available for movies and shows")
)

// Candidate is a search result.
type Candidate struct {
	ProviderID    string  `json:"providerId"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"originalTitle,omitempty"`
	Year          int     `json:"year,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	PosterURL     string  `json:"posterUrl,omitempty"`
	BackdropURL   string  `json:"backdropUrl,omitempty"`
	Rating        float64 `json:"rating,omitempty"`
	Popularity    float64 `json:"popularity,omitempty"`
	MediaType     string  `json:"mediaType"` // movie|tv
}

// Credit is one cast or crew member.
type Credit struct {
	Name      string
	Character string
	Job       string // Director, Writer, Screenplay…
	Ordering  int
}

// Details is a full metadata record for one item.
type Details struct {
	Candidate
	Tagline       string
	ContentRating string
	ReleaseDate   string
	RuntimeMS     int64
	Genres        []string
	Cast          []Credit
	Crew          []Credit
}

// EpisodeDetails is provider metadata for one episode.
type EpisodeDetails struct {
	Title     string
	Overview  string
	AirDate   string
	Rating    float64
	RuntimeMS int64
	StillURL  string
}

// EpisodeProvider is implemented by providers that can fetch an episode by the
// show id + season/episode numbers.
type EpisodeProvider interface {
	EpisodeLang(ctx context.Context, showID string, season, episode int, language string) (*EpisodeDetails, error)
}

// Provider is implemented by TMDB (and by test doubles).
type Provider interface {
	Name() string
	SearchMovies(ctx context.Context, query string, year int) ([]Candidate, error)
	SearchTV(ctx context.Context, query string, year int) ([]Candidate, error)
	Movie(ctx context.Context, id string) (*Details, error)
	TV(ctx context.Context, id string) (*Details, error)
}

// LanguageProvider is optionally implemented by providers that can search in a
// caller-specified language (the edit dialog's language picker).
type LanguageProvider interface {
	SearchMoviesLang(ctx context.Context, query string, year int, language string) ([]Candidate, error)
	SearchTVLang(ctx context.Context, query string, year int, language string) ([]Candidate, error)
}

// LanguageDetailsProvider is optionally implemented by providers that can fetch
// full details in a caller-specified language.
type LanguageDetailsProvider interface {
	MovieLang(ctx context.Context, id string, language string) (*Details, error)
	TVLang(ctx context.Context, id string, language string) (*Details, error)
}

// Factory builds a provider from current settings. It returns ErrNotConfigured when
// no API key has been set.
type Factory func(ctx context.Context) (Provider, error)

// Normalize prepares a title for comparison.
func Normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Score estimates how well a candidate matches a wanted title/year (0..1).
func Score(c Candidate, title string, year int) float64 {
	want := Normalize(title)
	got := Normalize(c.Title)
	orig := Normalize(c.OriginalTitle)

	sim := similarity(want, got)
	if orig != "" {
		if s := similarity(want, orig); s > sim {
			sim = s
		}
	}

	yearScore := 0.5
	if year > 0 && c.Year > 0 {
		diff := year - c.Year
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff == 0:
			yearScore = 1
		case diff == 1:
			yearScore = 0.8
		case diff <= 3:
			yearScore = 0.5
		default:
			yearScore = 0
		}
	}

	pop := 0.0
	if c.Popularity > 0 {
		pop = c.Popularity / (c.Popularity + 40)
	}
	return 0.65*sim + 0.25*yearScore + 0.10*pop
}

// similarity is a token-overlap (Jaccard) score with an exact-match shortcut.
func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	ta := strings.Fields(a)
	tb := strings.Fields(b)
	set := make(map[string]struct{}, len(ta))
	for _, t := range ta {
		set[t] = struct{}{}
	}
	inter := 0
	for _, t := range tb {
		if _, ok := set[t]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
