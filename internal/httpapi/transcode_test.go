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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/httpapi"
	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/media"
	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
)

// newTranscodeTestServer wires a server that can actually probed and
// transcoded media (requires ffmpeg/ffprobe).
func newTranscodeTestServer(t *testing.T) (*httptest.Server, *store.Store, string, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	prober, err := media.Detect("")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
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
	scan := scanner.New(st, prober, log)
	manager.Handle("scan", func(ctx context.Context, job *store.Job, report jobs.Progress) error {
		var payload struct {
			LibraryID int64  `json:"libraryId"`
			Force     bool   `json:"force"`
			Scope     string `json:"scope"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		if payload.Scope != "" {
			_, err := scan.ScanLibraryScoped(ctx, payload.LibraryID, payload.Scope, payload.Force, func(float64, string) {})
			return err
		}
		_, err := scan.ScanLibraryForce(ctx, payload.LibraryID, payload.Force, func(float64, string) {})
		return err
	})
	manager.Start(ctx)
	cacheDir := filepath.Join(dir, "cache")
	scan.SetCacheDir(cacheDir)
	api := httpapi.New(httpapi.Options{
		Store: st, Jobs: manager, Logger: log, DataDir: dir, CacheDir: cacheDir,
		FFmpeg:            ffmpeg,
		TranscodeSessions: 2,
		TranscodeIdle:     time.Minute,
		TranscodeEncoder:  "libx264",
		TranscodePaced:    false,
	})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv, st, dir, ffmpeg
}

// makeClip renders a short sample with the requested codecs.
func makeClip(t *testing.T, ffmpeg, out, vcodec, acodec string) {
	t.Helper()
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "6", "-c:v", vcodec, "-g", "25", "-c:a", acodec, "-shortest", out,
	}
	if vcodec == "libx264" {
		args = append(args[:len(args)-4], append([]string{"-preset", "ultrafast"}, args[len(args)-4:]...)...)
	}
	if b, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("create clip %s: %v\n%s", out, err, b)
	}
}

func TestPlaybackInfoAndCapabilities(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "movies")
	for _, d := range []string{"Remux Movie (2020)", "Legacy Movie (1999)"} {
		if err := os.MkdirAll(filepath.Join(mediaDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	makeClip(t, ffmpeg, filepath.Join(mediaDir, "Remux Movie (2020)", "Remux Movie (2020).mkv"), "libx264", "aac")
	makeClip(t, ffmpeg, filepath.Join(mediaDir, "Legacy Movie (1999)", "Legacy Movie (1999).avi"), "mpeg4", "libmp3lame")

	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}

	var caps struct {
		Transcoding struct {
			Available bool   `json:"available"`
			Encoder   string `json:"encoder"`
		} `json:"transcoding"`
	}
	getJSON(t, srv.URL+"/api/v1/playback/capabilities", &caps)
	if !caps.Transcoding.Available || caps.Transcoding.Encoder != "libx264" {
		t.Fatalf("capabilities = %+v", caps)
	}

	for _, id := range itemIDs(t, srv, lib.ID) {
		var pb struct {
			ItemID     int64    `json:"itemId"`
			FileID     int64    `json:"fileId"`
			Container  string   `json:"container"`
			VideoCodec string   `json:"videoCodec"`
			Height     int      `json:"height"`
			DirectPlay bool     `json:"directPlay"`
			HLS        bool     `json:"hls"`
			Reasons    []string `json:"reasons"`
			Qualities  []struct {
				Label  string `json:"label"`
				Height int    `json:"height"`
			} `json:"qualities"`
		}
		getJSON(t, srv.URL+"/api/v1/items/"+itoa(id)+"/playback", &pb)
		if pb.DirectPlay {
			t.Fatalf("item %d (%s/%s) should need transcoding", id, pb.Container, pb.VideoCodec)
		}
		if !pb.HLS {
			t.Fatalf("item %d: hls = false with ffmpeg configured", id)
		}
		if len(pb.Reasons) == 0 {
			t.Fatalf("item %d: no transcoding reason", id)
		}
		if len(pb.Qualities) == 0 || pb.Qualities[0].Height != 0 {
			t.Fatalf("item %d: qualities = %+v", id, pb.Qualities)
		}
	}
}

func TestTranscodeSessionHTTP(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)

	mediaDir := filepath.Join(dir, "movies")
	if err := os.MkdirAll(filepath.Join(mediaDir, "Legacy Movie (1999)"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeClip(t, ffmpeg, filepath.Join(mediaDir, "Legacy Movie (1999)", "Legacy Movie (1999).avi"), "mpeg4", "libmp3lame")
	lib, err := st.CreateLibrary(context.Background(), "Movies", "movie", "", []string{mediaDir})
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

	var started struct {
		SessionID   string   `json:"sessionId"`
		PlaylistURL string   `json:"playlistUrl"`
		StartMS     int64    `json:"startMs"`
		DurationMS  int64    `json:"durationMs"`
		Mode        string   `json:"mode"`
		Height      int      `json:"height"`
		Reasons     []string `json:"reasons"`
	}
	body, _ := json.Marshal(map[string]any{"startMs": 0, "maxHeight": 120})
	res, err := http.Post(srv.URL+"/api/v1/playback/"+itoa(ids[0])+"/transcode", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("transcode start: %d %s", res.StatusCode, b)
	}
	if err := json.NewDecoder(res.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	if started.Mode != "transcode" || started.Height != 120 {
		t.Fatalf("start response = %+v", started)
	}
	if started.SessionID == "" || !strings.HasSuffix(started.PlaylistURL, "/index.m3u8") {
		t.Fatalf("start response = %+v", started)
	}
	if started.DurationMS <= 0 {
		t.Fatalf("duration = %d", started.DurationMS)
	}

	waitForPlaylist(t, srv.URL+started.PlaylistURL)
	if seg := firstSegmentURL(t, srv.URL+started.PlaylistURL); seg != "" {
		res, err := http.Get(seg)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("segment %s: %d", seg, res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "iso.segment") {
			t.Fatalf("segment content type = %q", ct)
		}
	}

	// Seeking restarts the job at the requested offset.
	seekBody, _ := json.Marshal(map[string]any{"startMs": 4000})
	sres, err := http.Post(srv.URL+"/api/v1/playback/"+itoa(ids[0])+"/transcode", "application/json", bytes.NewReader(seekBody))
	if err != nil {
		t.Fatal(err)
	}
	defer sres.Body.Close()
	if sres.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(sres.Body)
		t.Fatalf("seek transcode: %d %s", sres.StatusCode, b)
	}
	var seeked struct {
		SessionID  string `json:"sessionId"`
		StartMS    int64  `json:"startMs"`
		DurationMS int64  `json:"durationMs"`
	}
	_ = json.NewDecoder(sres.Body).Decode(&seeked)
	if seeked.SessionID == started.SessionID {
		t.Fatal("seek should have started a new session")
	}
	if seeked.StartMS != 4000 || seeked.DurationMS != started.DurationMS-4000 {
		t.Fatalf("seek response = %+v (original duration %d)", seeked, started.DurationMS)
	}

	// Status endpoint reports progress.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var status struct {
			EncodedMS int64  `json:"encodedMs"`
			Running   bool   `json:"running"`
			Error     string `json:"error"`
		}
		getJSON(t, srv.URL+"/api/v1/playback/sessions/"+seeked.SessionID, &status)
		if status.Error != "" {
			t.Fatalf("session failed: %s", status.Error)
		}
		if status.EncodedMS > 0 || !status.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session never reported progress")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Stop removes the session.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/playback/sessions/"+started.SessionID, nil)
	dres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dres.Body.Close()
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("delete session: %d", dres.StatusCode)
	}
	res, err = http.Get(srv.URL + started.PlaylistURL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("playlist after stop: %d", res.StatusCode)
	}
}

func TestTranscodeUnavailableWithoutFFmpeg(t *testing.T) {
	srv, st, dir := newTestServer(t)
	mediaDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(mediaDir, "Some Movie (2001)", "Some Movie (2001).mkv"))
	lib, err := st.CreateLibrary(context.Background(), "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	ids := itemIDs(t, srv, lib.ID)
	body, _ := json.Marshal(map[string]any{"startMs": 0})
	res, err := http.Post(srv.URL+"/api/v1/playback/"+itoa(ids[0])+"/transcode", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d %s, want 503", res.StatusCode, b)
	}
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), "ffmpeg_missing") {
		t.Fatalf("body = %s", b)
	}

	res, err = http.Get(srv.URL + "/hls/nope/index.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown session: %d", res.StatusCode)
	}
}

// scanLibrary asks the API to scan a library and waits for it.
func scanLibrary(t *testing.T, srv *httptest.Server, libraryID int64) (int, error) {
	t.Helper()
	res, err := http.Post(srv.URL+"/api/v1/libraries/"+itoa(libraryID)+"/scan", "application/json", nil)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("scan: %d %s", res.StatusCode, b)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Jobs []struct {
				Status string `json:"status"`
			} `json:"jobs"`
		}
		getJSON(t, srv.URL+"/api/v1/jobs", &list)
		if len(list.Jobs) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		running := false
		for _, j := range list.Jobs {
			if j.Status == "running" || j.Status == "queued" || j.Status == "pending" {
				running = true
			}
		}
		if !running {
			return res.StatusCode, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
	return 0, nil
}

func itemIDs(t *testing.T, srv *httptest.Server, libraryID int64) []int64 {
	t.Helper()
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?libraryId="+itoa(libraryID), &list)
	out := make([]int64, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, it.ID)
	}
	return out
}

// waitForPlaylist polls the HLS playlist until the transcode produced it.
func waitForPlaylist(t *testing.T, playlistURL string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(playlistURL)
		if err == nil {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode == http.StatusOK && strings.Contains(string(body), "#EXTM3U") {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("playlist never became available")
}

func firstSegmentURL(t *testing.T, playlistURL string) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(playlistURL)
		if err == nil {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			for _, line := range strings.Split(string(body), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasSuffix(line, ".m4s") {
					// Playlist URLs are requested relative to the playlist.
					u, err := url.Parse(playlistURL)
					if err != nil {
						return ""
					}
					u.Path = u.Path[:strings.LastIndex(u.Path, "/")+1] + line
					return u.String()
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}
