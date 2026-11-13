package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/liyu1981/myvideostation/internal/store"
)

// waitJobsIdle polls until no job is queued or running.
func waitJobsIdle(t *testing.T, srvURL string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Jobs []struct {
				Status string `json:"status"`
			} `json:"jobs"`
		}
		getJSON(t, srvURL+"/api/v1/jobs", &list)
		running := false
		for _, j := range list.Jobs {
			if j.Status == "running" || j.Status == "queued" || j.Status == "pending" {
				running = true
			}
		}
		if !running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("jobs did not become idle")
}

// itemChildren returns the ids of an item's direct children of one kind.
func itemChildren(t *testing.T, st *store.Store, parentID int64, kind string) []int64 {
	t.Helper()
	items, _, err := st.ListItems(context.Background(), store.ItemFilter{
		ParentID: parentID, Kind: kind, Page: 1, PerPage: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int64, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// TestItemRescanScopesToFolder proves a show rescan only touches that show's
// folder: new files inside it are indexed, while files outside it are neither
// indexed nor marked missing, even when they changed on disk.
func TestItemRescanScopesToFolder(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	alphaE1 := filepath.Join(tvDir, "Alpha Show", "Season 01", "Alpha Show S01E01.mkv")
	betaE1 := filepath.Join(tvDir, "Beta Show", "Season 01", "Beta Show S01E01.mkv")
	writeFile(t, alphaE1)
	writeFile(t, betaE1)

	tvLib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, tvLib.ID); err != nil {
		t.Fatal(err)
	}

	alpha := itemIDByTitle(t, srv.URL, tvLib.ID, "Alpha Show")
	beta := itemIDByTitle(t, srv.URL, tvLib.ID, "Beta Show")
	if alpha == 0 || beta == 0 {
		t.Fatalf("shows not indexed: alpha=%d beta=%d", alpha, beta)
	}
	alphaSeasons := itemChildren(t, st, alpha, store.KindSeason)
	betaSeasons := itemChildren(t, st, beta, store.KindSeason)
	if len(alphaSeasons) != 1 || len(betaSeasons) != 1 {
		t.Fatalf("seasons: alpha=%v beta=%v", alphaSeasons, betaSeasons)
	}
	if got := itemChildren(t, st, alphaSeasons[0], store.KindEpisode); len(got) != 1 {
		t.Fatalf("alpha episodes before rescan = %d", len(got))
	}
	betaEpisodes := itemChildren(t, st, betaSeasons[0], store.KindEpisode)
	if len(betaEpisodes) != 1 {
		t.Fatalf("beta episodes before rescan = %d", len(betaEpisodes))
	}

	// Change both folders: add an episode to Alpha, add one and delete one in Beta.
	alphaE2 := filepath.Join(tvDir, "Alpha Show", "Season 01", "Alpha Show S01E02.mkv")
	betaE2 := filepath.Join(tvDir, "Beta Show", "Season 01", "Beta Show S01E02.mkv")
	writeFile(t, alphaE2)
	writeFile(t, betaE2)
	if err := os.Remove(betaE1); err != nil {
		t.Fatal(err)
	}

	// Rescan Alpha only.
	body, _ := json.Marshal(map[string]bool{"force": false})
	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(alpha)+"/rescan", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("rescan: %d", res.StatusCode)
	}
	waitJobsIdle(t, srv.URL)

	// Alpha picked up its new episode.
	alphaSeasons = itemChildren(t, st, alpha, store.KindSeason)
	if got := itemChildren(t, st, alphaSeasons[0], store.KindEpisode); len(got) != 2 {
		t.Fatalf("alpha episodes after rescan = %d, want 2", len(got))
	}

	// Beta was not touched: its new file was not indexed and its deleted file was
	// not flagged missing (that is a full library scan's job).
	betaSeasons = itemChildren(t, st, beta, store.KindSeason)
	if got := itemChildren(t, st, betaSeasons[0], store.KindEpisode); len(got) != 1 {
		t.Fatalf("beta episodes after alpha rescan = %d, want 1 (scope leak)", len(got))
	}
	files, err := st.ItemFiles(ctx, betaEpisodes[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("beta episode lost its files")
	}
	if files[0].Missing {
		t.Fatal("scoped rescan marked a file outside the scope missing")
	}
}

// TestShowRescanPicksUpNewSeason: rescanning a show re-walks the show folder, so a
// brand-new season directory is indexed (a season-scoped walk would miss it).
func TestShowRescanPicksUpNewSeason(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	writeFile(t, filepath.Join(tvDir, "Delta Show", "Season 01", "Delta Show S01E01.mkv"))
	writeFile(t, filepath.Join(tvDir, "Other Show", "Season 01", "Other Show S01E01.mkv"))

	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	delta := itemIDByTitle(t, srv.URL, lib.ID, "Delta Show")
	if got := len(itemChildren(t, st, delta, store.KindSeason)); got != 1 {
		t.Fatalf("seasons before = %d", got)
	}

	writeFile(t, filepath.Join(tvDir, "Delta Show", "Season 02", "Delta Show S02E01.mkv"))
	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(delta)+"/rescan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("rescan: %d", res.StatusCode)
	}
	waitJobsIdle(t, srv.URL)

	seasons := itemChildren(t, st, delta, store.KindSeason)
	if len(seasons) != 2 {
		t.Fatalf("seasons after show rescan = %d, want 2", len(seasons))
	}
	total := 0
	for _, s := range seasons {
		total += len(itemChildren(t, st, s, store.KindEpisode))
	}
	if total != 2 {
		t.Fatalf("episodes after show rescan = %d, want 2", total)
	}
	other := itemIDByTitle(t, srv.URL, lib.ID, "Other Show")
	if got := len(itemChildren(t, st, other, store.KindSeason)); got != 1 {
		t.Fatalf("other show seasons changed: %d", got)
	}
}

// TestItemRescanReportsMissingFileInsideScope: a file deleted under the scope is
// flagged by the scoped rescan.
func TestItemRescanReportsMissingFileInsideScope(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	e1 := filepath.Join(tvDir, "Gamma Show", "Season 01", "Gamma Show S01E01.mkv")
	writeFile(t, e1)

	tvLib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, tvLib.ID); err != nil {
		t.Fatal(err)
	}
	gamma := itemIDByTitle(t, srv.URL, tvLib.ID, "Gamma Show")
	seasons := itemChildren(t, st, gamma, store.KindSeason)
	episodes := itemChildren(t, st, seasons[0], store.KindEpisode)
	if len(episodes) != 1 {
		t.Fatalf("episodes = %d", len(episodes))
	}

	if err := os.Remove(e1); err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(gamma)+"/rescan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("rescan: %d", res.StatusCode)
	}
	waitJobsIdle(t, srv.URL)

	files, err := st.ItemFiles(ctx, episodes[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 || !files[0].Missing {
		t.Fatalf("deleted file under scope not marked missing: %+v", files)
	}
}

// TestShowRescanUsesSeriesAsBase: a show-scoped rescan reattaches episodes whose
// filenames name a different show, using the show the rescan was started from.
func TestShowRescanUsesSeriesAsBase(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	writeFile(t, filepath.Join(tvDir, "Breaking Bad", "Season 01", "Breaking Bad S01E01.mkv"))
	writeFile(t, filepath.Join(tvDir, "Breaking Bad", "Season 01", "Wrong Show S01E02.mkv"))
	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	breaking := itemIDByTitle(t, srv.URL, lib.ID, "Breaking Bad")
	wrong := itemIDByTitle(t, srv.URL, lib.ID, "Wrong Show")
	if breaking == 0 || wrong == 0 {
		t.Fatalf("initial shows: breaking=%d wrong=%d", breaking, wrong)
	}

	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(breaking)+"/rescan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("rescan: %d", res.StatusCode)
	}
	waitJobsIdle(t, srv.URL)

	// Both episodes now belong to Breaking Bad; the wrongly named show is gone.
	seasons := itemChildren(t, st, breaking, store.KindSeason)
	episodes := 0
	for _, s := range seasons {
		episodes += len(itemChildren(t, st, s, store.KindEpisode))
	}
	if episodes != 2 {
		t.Fatalf("Breaking Bad episodes after rescan = %d, want 2", episodes)
	}
	shows, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindShow, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shows {
		if s.Title == "Wrong Show" {
			t.Fatalf("wrong show still present: %+v", shows)
		}
	}
}

