package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
)

func TestCastTokenMediaAccess(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Cast Movie (2020)", "Cast.Movie.2020.mkv"))
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := sc.ScanLibrary(ctx, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %+v err=%v", items, err)
	}
	files, err := st.ItemFiles(ctx, items[0].ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %+v err=%v", files, err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE media_files SET container = ?, video_codec = ?, audio_streams = ? WHERE id = ?`,
		"mov,mp4,m4a,3gp,3g2,mj2", "h264", `[{"index":1,"codec":"aac","channels":2}]`, files[0].ID); err != nil {
		t.Fatal(err)
	}

	var playback struct {
		Cast struct {
			URL        string `json:"url"`
			Compatible bool   `json:"compatible"`
			Reachable  bool   `json:"reachable"`
			Reason     string `json:"reason"`
		} `json:"cast"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(items[0].ID)+"/playback", &playback)
	if !playback.Cast.Compatible || playback.Cast.URL == "" {
		t.Fatalf("cast info = %+v", playback.Cast)
	}
	if playback.Cast.Reachable {
		t.Fatalf("localhost cast URL should not be marked reachable: %+v", playback.Cast)
	}

	anon := &http.Client{}
	res, err := anon.Get(playback.Cast.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("cast fetch = %d %s", res.StatusCode, b)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("cast CORS = %q, want *", got)
	}

	tampered := strings.TrimSuffix(playback.Cast.URL, "A") + "B"
	if tampered == playback.Cast.URL {
		tampered = playback.Cast.URL + "x"
	}
	res2, err := anon.Get(tampered)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered cast token = %d, want 401", res2.StatusCode)
	}
}
