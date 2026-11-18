package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

// TestSplitFileIntoEditableItem: a file merged into a multi-file movie can be
// split into its own home-video item and edited independently.
func TestSplitFileIntoEditableItem(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	movieDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(movieDir, "Split Movie (2020)", "Split Movie (2020) Part1.mkv"))
	writeFile(t, filepath.Join(movieDir, "Split Movie (2020)", "Split Movie (2020) Part2.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{movieDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	itemID := itemIDByTitle(t, srv.URL, lib.ID, "Split Movie")
	if itemID == 0 {
		t.Fatal("movie not indexed")
	}
	files, err := st.ItemFiles(ctx, itemID)
	if err != nil || len(files) != 2 {
		t.Fatalf("files before split = %+v (%v)", files, err)
	}
	fileID := files[0].ID

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(itemID)+"/files/"+itoa(fileID)+"/split", map[string]any{})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("split: %d", res.StatusCode)
	}
	var created itemDetail
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == itemID || created.Kind != "home_video" {
		t.Fatalf("split item = %+v", created)
	}
	newFiles, err := st.ItemFiles(ctx, created.ID)
	if err != nil || len(newFiles) != 1 || newFiles[0].ID != fileID {
		t.Fatalf("split files = %+v (%v)", newFiles, err)
	}
	oldFiles, err := st.ItemFiles(ctx, itemID)
	if err != nil || len(oldFiles) != 1 {
		t.Fatalf("source files after split = %+v (%v)", oldFiles, err)
	}

	// A periodic scan must not re-merge the two items.
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetItem(ctx, created.ID); err != nil {
		t.Fatalf("split item disappeared after scan: %v", err)
	}
	var count int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items WHERE library_id = ? AND kind IN ('movie','home_video')`, lib.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("items after scan = %d, want 2", count)
	}
}
