package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if err := st.UpdateLibrary(ctx, lib.ID, "", "", nil, &mins); err != nil {
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
