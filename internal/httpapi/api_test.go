package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/enrich"
	"github.com/liyu1981/myvideostation/internal/httpapi"
	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/metadata"
	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
)

// newTestServer builds an isolated stack (temp DB, temp media dir).
func newTestServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := store.New(d)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := jobs.New(st, log)
	manager.Start(ctx)

	api := httpapi.New(httpapi.Options{Store: st, Jobs: manager, Logger: log})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv, st, dir
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a real video"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanPipelineAndAPI(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Movies", "The King's Speech (2010)", "The King's Speech (2010) (1080p BluRay x265).mkv"))
	writeFile(t, filepath.Join(mediaDir, "Movies", "Dune Part Two (2024)", "Dune.Part.Two.2024.1080p.WEB-DL.mkv"))
	writeFile(t, filepath.Join(mediaDir, "Movies", "The King's Speech (2010)", "Extras", "trailer.mkv"))

	// library via API (validates path handling + response shape)
	body, _ := json.Marshal(map[string]any{
		"name": "Movies", "type": "movie", "paths": []string{filepath.Join(mediaDir, "Movies")},
	})
	res, err := http.Post(srv.URL+"/api/v1/libraries", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create library: status %d body %s", res.StatusCode, b)
	}
	var lib store.Library
	if err := json.NewDecoder(res.Body).Decode(&lib); err != nil {
		t.Fatal(err)
	}

	// scan synchronously (no ffprobe in the test env)
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := sc.ScanLibrary(ctx, lib.ID, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Created != 3 {
		t.Fatalf("created = %d, want 3 (2 movies + 1 extra): %+v", stats.Created, stats)
	}
	if stats.Extras != 1 {
		t.Errorf("extras = %d, want 1", stats.Extras)
	}

	// browse
	var browse struct {
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
			Year  int    `json:"year"`
			Kind  string `json:"kind"`
		} `json:"items"`
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&libraryId="+itoa(lib.ID), &browse)
	if browse.Total != 2 {
		t.Fatalf("total = %d, want 2", browse.Total)
	}
	titles := map[string]int{}
	for _, it := range browse.Items {
		titles[it.Title] = it.Year
		if it.Kind != "movie" {
			t.Errorf("kind = %q, want movie", it.Kind)
		}
	}
	if titles["The King's Speech"] != 2010 {
		t.Errorf("missing/incorrect parse for The King's Speech: %v", titles)
	}
	if titles["Dune Part Two"] != 2024 {
		t.Errorf("missing/incorrect parse for Dune Part Two: %v", titles)
	}

	// search (FTS)
	var search struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/search?q=king", &search)
	if len(search.Items) != 1 || search.Items[0].Title != "The King's Speech" {
		t.Fatalf("search results = %+v", search.Items)
	}

	// item detail + metadata patch + per-field locks
	var detail map[string]any
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(browse.Items[0].ID), &detail)
	if _, ok := detail["files"]; !ok {
		t.Fatalf("detail missing files: %v", detail)
	}

	patch, _ := json.Marshal(map[string]any{
		"overview": "A king finds his voice.",
		"genres":   []string{"Drama", "History"},
		"locks":    []string{"title", "overview"},
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/items/"+itoa(browse.Items[0].ID), bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("patch: %d %s", res2.StatusCode, b)
	}
	var patched struct {
		Overview string   `json:"overview"`
		Genres   []string `json:"genres"`
		Locks    []string `json:"locks"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.Overview != "A king finds his voice." || len(patched.Genres) != 2 || len(patched.Locks) != 2 {
		t.Fatalf("patch not applied: %+v", patched)
	}

	// extras are indexed hidden and attached to the movie in the same folder
	var withExtras struct {
		Extras []struct {
			Title  string `json:"title"`
			Hidden bool   `json:"hidden"`
		} `json:"extras"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(browse.Items[0].ID), &withExtras)
	if len(withExtras.Extras) != 1 || withExtras.Extras[0].Title != "Trailer" {
		t.Fatalf("extras = %+v, want one Trailer", withExtras.Extras)
	}

	// progress + home continue-watching
	prog, _ := json.Marshal(map[string]any{"positionMs": 5000, "durationMs": 10000, "played": false})
	res3, err := http.Post(srv.URL+"/api/v1/items/"+itoa(browse.Items[0].ID)+"/progress", "application/json", bytes.NewReader(prog))
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()
	var home struct {
		Continue []struct {
			PositionMS int64 `json:"positionMs"`
		} `json:"continue"`
	}
	getJSON(t, srv.URL+"/api/v1/home", &home)
	if len(home.Continue) != 1 || home.Continue[0].PositionMS != 5000 {
		t.Fatalf("continue watching = %+v", home.Continue)
	}

	// re-scan must be incremental (no re-probe, nothing created)
	stats2, err := sc.ScanLibrary(ctx, lib.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Created != 0 {
		t.Errorf("second scan created %d items, want 0", stats2.Created)
	}
	if stats2.Probed != 0 {
		t.Errorf("second scan probed %d files, want 0", stats2.Probed)
	}
}

func TestMultiEpisodeFiles(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	writeFile(t, filepath.Join(tvDir, "Show (2020)", "Season 1", "Show.S01E01-E02.1080p.mkv"))
	writeFile(t, filepath.Join(tvDir, "Show (2020)", "Season 1", "Show.S01E03.1080p.mkv"))

	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := sc.ScanLibrary(ctx, lib.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 2 {
		t.Fatalf("created = %d, want 2 (show+season are not counted here): %+v", stats.Created, stats)
	}

	var seasons struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=season&parentId=1", &seasons)
	if len(seasons.Items) != 1 {
		t.Fatalf("seasons = %+v", seasons.Items)
	}
	var episodes struct {
		Items []struct {
			Title      string `json:"title"`
			Episode    int    `json:"episodeNumber"`
			EpisodeEnd int    `json:"episodeEnd"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(seasons.Items[0].ID)+"/children", &episodes)
	if len(episodes.Items) != 2 {
		t.Fatalf("episodes = %+v", episodes.Items)
	}
	var found bool
	for _, e := range episodes.Items {
		if e.Episode == 1 {
			found = true
			if e.EpisodeEnd != 2 {
				t.Errorf("episodeEnd = %d, want 2", e.EpisodeEnd)
			}
			if e.Title != "Episode 1" {
				t.Errorf("title = %q, want Episode 1", e.Title)
			}
		}
	}
	if !found {
		t.Fatalf("episode 1 missing: %+v", episodes.Items)
	}
}

func TestValidationErrors(t *testing.T) {
	srv, _, dir := newTestServer(t)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing name", map[string]any{"type": "movie", "paths": []string{dir}}, http.StatusBadRequest},
		{"bad type", map[string]any{"name": "x", "type": "nope", "paths": []string{dir}}, http.StatusBadRequest},
		{"no paths", map[string]any{"name": "x", "type": "movie", "paths": []string{}}, http.StatusBadRequest},
		{"relative path", map[string]any{"name": "x", "type": "movie", "paths": []string{"media"}}, http.StatusBadRequest},
		{"missing path", map[string]any{"name": "x", "type": "movie", "paths": []string{"/definitely/not/here"}}, http.StatusBadRequest},
		{"file path", map[string]any{"name": "x", "type": "movie", "paths": []string{"/etc/hostname"}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			res, err := http.Post(srv.URL+"/api/v1/libraries", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	srv, _, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", res.StatusCode)
	}
}

func getJSON(t *testing.T, url string, dst any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("GET %s: %d %s", url, res.StatusCode, b)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		t.Fatal(err)
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// ---- metadata (M2) -------------------------------------------------------

type fakeProvider struct{}

func (fakeProvider) Name() string { return "fake" }

func (fakeProvider) SearchMovies(_ context.Context, query string, year int) ([]metadata.Candidate, error) {
	return []metadata.Candidate{{
		ProviderID: "603", Title: "The King's Speech", OriginalTitle: "The King's Speech",
		Year: 2010, Overview: "A king finds his voice.", Rating: 7.7, MediaType: "movie",
	}}, nil
}

func (fakeProvider) SearchTV(_ context.Context, _ string, _ int) ([]metadata.Candidate, error) {
	return nil, nil
}

func (fakeProvider) Movie(_ context.Context, id string) (*metadata.Details, error) {
	return &metadata.Details{
		Candidate:     metadata.Candidate{ProviderID: id, Title: "The King's Speech", Year: 2010, Rating: 7.7},
		Tagline:       "Find your voice.",
		ContentRating: "R",
		RuntimeMS:     118 * 60 * 1000,
		Genres:        []string{"Drama", "History"},
		Cast:          []metadata.Credit{{Name: "Colin Firth", Character: "King George VI"}},
		Crew:          []metadata.Credit{{Name: "Tom Hooper", Job: "Director"}},
	}, nil
}

func (fakeProvider) TV(_ context.Context, id string) (*metadata.Details, error) {
	return &metadata.Details{Candidate: metadata.Candidate{ProviderID: id, Title: "For All Mankind", Year: 2019}}, nil
}

func TestMetadataMatchingFlow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	factory := func(context.Context) (metadata.Provider, error) { return fakeProvider{}, nil }
	enricher := enrich.New(st, factory, filepath.Join(dir, "cache"), log)
	manager := jobs.New(st, log)
	manager.Start(ctx)
	srv := httptest.NewServer(httpapi.New(httpapi.Options{
		Store: st, Jobs: manager, Logger: log, Provider: factory, Enricher: enricher,
	}).Handler())
	t.Cleanup(srv.Close)

	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "The King's Speech (2010)", "The King's Speech (2010) 1080p.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, log)
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %v err=%v", items, err)
	}
	id := items[0].ID
	if items[0].MetadataStatus != "unmatched" {
		t.Fatalf("status = %q, want unmatched", items[0].MetadataStatus)
	}

	// candidate search
	var matches struct {
		Candidates []struct {
			ProviderID string `json:"providerId"`
			Title      string `json:"title"`
		} `json:"candidates"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/matches?q=king", &matches)
	if len(matches.Candidates) != 1 || matches.Candidates[0].ProviderID != "603" {
		t.Fatalf("candidates = %+v", matches.Candidates)
	}

	// manual match
	body, _ := json.Marshal(map[string]string{"providerId": "603"})
	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(id)+"/match", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("match: %d %s", res.StatusCode, b)
	}
	var matched struct {
		Title          string   `json:"title"`
		Tagline        string   `json:"tagline"`
		ContentRating  string   `json:"contentRating"`
		Genres         []string `json:"genres"`
		ProviderID     string   `json:"providerId"`
		MetadataStatus string   `json:"metadataStatus"`
		People         []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"people"`
	}
	if err := json.NewDecoder(res.Body).Decode(&matched); err != nil {
		t.Fatal(err)
	}
	if matched.Title != "The King's Speech" || matched.ProviderID != "603" || matched.MetadataStatus != "manual" {
		t.Fatalf("matched = %+v", matched)
	}
	if matched.Tagline != "Find your voice." || matched.ContentRating != "R" || len(matched.Genres) != 2 {
		t.Fatalf("details not applied: %+v", matched)
	}
	if len(matched.People) != 2 { // one cast + one director
		t.Fatalf("people = %+v", matched.People)
	}

	// refresh must respect locked fields
	patch, _ := json.Marshal(map[string]any{"overview": "Manual overview", "locks": []string{"overview"}})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/items/"+itoa(id), bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()

	rres, err := http.Post(srv.URL+"/api/v1/items/"+itoa(id)+"/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rres.Body.Close()
	if rres.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(rres.Body)
		t.Fatalf("refresh: %d %s", rres.StatusCode, b)
	}
	var refreshed struct {
		Overview string `json:"overview"`
	}
	if err := json.NewDecoder(rres.Body).Decode(&refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Overview != "Manual overview" {
		t.Fatalf("locked overview overwritten: %q", refreshed.Overview)
	}
}

func TestMetadataNotConfigured(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "m")
	writeFile(t, filepath.Join(mediaDir, "Some Movie (2020)", "Some.Movie.2020.mkv"))
	lib, err := st.CreateLibrary(ctx, "M", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, _ := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	res, err := http.Get(srv.URL + "/api/v1/items/" + itoa(items[0].ID) + "/matches")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
}
