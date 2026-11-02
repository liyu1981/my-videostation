package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTMDBClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "test-key" {
			t.Errorf("api_key = %q", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("query") != "the king's speech" {
			t.Errorf("query = %q", r.URL.Query().Get("query"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id": 45269, "title": "The King's Speech", "original_title": "The King's Speech",
				"release_date": "2010-11-26", "overview": "A king finds his voice.",
				"poster_path": "/poster.jpg", "backdrop_path": "/backdrop.jpg",
				"vote_average": 7.7, "popularity": 42.2,
			}},
		})
	})
	mux.HandleFunc("/movie/45269", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 45269, "title": "The King's Speech", "release_date": "2010-11-26",
			"runtime": 118, "tagline": "Find your voice.", "vote_average": 7.7,
			"poster_path": "/poster.jpg", "backdrop_path": "/backdrop.jpg",
			"genres": []map[string]any{{"name": "Drama"}, {"name": "History"}},
			"credits": map[string]any{
				"cast": []map[string]any{
					{"name": "Colin Firth", "character": "King George VI", "order": 0},
					{"name": "Geoffrey Rush", "character": "Lionel Logue", "order": 1},
				},
				"crew": []map[string]any{
					{"name": "Tom Hooper", "job": "Director"},
					{"name": "David Seidler", "job": "Screenplay"},
				},
			},
			"release_dates": map[string]any{"results": []map[string]any{
				{"iso_3166_1": "GB", "release_dates": []map[string]any{{"certification": "12A"}}},
				{"iso_3166_1": "US", "release_dates": []map[string]any{{"certification": "R"}}},
			}},
		})
	})
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id": 87917, "name": "For All Mankind", "original_name": "For All Mankind",
				"first_air_date": "2019-11-01", "overview": "Alternative space race.",
				"poster_path": "/fam.jpg", "vote_average": 8.1, "popularity": 30,
			}},
		})
	})
	mux.HandleFunc("/tv/87917", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 87917, "name": "For All Mankind", "first_air_date": "2019-11-01",
			"episode_run_time": []int{60}, "genres": []map[string]any{{"name": "Drama"}},
			"content_ratings": map[string]any{"results": []map[string]any{{"iso_3166_1": "US", "rating": "TV-MA"}}},
			"credits":         map[string]any{"cast": []map[string]any{}, "crew": []map[string]any{}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewTMDB("test-key", "en-US", "US").WithBaseURLs(srv.URL, "https://img.test/t/p")
	ctx := context.Background()

	movies, err := client.SearchMovies(ctx, "the king's speech", 2010)
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 1 {
		t.Fatalf("movies = %+v", movies)
	}
	m := movies[0]
	if m.Title != "The King's Speech" || m.Year != 2010 || m.ProviderID != "45269" {
		t.Errorf("candidate = %+v", m)
	}
	if m.PosterURL != "https://img.test/t/p/w500/poster.jpg" {
		t.Errorf("poster = %q", m.PosterURL)
	}

	details, err := client.Movie(ctx, "45269")
	if err != nil {
		t.Fatal(err)
	}
	if details.RuntimeMS != 118*60_000 || details.ContentRating != "R" {
		t.Errorf("details = %+v", details)
	}
	if len(details.Genres) != 2 || len(details.Cast) != 2 || len(details.Crew) != 2 {
		t.Errorf("genres/cast/crew = %v %v %v", details.Genres, details.Cast, details.Crew)
	}
	if details.Tagline != "Find your voice." {
		t.Errorf("tagline = %q", details.Tagline)
	}

	tv, err := client.SearchTV(ctx, "for all mankind", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tv) != 1 || tv[0].Title != "For All Mankind" || tv[0].Year != 2019 {
		t.Fatalf("tv = %+v", tv)
	}
	td, err := client.TV(ctx, "87917")
	if err != nil {
		t.Fatal(err)
	}
	if td.RuntimeMS != 60*60_000 || td.ContentRating != "TV-MA" {
		t.Errorf("tv details = %+v", td)
	}
}

func TestTMDBNotConfigured(t *testing.T) {
	client := NewTMDB("", "", "")
	if _, err := client.SearchMovies(context.Background(), "x", 0); err != ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestScore(t *testing.T) {
	exact := Score(Candidate{Title: "The King's Speech", Year: 2010}, "The King's Speech", 2010)
	wrongYear := Score(Candidate{Title: "The King's Speech", Year: 1999}, "The King's Speech", 2010)
	other := Score(Candidate{Title: "The Queen", Year: 2006}, "The King's Speech", 2010)

	if exact < 0.9 {
		t.Errorf("exact score = %.3f, want >= 0.9", exact)
	}
	if wrongYear >= exact {
		t.Errorf("wrong year (%.3f) should score below exact (%.3f)", wrongYear, exact)
	}
	if other >= 0.5 {
		t.Errorf("unrelated score = %.3f, want < 0.5", other)
	}
}

func TestTMDBNetworkErrorRedactsAPIKey(t *testing.T) {
	client := NewTMDB("super-secret-key", "en-US", "US")
	client.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", req.URL.String())
	})
	_, err := client.SearchMovies(context.Background(), "test", 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Fatalf("error leaks the API key: %v", err)
	}
	if !strings.Contains(err.Error(), "api_key=[redacted]") {
		t.Fatalf("error did not redact the key: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