// TestShowRescanInfersBareEpisodeNumbers: files named "02.mkv" or
// "18-4K.高码率.mkv" inside an identified show folder are promoted from
// home-video placeholders to episodes of that show.
func TestShowRescanInfersBareEpisodeNumbers(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv")
	showDir := filepath.Join(tvDir, "低智商犯罪")
	writeFile(t, filepath.Join(showDir, "低智商犯罪 S01E01.mkv"))
	writeFile(t, filepath.Join(showDir, "02.mkv"))
	writeFile(t, filepath.Join(showDir, "18-4K.高码率.mkv"))
	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	show := itemIDByTitle(t, srv.URL, lib.ID, "低智商犯罪")
	if show == 0 {
		t.Fatal("show not indexed")
	}
	placeholders, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindHomeVideo, Page: 1, PerPage: 20})
	if err != nil || len(placeholders) != 2 {
		t.Fatalf("placeholders before = %+v (%v)", placeholders, err)
	}

	res, err := http.Post(srv.URL+"/api/v1/items/"+itoa(show)+"/rescan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("rescan: %d", res.StatusCode)
	}
	waitJobsIdle(t, srv.URL)

	seasons := itemChildren(t, st, show, store.KindSeason)
	episodes := 0
	for _, s := range seasons {
		episodes += len(itemChildren(t, st, s, store.KindEpisode))
	}
	if episodes != 3 {
		t.Fatalf("episodes after rescan = %d, want 3", episodes)
	}
	left, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Kind: store.KindHomeVideo, Page: 1, PerPage: 20})
	if err != nil || len(left) != 0 {
		t.Fatalf("placeholders after = %+v (%v)", left, err)
	}
}
