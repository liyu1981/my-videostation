package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/store"
)

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

type itemDetail struct {
	ID            int64  `json:"id"`
	Kind          string `json:"kind"`
	ParentID      int64  `json:"parentId"`
	Title         string `json:"title"`
	ShowTitle     string `json:"showTitle"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
}

func getItemDetail(t *testing.T, srvURL string, id int64) itemDetail {
	t.Helper()
	var d itemDetail
	getJSON(t, srvURL+"/api/v1/items/"+itoa(id), &d)
	return d
}

// TestReclassifyEpisodeToMovie: a misparsed episode can be turned into a movie,
// the empty show/season are pruned, and a later scan does not turn it back.
func TestReclassifyEpisodeToMovie(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	writeFile(t, filepath.Join(tvDir, "Alpha Show", "Season 01", "Alpha Show S01E01.mkv"))
	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	eps, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindEpisode, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 {
		t.Fatalf("episodes indexed = %d", len(eps))
	}
	episode := eps[0].ID
	if got := getItemDetail(t, srv.URL, episode); got.Kind != "episode" || got.ParentID == 0 {
		t.Fatalf("before: %+v", got)
	}

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(episode)+"/reclassify", map[string]any{"kind": "movie"})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reclassify: %d", res.StatusCode)
	}
	got := getItemDetail(t, srv.URL, episode)
	if got.Kind != "movie" || got.ParentID != 0 || got.SeasonNumber != 0 || got.EpisodeNumber != 0 {
		t.Fatalf("after reclassify: %+v", got)
	}

	// The show and season no longer have children and are pruned.
	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kinds: []string{store.KindShow, store.KindSeason}, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("empty show/season left behind: %+v", items)
	}

	// A full rescan parses the filename as an episode again, but the manual type
	// wins: the item stays a movie and no duplicate is created.
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	got = getItemDetail(t, srv.URL, episode)
	if got.Kind != "movie" {
		t.Fatalf("rescan changed kind back to %q", got.Kind)
	}
	items, _, err = st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kinds: []string{store.KindEpisode}, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("rescan created episode rows: %+v", items)
	}
}

// TestReclassifyEpisodeRenumbersWithoutShowTitle: an existing episode can be
// moved to another season/episode by hand (specials/trailers) without retyping
// the show title.
func TestReclassifyEpisodeRenumbersWithoutShowTitle(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	writeFile(t, filepath.Join(tvDir, "Renumber Show", "Season 01", "Renumber Show S01E01.mkv"))
	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	eps, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindEpisode, Page: 1, PerPage: 10})
	if err != nil || len(eps) != 1 {
		t.Fatalf("episodes = %+v (%v)", eps, err)
	}
	id := eps[0].ID

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/reclassify", map[string]any{
		"kind": "episode", "seasonNumber": 2, "episodeNumber": 5,
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reclassify: %d", res.StatusCode)
	}
	got := getItemDetail(t, srv.URL, id)
	if got.Kind != "episode" || got.ShowTitle != "Renumber Show" || got.SeasonNumber != 2 || got.EpisodeNumber != 5 {
		t.Fatalf("after renumber: %+v", got)
	}
	// The old season had no other episodes and is pruned; the show remains.
	seasons, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindSeason, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(seasons) != 1 || seasons[0].SeasonNumber != 2 {
		t.Fatalf("seasons = %+v", seasons)
	}
}

// TestReclassifyHomeVideoToEpisode: a home video can be attached to a show/season
// and stays an episode across rescans.
func TestReclassifyHomeVideoToEpisode(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	homeDir := filepath.Join(dir, "home")
	writeFile(t, filepath.Join(homeDir, "random clip.mkv"))
	lib, err := st.CreateLibrary(ctx, "Home", "home_video", "", []string{homeDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	item := itemIDByTitle(t, srv.URL, lib.ID, "random clip")
	if item == 0 {
		t.Fatal("home video not indexed")
	}
	if got := getItemDetail(t, srv.URL, item); got.Kind != "home_video" {
		t.Fatalf("before: %+v", got)
	}

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(item)+"/reclassify", map[string]any{
		"kind": "episode", "showTitle": "Some Show", "seasonNumber": 2, "episodeNumber": 5,
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reclassify: %d", res.StatusCode)
	}
	got := getItemDetail(t, srv.URL, item)
	if got.Kind != "episode" || got.ShowTitle != "Some Show" || got.SeasonNumber != 2 || got.EpisodeNumber != 5 {
		t.Fatalf("after reclassify: %+v", got)
	}
	if got.ParentID == 0 {
		t.Fatal("episode has no season parent")
	}

	// The show and season now exist in the same library.
	shows, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindShow, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(shows) != 1 || shows[0].Title != "Some Show" {
		t.Fatalf("shows = %+v", shows)
	}

	// A rescan parses the filename as a home video again; the manual episode
	// assignment wins and the show keeps exactly one episode.
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	got = getItemDetail(t, srv.URL, item)
	if got.Kind != "episode" || got.ShowTitle != "Some Show" {
		t.Fatalf("rescan changed item: %+v", got)
	}
	episodes, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindEpisode, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes after rescan = %d", len(episodes))
	}
}

// TestReclassifyRejectsBadInput covers validation and unknown kinds.
func TestReclassifyRejectsBadInput(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	homeDir := filepath.Join(dir, "home")
	writeFile(t, filepath.Join(homeDir, "clip.mkv"))
	lib, err := st.CreateLibrary(ctx, "Home", "home_video", "", []string{homeDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	item := itemIDByTitle(t, srv.URL, lib.ID, "clip")

	for _, tc := range []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"kind": "episode"}, http.StatusBadRequest},
		{map[string]any{"kind": "show"}, http.StatusBadRequest},
		{map[string]any{"kind": "episode", "showTitle": "Show", "seasonNumber": -1}, http.StatusBadRequest},
	} {
		res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(item)+"/reclassify", tc.body)
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("%v = %d, want %d", tc.body, res.StatusCode, tc.want)
		}
	}
}
