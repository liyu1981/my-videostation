package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	scan := scanner.New(st, nil, log)
	manager.Handle("scan", func(ctx context.Context, job *store.Job, report jobs.Progress) error {
		var payload struct {
			LibraryID int64 `json:"libraryId"`
			Force     bool  `json:"force"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		_, err := scan.ScanLibraryForce(ctx, payload.LibraryID, payload.Force, func(p float64, msg string) {
			report(p, msg)
		})
		return err
	})
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

// ---- M3: collections, bulk actions, facets, folders, scheduler ----------

func TestCollectionsAndBulkActions(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(mediaDir, "Alpha (2001)", "Alpha.2001.mkv"))
	writeFile(t, filepath.Join(mediaDir, "Beta (2002)", "Beta.2002.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie, Sort: "title"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v err=%v", items, err)
	}
	a, b := items[0].ID, items[1].ID

	// system collections exist
	var cols struct {
		Collections []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			SystemKey string `json:"systemKey"`
		} `json:"collections"`
	}
	getJSON(t, srv.URL+"/api/v1/collections", &cols)
	if len(cols.Collections) != 2 {
		t.Fatalf("collections = %+v", cols.Collections)
	}

	// create a user collection
	body, _ := json.Marshal(map[string]string{"name": "Rewatch"})
	res, err := http.Post(srv.URL+"/api/v1/collections", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create collection: %d %s", res.StatusCode, b)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	// add items
	addBody, _ := json.Marshal(map[string]any{"itemIds": []int64{a}})
	ares, err := http.Post(srv.URL+"/api/v1/collections/"+itoa(created.ID)+"/items", "application/json", bytes.NewReader(addBody))
	if err != nil {
		t.Fatal(err)
	}
	ares.Body.Close()
	if ares.StatusCode != http.StatusOK {
		t.Fatalf("add items: %d", ares.StatusCode)
	}

	var colItems struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/collections/"+itoa(created.ID)+"/items", &colItems)
	if colItems.Total != 1 || colItems.Items[0].ID != a {
		t.Fatalf("collection items = %+v", colItems)
	}

	// system collections are immutable
	renameBody, _ := json.Marshal(map[string]string{"name": "Nope"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/collections/"+itoa(cols.Collections[0].ID), bytes.NewReader(renameBody))
	req.Header.Set("Content-Type", "application/json")
	rres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rres.Body.Close()
	if rres.StatusCode != http.StatusBadRequest {
		t.Fatalf("rename system collection = %d, want 400", rres.StatusCode)
	}

	// bulk mark watched
	batchBody, _ := json.Marshal(map[string]any{"action": "watched", "itemIds": []int64{a, b}})
	bres, err := http.Post(srv.URL+"/api/v1/items/batch", "application/json", bytes.NewReader(batchBody))
	if err != nil {
		t.Fatal(err)
	}
	bres.Body.Close()
	if bres.StatusCode != http.StatusOK {
		t.Fatalf("batch watched: %d", bres.StatusCode)
	}
	var watched struct {
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&watched=true", &watched)
	if watched.Total != 2 {
		t.Fatalf("watched total = %d, want 2", watched.Total)
	}

	// bulk unwatched
	batchBody, _ = json.Marshal(map[string]any{"action": "unwatched", "itemIds": []int64{a}})
	bres, err = http.Post(srv.URL+"/api/v1/items/batch", "application/json", bytes.NewReader(batchBody))
	if err != nil {
		t.Fatal(err)
	}
	bres.Body.Close()
	var unwatched struct {
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&watched=true", &unwatched)
	if unwatched.Total != 1 {
		t.Fatalf("watched after unwatch = %d, want 1", unwatched.Total)
	}

	// remove item + delete collection
	dreq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/collections/"+itoa(created.ID)+"/items/"+itoa(a), nil)
	dres, err := http.DefaultClient.Do(dreq)
	if err != nil {
		t.Fatal(err)
	}
	dres.Body.Close()
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("remove item = %d", dres.StatusCode)
	}
	dreq2, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/collections/"+itoa(created.ID), nil)
	dres2, err := http.DefaultClient.Do(dreq2)
	if err != nil {
		t.Fatal(err)
	}
	dres2.Body.Close()
	if dres2.StatusCode != http.StatusNoContent {
		t.Fatalf("delete collection = %d", dres2.StatusCode)
	}
}

func TestFacetsFoldersAndCategories(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(mediaDir, "Alpha (2001)", "Alpha.2001.mkv"))
	writeFile(t, filepath.Join(mediaDir, "Beta (2002)", "Beta.2002.mkv"))
	writeFile(t, filepath.Join(mediaDir, "Nested", "Gamma (2003)", "Gamma.2003.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}

	var folders struct {
		Folders []struct {
			Path  string `json:"path"`
			Count int    `json:"count"`
		} `json:"folders"`
	}
	getJSON(t, srv.URL+"/api/v1/libraries/"+itoa(lib.ID)+"/folders", &folders)
	if len(folders.Folders) < 2 {
		t.Fatalf("folders = %+v", folders.Folders)
	}

	// add a genre to one item so facets have data
	items, _, _ := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}
	patch, _ := json.Marshal(map[string]any{"genres": []string{"Drama"}})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/items/"+itoa(items[0].ID), bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()

	var facets struct {
		Genres []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		} `json:"genres"`
		Years []struct {
			Value string `json:"value"`
		} `json:"years"`
	}
	getJSON(t, srv.URL+"/api/v1/libraries/"+itoa(lib.ID)+"/facets?kind=movie", &facets)
	if len(facets.Genres) != 1 || facets.Genres[0].Value != "Drama" {
		t.Fatalf("genres = %+v", facets.Genres)
	}
	if len(facets.Years) != 3 {
		t.Fatalf("years = %+v", facets.Years)
	}

	// genre filter
	var byGenre struct {
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&genre=Drama", &byGenre)
	if byGenre.Total != 1 {
		t.Fatalf("genre filter total = %d", byGenre.Total)
	}

	// folder filter
	var byFolder struct {
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&folder="+url.QueryEscape("Nested/Gamma (2003)"), &byFolder)
	if byFolder.Total != 1 {
		t.Fatalf("folder filter total = %d", byFolder.Total)
	}

	// category ordering: recently released puts 2003 first
	var released struct {
		Items []struct {
			Year int `json:"year"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?kind=movie&category=recent_released", &released)
	if len(released.Items) == 0 || released.Items[0].Year != 2003 {
		t.Fatalf("recent_released = %+v", released.Items)
	}

	// auto-scan scheduler bookkeeping
	mins := 60
	if err := st.UpdateLibrary(ctx, lib.ID, "", "", nil, &mins, nil); err != nil {
		t.Fatal(err)
	}
	due, err := st.LibrariesDueForScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0] != lib.ID {
		t.Fatalf("due = %v", due)
	}
	if err := st.MarkAutoScan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	due, err = st.LibrariesDueForScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("due after mark = %v", due)
	}
}

