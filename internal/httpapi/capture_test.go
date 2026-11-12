package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCaptureBackdropFromVideo grabs a frame at the requested second and stores
// it under the data cache (never the media folder).
func TestCaptureBackdropFromVideo(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	clip := filepath.Join(mediaDir, "Clip Movie (2020)", "Clip Movie (2020).mkv")
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
	id := itemIDByTitle(t, srv.URL, lib.ID, "Clip Movie")
	if id == 0 {
		t.Fatal("movie not indexed")
	}

	// Beyond the clip's duration: the server clamps instead of failing.
	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/backdrop/capture", map[string]any{"positionMs": 999000})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("capture: %d", res.StatusCode)
	}
	var detail struct {
		BackdropURL string `json:"backdropUrl"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.BackdropURL == "" {
		t.Fatal("capture did not set a backdrop")
	}

	// The JPEG is written under <data>/cache/images, not beside the media.
	imagesDir := filepath.Join(dir, "cache", "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		t.Fatalf("read cache images: %v", err)
	}
	var captured string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backdrop-") && strings.HasSuffix(e.Name(), ".jpg") {
			captured = filepath.Join(imagesDir, e.Name())
		}
	}
	if captured == "" {
		t.Fatalf("no captured backdrop in %s", imagesDir)
	}
	data, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatalf("captured file is not a JPEG (%d bytes)", len(data))
	}
	// The media folder stays untouched.
	if _, err := os.Stat(filepath.Join(filepath.Dir(clip), "backdrop.jpg")); err == nil {
		t.Fatal("capture wrote into the media folder")
	}
}

// TestCaptureBackdropWithoutFFmpeg reports a clear 503.
func TestCaptureBackdropWithoutFFmpeg(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(mediaDir, "No Ffmpeg (2020)", "No Ffmpeg (2020).mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	id := itemIDByTitle(t, srv.URL, lib.ID, "No Ffmpeg")
	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/backdrop/capture", map[string]any{"positionMs": 0})
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capture without ffmpeg = %d, want 503", res.StatusCode)
	}
}
