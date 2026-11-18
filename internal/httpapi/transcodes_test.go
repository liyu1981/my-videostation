package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type variantRow struct {
	ID           int64  `json:"id"`
	FileID       int64  `json:"fileId"`
	Path         string `json:"path"`
	State        string `json:"state"`
	Bytes        int64  `json:"bytes"`
	OriginalPath string `json:"originalPath"`
	Title        string `json:"title"`
}

// TestTranscodeCacheAdmin: variants are listed for one item and globally, and
// deleting one removes both its row and its cached file.
func TestTranscodeCacheAdmin(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	clip := filepath.Join(mediaDir, "Cache Movie (2020)", "Cache Movie (2020).mkv")
	if err := os.MkdirAll(filepath.Dir(clip), 0o755); err != nil {
		t.Fatal(err)
	}
	makeClip(t, ffmpeg, clip, "libx264", "aac")
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	id := itemIDByTitle(t, srv.URL, lib.ID, "Cache Movie")
	if id == 0 {
		t.Fatal("movie not indexed")
	}

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/prepare", map[string]any{})
	var prep struct {
		VariantID int64 `json:"variantId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&prep); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if prep.VariantID == 0 {
		t.Fatal("prepare did not create a variant")
	}
	waitJobsIdle(t, srv.URL)

	var itemList struct {
		Variants []variantRow `json:"variants"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/variants", &itemList)
	if len(itemList.Variants) != 1 || itemList.Variants[0].State != "ready" {
		t.Fatalf("item variants = %+v", itemList.Variants)
	}
	variant := itemList.Variants[0]
	if variant.Path == "" || variant.OriginalPath != clip || variant.Title != "Cache Movie" || variant.Bytes <= 0 {
		t.Fatalf("variant info = %+v", variant)
	}

	var all struct {
		Variants   []variantRow `json:"variants"`
		TotalBytes int64        `json:"totalBytes"`
	}
	getJSON(t, srv.URL+"/api/v1/transcodes", &all)
	if len(all.Variants) != 1 || all.TotalBytes <= 0 {
		t.Fatalf("transcodes = %+v", all)
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/transcodes/"+itoa(variant.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	del, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete variant: %d", del.StatusCode)
	}
	if _, err := os.Stat(variant.Path); !os.IsNotExist(err) {
		t.Fatalf("variant file still exists: %v", err)
	}
	getJSON(t, srv.URL+"/api/v1/transcodes", &all)
	if len(all.Variants) != 0 {
		t.Fatalf("variants after delete = %+v", all.Variants)
	}
}