// ---- M4: playback, subtitles, next-up -----------------------------------

func TestPlaybackSubtitlesAndNextUp(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv", "Show (2020)", "Season 1")
	writeFile(t, filepath.Join(tvDir, "Show.S01E01.mkv"))
	writeFile(t, filepath.Join(tvDir, "Show.S01E02.mkv"))
	if err := os.WriteFile(filepath.Join(tvDir, "Show.S01E01.en.srt"),
		[]byte("1\n00:00:01,000 --> 00:00:03,000\nHello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tvDir, "Show.S01E01.zh.srt"),
		[]byte("1\n00:00:01,000 --> 00:00:03,000\n你好\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{filepath.Join(dir, "tv")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
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
			ID      int64 `json:"id"`
			Episode int   `json:"episodeNumber"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(seasons.Items[0].ID)+"/children", &episodes)
	if len(episodes.Items) != 2 {
		t.Fatalf("episodes = %+v", episodes.Items)
	}
	ep1, ep2 := episodes.Items[0].ID, episodes.Items[1].ID
	if episodes.Items[0].Episode != 1 {
		ep1, ep2 = ep2, ep1
	}

	var pb struct {
		ItemID    int64  `json:"itemId"`
		FileID    int64  `json:"fileId"`
		StreamURL string `json:"streamUrl"`
		ResumeMS  int64  `json:"resumeMs"`
		Subtitles []struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			Language string `json:"language"`
			URL      string `json:"url"`
			Source   string `json:"source"`
			Default  bool   `json:"default"`
		} `json:"subtitles"`
		Next *struct {
			ItemID int64 `json:"itemId"`
		} `json:"next"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(ep1)+"/playback", &pb)
	if pb.StreamURL == "" || pb.FileID == 0 {
		t.Fatalf("playback = %+v", pb)
	}
	if len(pb.Subtitles) != 2 {
		t.Fatalf("subtitles = %+v", pb.Subtitles)
	}
	if pb.Next == nil || pb.Next.ItemID != ep2 {
		t.Fatalf("next = %+v, want %d", pb.Next, ep2)
	}

	// external subtitle is served as WebVTT
	var subURL string
	for _, s := range pb.Subtitles {
		if s.Language == "en" {
			subURL = s.URL
		}
	}
	if subURL == "" {
		t.Fatal("no english subtitle track")
	}
	res, err := http.Get(srv.URL + subURL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("subtitle status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/vtt") {
		t.Errorf("content type = %q", ct)
	}
	vtt, _ := io.ReadAll(res.Body)
	if !strings.HasPrefix(string(vtt), "WEBVTT") || !strings.Contains(string(vtt), "00:00:01.000 --> 00:00:03.000") {
		t.Fatalf("vtt = %q", string(vtt))
	}

	// resume position round-trips, and is cleared once watched
	prog, _ := json.Marshal(map[string]any{"positionMs": 30000, "durationMs": 60000, "played": false})
	pres, err := http.Post(srv.URL+"/api/v1/items/"+itoa(ep1)+"/progress", "application/json", bytes.NewReader(prog))
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(ep1)+"/playback", &pb)
	if pb.ResumeMS != 30000 {
		t.Fatalf("resumeMs = %d, want 30000", pb.ResumeMS)
	}

	done, _ := json.Marshal(map[string]any{"positionMs": 60000, "durationMs": 60000, "played": true})
	dres, err := http.Post(srv.URL+"/api/v1/items/"+itoa(ep1)+"/progress", "application/json", bytes.NewReader(done))
	if err != nil {
		t.Fatal(err)
	}
	dres.Body.Close()
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(ep1)+"/playback", &pb)
	if pb.ResumeMS != 0 {
		t.Fatalf("resumeMs after watched = %d, want 0", pb.ResumeMS)
	}

	// last episode has no next
	pb.Next = nil
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(ep2)+"/playback", &pb)
	if pb.Next != nil {
		t.Fatalf("last episode next = %+v", pb.Next)
	}
}

