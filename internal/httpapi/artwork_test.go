package httpapi_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJPEG renders a w×h JPEG poster.
func writeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 60, A: 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}

func imageSize(t *testing.T, res *http.Response) (int, int, string) {
	t.Helper()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("image status = %d %s", res.StatusCode, b)
	}
	img, format, err := image.Decode(res.Body)
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy(), format
}

func putArtworkSettings(t *testing.T, baseURL string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"artwork": body})
	req, _ := http.NewRequest(http.MethodPut, baseURL+"/api/v1/settings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("settings PUT: %d %s", res.StatusCode, b)
	}
}

func waitForIdleJobs(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Jobs []struct {
				Status string `json:"status"`
			} `json:"jobs"`
		}
		getJSON(t, baseURL+"/api/v1/jobs", &list)
		busy := false
		for _, j := range list.Jobs {
			if j.Status == "running" || j.Status == "queued" {
				busy = true
			}
		}
		if !busy {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("jobs did not finish")
}

func TestThumbnailGenerationAndControls(t *testing.T) {
	srv, st, dir := newTestServer(t)
	mediaDir := filepath.Join(dir, "media")
	movieDir := filepath.Join(mediaDir, "Poster Movie (2001)")
	writeFile(t, filepath.Join(movieDir, "Poster Movie (2001).mkv"))
	writeJPEG(t, filepath.Join(movieDir, "poster.jpg"), 800, 1200)
	if err := os.WriteFile(filepath.Join(movieDir, "Poster Movie (2001).nfo"),
		[]byte(`<movie><title>Poster Movie</title><year>2001</year><thumb>poster.jpg</thumb></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := st.CreateLibrary(t.Context(), "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	ids := itemIDs(t, srv, lib.ID)
	if len(ids) != 1 {
		t.Fatalf("items = %v", ids)
	}
	posterURL := srv.URL + "/api/v1/items/" + itoa(ids[0]) + "/poster"

	// Default: poster is served as a 320-wide thumbnail.
	w, h, format := imageSize(t, mustGet(t, posterURL))
	if w != 320 || h != 480 || format != "jpeg" {
		t.Fatalf("default thumb = %dx%d %s, want 320x480 jpeg", w, h, format)
	}

	// ?original=1 bypasses thumbnails.
	w, h, _ = imageSize(t, mustGet(t, posterURL+"?original=1"))
	if w != 800 || h != 1200 {
		t.Fatalf("original = %dx%d, want 800x1200", w, h)
	}

	// Poster size setting applies to newly generated variants.
	putArtworkSettings(t, srv.URL, map[string]any{"posterWidth": 200})
	w, h, _ = imageSize(t, mustGet(t, posterURL))
	if w != 200 || h != 300 {
		t.Fatalf("200px thumb = %dx%d", w, h)
	}

	// Turning generation off serves the original again.
	putArtworkSettings(t, srv.URL, map[string]any{"thumbnails": false})
	w, h, _ = imageSize(t, mustGet(t, posterURL))
	if w != 800 {
		t.Fatalf("thumbnails off = %dpx wide, want original 800", w)
	}
	putArtworkSettings(t, srv.URL, map[string]any{"thumbnails": true, "posterWidth": 320})
	w, _, _ = imageSize(t, mustGet(t, posterURL))
	if w != 320 {
		t.Fatalf("thumbnails back on = %dpx", w)
	}

	// Settings are exposed in the redacted settings payload.
	var settings struct {
		Artwork struct {
			Thumbnails      bool   `json:"thumbnails"`
			PosterWidth     int    `json:"posterWidth"`
			Quality         int    `json:"quality"`
			CoverPreference string `json:"coverPreference"`
		} `json:"artwork"`
	}
	getJSON(t, srv.URL+"/api/v1/settings", &settings)
	if !settings.Artwork.Thumbnails || settings.Artwork.PosterWidth != 320 || settings.Artwork.CoverPreference != "provider" {
		t.Fatalf("settings artwork = %+v", settings.Artwork)
	}

	// Validation.
	raw, _ := json.Marshal(map[string]any{"artwork": map[string]any{"posterWidth": 5}})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad poster width status = %d, want 400", res.StatusCode)
	}

	// Rebuild creates variants for every item with art, as a job.
	post, err := http.Post(srv.URL+"/api/v1/artwork/thumbnails/rebuild", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != http.StatusAccepted {
		t.Fatalf("rebuild status = %d, want 202", post.StatusCode)
	}
	waitForIdleJobs(t, srv.URL)

	var stats struct {
		Files int   `json:"files"`
		Bytes int64 `json:"bytes"`
	}
	getJSON(t, srv.URL+"/api/v1/artwork/thumbnails", &stats)
	if stats.Files == 0 || stats.Bytes == 0 {
		t.Fatalf("thumbnail stats = %+v after rebuild", stats)
	}

	// Clear removes them.
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/artwork/thumbnails", nil)
	dres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var cleared struct {
		Files int `json:"files"`
	}
	_ = json.NewDecoder(dres.Body).Decode(&cleared)
	dres.Body.Close()
	if dres.StatusCode != http.StatusOK || cleared.Files == 0 {
		t.Fatalf("clear = %d %+v", dres.StatusCode, cleared)
	}
	getJSON(t, srv.URL+"/api/v1/artwork/thumbnails", &stats)
	if stats.Files != 0 {
		t.Fatalf("thumbnail stats after clear = %+v", stats)
	}
	// Serving regenerates on demand.
	w, _, _ = imageSize(t, mustGet(t, posterURL))
	if w != 320 {
		t.Fatalf("regenerated thumb = %dpx", w)
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
