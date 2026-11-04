package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/liyu1981/myvideostation/internal/store"
)

func newScanRulesTree(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	files := []string{
		"Kept Movie (2001)/Kept Movie (2001).mkv",
		"Kept Movie (2001)/Kept Movie (2001)-trailer.mkv",
		"Kept Movie (2001)/Kept Movie (2001)-sample.mkv",
		"Excluded Movie (2002)/Excluded Movie (2002).mkv",
		"Tiny Movie (2003)/Tiny Movie (2003).mkv",
		"Filtered Movie (2004)/Filtered Movie (2004).mkv",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		size := int64(2 * 1024 * 1024) // above the 1 MB floor
		if strings.Contains(f, "Tiny Movie") {
			size = 1 // below the floor
		}
		// Sparse files: instant to create, no real disk usage.
		if err := os.Truncate(p, size); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// itemIDByTitle finds a top-level item id by title.
func itemIDByTitle(t *testing.T, baseURL string, libraryID int64, title string) int64 {
	t.Helper()
	var out struct {
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	getJSON(t, baseURL+"/api/v1/browse?libraryId="+itoa(libraryID)+"&perPage=100", &out)
	for _, it := range out.Items {
		if it.Title == title {
			return it.ID
		}
	}
	return 0
}

// extraTitles reads the extras of an item detail.
func extraTitles(t *testing.T, baseURL string, itemID int64) []string {
	t.Helper()
	var detail struct {
		Extras []struct {
			Title string `json:"title"`
		} `json:"extras"`
	}
	getJSON(t, baseURL+"/api/v1/items/"+itoa(itemID), &detail)
	out := make([]string, 0, len(detail.Extras))
	for _, e := range detail.Extras {
		out = append(out, e.Title)
	}
	return out
}

func browseTitles(t *testing.T, baseURL string, libraryID int64) []string {
	t.Helper()
	var out struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	getJSON(t, baseURL+"/api/v1/browse?libraryId="+itoa(libraryID)+"&perPage=100", &out)
	titles := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		titles = append(titles, it.Title)
	}
	sort.Strings(titles)
	return titles
}

func TestScanRulesCreateScanAndPatch(t *testing.T) {
	srv, st, _ := newTestServer(t)
	root := newScanRulesTree(t)

	// 1. No rules: everything except samples is indexed.
	body, _ := json.Marshal(map[string]any{
		"name": "Movies", "type": "movie", "paths": []string{root},
	})
	res, err := http.Post(srv.URL+"/api/v1/libraries", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create library: %d %s", res.StatusCode, b)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&created)
	if _, err := scanLibrary(t, srv, created.ID); err != nil {
		t.Fatal(err)
	}
	baseline := browseTitles(t, srv.URL, created.ID)
	wantBaseline := []string{"Excluded Movie", "Filtered Movie", "Kept Movie", "Tiny Movie"}
	if strings.Join(baseline, "|") != strings.Join(wantBaseline, "|") {
		t.Fatalf("baseline titles = %v, want %v", baseline, wantBaseline)
	}
	keptID := itemIDByTitle(t, srv.URL, created.ID, "Kept Movie")
	if keptID == 0 {
		t.Fatal("Kept Movie not indexed")
	}
	if extras := extraTitles(t, srv.URL, keptID); len(extras) != 1 || extras[0] != "Trailer" {
		t.Fatalf("baseline extras = %v, want [Trailer]", extras)
	}

	// 2. PATCH rules: exclusion, extra skipping and a 1 MB floor. The handler
	//    queues a scan on its own, so no explicit scan call here.
	patch, _ := json.Marshal(map[string]any{
		"excludeGlobs":  []string{"Excluded Movie*"},
		"minFileSizeMb": 1,
		"skipExtras":    true,
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/libraries/"+itoa(created.ID), bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer pres.Body.Close()
	if pres.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(pres.Body)
		t.Fatalf("patch library: %d %s", pres.StatusCode, b)
	}
	var patched struct {
		ExcludeGlobs  []string `json:"excludeGlobs"`
		IncludeGlobs  []string `json:"includeGlobs"`
		MinFileSizeMB int64    `json:"minFileSizeMb"`
		SkipExtras    bool     `json:"skipExtras"`
	}
	_ = json.NewDecoder(pres.Body).Decode(&patched)
	if len(patched.ExcludeGlobs) != 1 || patched.MinFileSizeMB != 1 || !patched.SkipExtras {
		t.Fatalf("patched rules = %+v", patched)
	}
	if _, err := scanLibrary(t, srv, created.ID); err != nil {
		t.Fatal(err)
	}
	got := browseTitles(t, srv.URL, created.ID)
	want := []string{"Filtered Movie", "Kept Movie"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("after rules titles = %v, want %v", got, want)
	}
	if extras := extraTitles(t, srv.URL, keptID); len(extras) != 0 {
		t.Fatalf("extras after skipExtras = %v, want none", extras)
	}

	// 3. Items excluded by rules stay in the DB (soft-hidden), not deleted.
	refs, err := st.LibraryFileRefs(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	itemHidden := map[int64]bool{}
	for _, r := range refs {
		itemHidden[r.ItemID] = itemHidden[r.ItemID] || r.Hidden
	}
	if len(itemHidden) != 4 {
		t.Fatalf("item rows = %d, want 4 (excluded rows must be kept)", len(itemHidden))
	}
	var hidden int
	for _, h := range itemHidden {
		if h {
			hidden++
		}
	}
	if hidden != 2 {
		t.Fatalf("hidden items = %d, want 2 (excluded and tiny movies)", hidden)
	}

	// 4. Removing the rules restores the previously hidden items.
	clear, _ := json.Marshal(map[string]any{
		"excludeGlobs": []string{}, "minFileSizeMb": 0, "skipExtras": false,
	})
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/libraries/"+itoa(created.ID), bytes.NewReader(clear))
	req.Header.Set("Content-Type", "application/json")
	cres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cres.Body.Close()
	if _, err := scanLibrary(t, srv, created.ID); err != nil {
		t.Fatal(err)
	}
	got = browseTitles(t, srv.URL, created.ID)
	if strings.Join(got, "|") != strings.Join(wantBaseline, "|") {
		t.Fatalf("after clearing rules titles = %v, want %v", got, wantBaseline)
	}
	if extras := extraTitles(t, srv.URL, keptID); len(extras) != 1 {
		t.Fatalf("extras after clearing rules = %v, want the trailer back", extras)
	}
}

func TestScanRulesIncludeFilter(t *testing.T) {
	srv, st, _ := newTestServer(t)
	root := newScanRulesTree(t)

	lib, err := st.CreateLibraryWithOptions(context.Background(), "Kept only", "movie", "", []string{root}, store.LibraryOptions{
		IncludeGlobs: []string{"Kept*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	got := browseTitles(t, srv.URL, lib.ID)
	if len(got) != 1 || got[0] != "Kept Movie" {
		t.Fatalf("include filter titles = %v, want [Kept Movie]", got)
	}
}

func TestScanRulesValidation(t *testing.T) {
	srv, _, _ := newTestServer(t)
	root := t.TempDir()

	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"absolute pattern", map[string]any{"excludeGlobs": []string{"/etc/passwd"}}},
		{"parent traversal", map[string]any{"excludeGlobs": []string{"../escape"}}},
		{"bad glob", map[string]any{"includeGlobs": []string{"[unclosed"}}},
		{"negative size", map[string]any{"minFileSizeMb": -5}},
		{"absurd size", map[string]any{"minFileSizeMb": 5_000_000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"name": "Bad " + tc.name, "type": "movie", "paths": []string{root}}
			for k, v := range tc.fields {
				body[k] = v
			}
			b, _ := json.Marshal(body)
			res, err := http.Post(srv.URL+"/api/v1/libraries", "application/json", bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				raw, _ := io.ReadAll(res.Body)
				t.Fatalf("status = %d %s, want 400", res.StatusCode, raw)
			}
		})
	}
}