func TestManualRenameSurvivesRescan(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "m")
	writeFile(t, filepath.Join(mediaDir, "Original (2020)", "Original.2020.mkv"))
	lib, err := st.CreateLibrary(ctx, "M", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sc := scanner.New(st, nil, log)
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, _ := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	patch, _ := json.Marshal(map[string]any{"title": "My Renamed Movie"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/items/"+itoa(items[0].ID), bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d", res.StatusCode)
	}
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	after, _, _ := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if len(after) != 1 || after[0].Title != "My Renamed Movie" {
		t.Fatalf("rescan lost identity/title: %+v", after)
	}
}

func TestPlaybackMissingFile(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "m")
	writeFile(t, filepath.Join(mediaDir, "Movie (2020)", "Movie.2020.mkv"))
	lib, err := st.CreateLibrary(ctx, "M", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, _ := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	res, err := http.Get(srv.URL + "/api/v1/items/" + itoa(items[0].ID) + "/playback")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("playback = %d %s", res.StatusCode, b)
	}
}

// ---- local sidecar metadata (.nfo / .vsmeta) ----------------------------

func vsmetaBytes(title string) []byte {
	vint := func(v uint64) []byte {
		var out []byte
		for v >= 0x80 {
			out = append(out, byte(v)|0x80)
			v >>= 7
		}
		return append(out, byte(v))
	}
	str := func(field int, s string) []byte {
		b := []byte(s)
		return append(append(vint(uint64(field<<3|2)), vint(uint64(len(b)))...), b...)
	}
	num := func(field int, v uint64) []byte { return append(vint(uint64(field<<3)), vint(v)...) }
	cat := func(parts ...[]byte) []byte {
		var out []byte
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	img := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1}, make([]byte, 64)...)
	credits := cat(str(1, "VS Actor"), str(3, "Action"))
	return cat(
		num(1, 1),
		str(2, title),
		num(5, 2021),
		str(8, "VS overview."),
		str(10, string(credits)),
		num(12, 82),
		str(17, base64.StdEncoding.EncodeToString(img)),
		str(18, "vs-poster-hash"),
	)
}

