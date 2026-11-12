package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestPreparePlaybackQueuesAndServesVariant walks the ADR 0006 flow: a
// non-direct-play file queues a transcode job, the job writes a cached MP4, and
// the prepared URL serves it with range support.
func TestPreparePlaybackQueuesAndServesVariant(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	clip := filepath.Join(mediaDir, "Prepare Movie (2020)", "Prepare Movie (2020).mkv")
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
	id := itemIDByTitle(t, srv.URL, lib.ID, "Prepare Movie")
	if id == 0 {
		t.Fatal("movie not indexed")
	}

	// MKV is not direct-playable, so this queues a variant.
	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/prepare", map[string]any{})
	var queued struct {
		State     string  `json:"state"`
		JobID     int64   `json:"jobId"`
		VariantID int64   `json:"variantId"`
		Progress  float64 `json:"progress"`
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted && res.StatusCode != http.StatusOK {
		t.Fatalf("prepare: %d %s", res.StatusCode, body)
	}
	if err := json.Unmarshal(body, &queued); err != nil {
		t.Fatalf("decode prepare: %v %s", err, body)
	}
	if queued.State != "queued" || queued.JobID == 0 || queued.VariantID == 0 {
		t.Fatalf("queued prepare = %+v", queued)
	}

	waitJobsIdle(t, srv.URL)

	// Poll again: the variant is ready and has a URL.
	res = postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/prepare", map[string]any{})
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("prepare ready: %d %s", res.StatusCode, body)
	}
	var ready struct {
		State string `json:"state"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(body, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.State != "ready" || ready.URL == "" {
		t.Fatalf("ready prepare = %+v", ready)
	}

	// The output is a cached MP4 under <data>/cache/transcodes.
	variantDir := filepath.Join(dir, "cache", "transcodes", itoa(int64(queued.VariantID)))
	_ = variantDir
	var found string
	_ = filepath.Walk(filepath.Join(dir, "cache", "transcodes"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(path) == ".mp4" {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatal("no variant mp4 written")
	}
	if st, err := os.Stat(found); err != nil || st.Size() == 0 {
		t.Fatalf("variant file = %v %v", st, err)
	}

	// Range request through the prepared URL.
	req, err := http.NewRequest(http.MethodGet, srv.URL+ready.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-1023")
	media, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Body.Close()
	if media.StatusCode != http.StatusPartialContent && media.StatusCode != http.StatusOK {
		t.Fatalf("variant stream = %d", media.StatusCode)
	}
	head := make([]byte, 12)
	n, _ := io.ReadFull(media.Body, head)
	if n < 8 || string(head[4:8]) != "ftyp" {
		t.Fatalf("variant is not an MP4: %q", head[:n])
	}
}

// TestPreparePlaybackDirect: a browser-native MP4 is returned immediately
// without queueing a job.
func TestPreparePlaybackDirect(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	clip := filepath.Join(mediaDir, "Direct Movie (2020)", "Direct Movie (2020).mp4")
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
	id := itemIDByTitle(t, srv.URL, lib.ID, "Direct Movie")

	res := postJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/prepare", map[string]any{})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("prepare direct: %d", res.StatusCode)
	}
	var out struct {
		State string `json:"state"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.State != "direct" || out.URL == "" {
		t.Fatalf("direct prepare = %+v", out)
	}
}