func TestLocalSidecarMetadata(t *testing.T) {
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
	cacheDir := filepath.Join(dir, "cache")
	factory := func(context.Context) (metadata.Provider, error) { return fakeProvider{}, nil }
	enricher := enrich.New(st, factory, cacheDir, log)
	manager := jobs.New(st, log)
	manager.Start(ctx)
	srv := httptest.NewServer(httpapi.New(httpapi.Options{
		Store: st, Jobs: manager, Logger: log, Provider: factory, Enricher: enricher, CacheDir: cacheDir,
	}).Handler())
	t.Cleanup(srv.Close)

	mediaDir := filepath.Join(dir, "media")
	nfoPath := filepath.Join(mediaDir, "NFO Movie (2010)", "NFO Movie (2010).mkv")
	writeFile(t, nfoPath)
	if err := os.WriteFile(filepath.Join(mediaDir, "NFO Movie (2010)", "NFO Movie (2010).nfo"), []byte(
		`<movie><title>NFO Title</title><year>2010</year><plot>NFO plot.</plot>
		 <genre>Drama</genre><actor><name>NFO Actor</name></actor>
		 <uniqueid type="tmdb">123</uniqueid></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	vsPath := filepath.Join(mediaDir, "VS Movie (2021)", "VS.Movie.2021.mkv")
	writeFile(t, vsPath)
	if err := os.WriteFile(vsPath+".vsmeta", vsmetaBytes("VS Title"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, log)
	sc.SetCacheDir(cacheDir)
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}

	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie, Sort: "title"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %v err=%v", items, err)
	}

	// NFO item
	var nfoDetail struct {
		Title          string   `json:"title"`
		Year           int      `json:"year"`
		Overview       string   `json:"overview"`
		Genres         []string `json:"genres"`
		MetadataStatus string   `json:"metadataStatus"`
		Provider       string   `json:"provider"`
		People         []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"people"`
	}
	var nfoID int64
	for _, it := range items {
		if it.Title == "NFO Title" {
			nfoID = it.ID
		}
	}
	if nfoID == 0 {
		t.Fatalf("NFO item not found: %+v", items)
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(nfoID), &nfoDetail)
	if nfoDetail.MetadataStatus != "local" || nfoDetail.Provider != "nfo" {
		t.Fatalf("nfo status/provider = %q/%q", nfoDetail.MetadataStatus, nfoDetail.Provider)
	}
	if nfoDetail.Year != 2010 || nfoDetail.Overview != "NFO plot." || len(nfoDetail.Genres) != 1 {
		t.Fatalf("nfo fields = %+v", nfoDetail)
	}
	if len(nfoDetail.People) != 1 || nfoDetail.People[0].Name != "NFO Actor" {
		t.Fatalf("nfo people = %+v", nfoDetail.People)
	}

	// vsmeta item + cached poster served
	var vsDetail struct {
		Title          string `json:"title"`
		Year           int    `json:"year"`
		MetadataStatus string `json:"metadataStatus"`
		Provider       string `json:"provider"`
		PosterURL      string `json:"posterUrl"`
	}
	var vsID int64
	for _, it := range items {
		if it.Title == "VS Title" {
			vsID = it.ID
		}
	}
	if vsID == 0 {
		t.Fatalf("vsmeta item not found: %+v", items)
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(vsID), &vsDetail)
	if vsDetail.MetadataStatus != "local" || vsDetail.Provider != "vsmeta" || vsDetail.Year != 2021 {
		t.Fatalf("vsmeta fields = %+v", vsDetail)
	}
	if vsDetail.PosterURL == "" {
		t.Fatal("vsmeta poster not cached")
	}
	pres, err := http.Get(srv.URL + vsDetail.PosterURL)
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()
	if pres.StatusCode != http.StatusOK {
		t.Fatalf("poster status = %d", pres.StatusCode)
	}

	// Re-scanning must not duplicate items when sidecar metadata (or a user edit)
	// renamed them: identity follows the file path.
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	again, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if err != nil || len(again) != 2 {
		t.Fatalf("after rescan items = %d (%v), want 2", len(again), err)
	}

	// Automatic refresh must not overwrite local sidecar metadata.
	rres, err := http.Post(srv.URL+"/api/v1/items/"+itoa(nfoID)+"/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rres.Body.Close()
	if rres.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(rres.Body)
		t.Fatalf("refresh: %d %s", rres.StatusCode, b)
	}
	var after struct {
		Title          string `json:"title"`
		MetadataStatus string `json:"metadataStatus"`
	}
	if err := json.NewDecoder(rres.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Title != "NFO Title" || after.MetadataStatus != "local" {
		t.Fatalf("local metadata was overwritten: %+v", after)
	}
}

// Regression: duplicate sidecar titles (same movie in two folders/qualities) must
// not abort the scan. Identity is the file path, not the title.
func TestDuplicateLocalTitlesDoNotAbortScan(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "media")
	for _, name := range []string{"A", "B"} {
		video := filepath.Join(mediaDir, name, name+".mkv")
		writeFile(t, video)
		nfo := `<movie><title>Same Movie</title><year>2020</year><plot>Plot ` + name + `.</plot></movie>`
		if err := os.WriteFile(filepath.Join(mediaDir, name, name+".nfo"), []byte(nfo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := sc.ScanLibrary(ctx, lib.ID, nil)
	if err != nil {
		t.Fatalf("scan aborted: %v", err)
	}
	if stats.Skipped != 0 {
		t.Fatalf("skipped = %d, want 0: %+v", stats.Skipped, stats)
	}
	items, total, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindMovie})
	if err != nil || total != 2 {
		t.Fatalf("items = %d (%v), want 2", total, err)
	}
	for _, it := range items {
		if it.Title != "Same Movie" || it.MetadataStatus != "local" {
			t.Errorf("item = %q / %q", it.Title, it.MetadataStatus)
		}
	}
	_ = srv
}

// ---- job logs, deletion, force rescan --------------------------------

func waitJob(t *testing.T, srvURL string, id int64) store.Job {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var job store.Job
		getJSON(t, srvURL+"/api/v1/jobs/"+itoa(id), &job)
		if job.Status == "succeeded" || job.Status == "failed" {
			return job
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job %d did not finish", id)
	return store.Job{}
}

func TestJobLogsDeletionAndForceRescan(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Movie (2020)", "Movie.2020.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}

	// incremental scan via the API
	res, err := http.Post(srv.URL+"/api/v1/libraries/"+itoa(lib.ID)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var job store.Job
	if err := json.NewDecoder(res.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	finished := waitJob(t, srv.URL, job.ID)
	if finished.Status != "succeeded" {
		t.Fatalf("scan status = %s: %s", finished.Status, finished.Error)
	}

	// logs are persisted
	var logs struct {
		Logs []struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		} `json:"logs"`
	}
	getJSON(t, srv.URL+"/api/v1/jobs/"+itoa(job.ID)+"/logs", &logs)
	if len(logs.Logs) < 2 {
		t.Fatalf("logs = %+v", logs.Logs)
	}
	if logs.Logs[0].Message != "job started (scan)" {
		t.Errorf("first log = %+v", logs.Logs[0])
	}

	// force rescan re-probes unchanged files
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	forceStats, err := sc.ScanLibraryForce(ctx, lib.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if forceStats.Probed != 0 { // no ffprobe in tests, but Force should still "attempt" probing
		t.Logf("probed = %d (ffprobe unavailable in test env)", forceStats.Probed)
	}
	if forceStats.Unchanged != 0 {
		t.Errorf("force rescan reported %d unchanged, want 0", forceStats.Unchanged)
	}

	// delete one job and its logs
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/jobs/"+itoa(job.ID), nil)
	dres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dres.Body.Close()
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("delete job = %d", dres.StatusCode)
	}
	gres, err := http.Get(srv.URL + "/api/v1/jobs/" + itoa(job.ID) + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	gres.Body.Close()
	if gres.StatusCode != http.StatusNotFound {
		t.Fatalf("logs after delete = %d, want 404", gres.StatusCode)
	}

	// a second (force) scan through the API keeps a finished job for the clear test
	fres, err := http.Post(srv.URL+"/api/v1/libraries/"+itoa(lib.ID)+"/scan", "application/json", bytes.NewReader([]byte(`{"force":true}`)))
	if err != nil {
		t.Fatal(err)
	}
	var forceJob store.Job
	if err := json.NewDecoder(fres.Body).Decode(&forceJob); err != nil {
		fres.Body.Close()
		t.Fatal(err)
	}
	fres.Body.Close()
	if forceJob.Status == "" {
		t.Fatalf("force scan job = %+v", forceJob)
	}
	if fdone := waitJob(t, srv.URL, forceJob.ID); fdone.Status != "succeeded" {
		t.Fatalf("force scan status = %s: %s", fdone.Status, fdone.Error)
	}

	// clear finished jobs
	var cleared struct {
		Deleted int64 `json:"deleted"`
	}
	creq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/jobs", nil)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(cres.Body).Decode(&cleared); err != nil {
		cres.Body.Close()
		t.Fatal(err)
	}
	cres.Body.Close()
	if cleared.Deleted < 1 {
		t.Fatalf("cleared = %d", cleared.Deleted)
	}
	var remaining struct {
		Jobs []store.Job `json:"jobs"`
	}
	getJSON(t, srv.URL+"/api/v1/jobs", &remaining)
	if len(remaining.Jobs) != 0 {
		t.Fatalf("jobs after clear = %d", len(remaining.Jobs))
	}
}
