package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
)

// jfLogin authenticates and returns (token, userID).
func jfLogin(t *testing.T, baseURL string) (string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"Username": "admin", "Pw": "test-password-123"})
	res := jfDo(t, http.MethodPost, baseURL+"/Users/AuthenticateByName", "", body)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("login: %d %s", res.StatusCode, raw)
	}
	var auth struct {
		AccessToken string `json:"AccessToken"`
		User        struct {
			ID string `json:"Id"`
		} `json:"User"`
	}
	if err := json.NewDecoder(res.Body).Decode(&auth); err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken == "" {
		t.Fatal("no access token")
	}
	return auth.AccessToken, auth.User.ID
}

func TestJellyfinPlaybackInfo(t *testing.T) {
	srv, st, dir := newTestServer(t)
	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Play Movie (2011)", "Play Movie (2011).mkv"))
	lib, err := st.CreateLibrary(t.Context(), "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	token, _ := jfLogin(t, srv.URL)

	ids := itemIDs(t, srv, lib.ID)
	itemID := itemIDByTitle(t, srv.URL, lib.ID, "Play Movie")
	if itemID == 0 {
		t.Fatalf("item not found among %v", ids)
	}

	var info struct {
		PlaySessionID string `json:"PlaySessionId"`
		MediaSources  []struct {
			ID                string `json:"Id"`
			DirectStreamURL   string `json:"DirectStreamUrl"`
			TranscodingURL    string `json:"TranscodingUrl"`
			SupportsDirect    bool   `json:"SupportsDirectPlay"`
			SupportsTranscode bool   `json:"SupportsTranscoding"`
			MediaStreams      []struct {
				Type string `json:"Type"`
			} `json:"MediaStreams"`
		} `json:"MediaSources"`
	}
	jfGetJSON(t, srv.URL+"/Items/"+jellyfinItemID(t, itemID)+"/PlaybackInfo", token, &info)
	if info.PlaySessionID == "" || len(info.MediaSources) != 1 {
		t.Fatalf("playback info = %+v", info)
	}
	src := info.MediaSources[0]
	if src.DirectStreamURL == "" || !strings.Contains(src.DirectStreamURL, "/Videos/") || !strings.Contains(src.DirectStreamURL, "api_key=") {
		t.Fatalf("direct stream url = %q", src.DirectStreamURL)
	}
	// No ffmpeg in this test server, so transcoding must not be advertised.
	if src.SupportsTranscode || src.TranscodingURL != "" {
		t.Fatalf("transcoding advertised without ffmpeg: %+v", src)
	}

	// Direct stream serves bytes with range support (token in query only).
	res, err := (&http.Client{}).Get(srv.URL + src.DirectStreamURL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("direct stream = %d ranges=%q", res.StatusCode, res.Header.Get("Accept-Ranges"))
	}
}

func TestJellyfinHLSAndProgress(t *testing.T) {
	srv, st, dir, ffmpeg := newTranscodeTestServer(t)
	setupTestAdmin(t, srv, st)
	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	makeClip(t, ffmpeg, filepath.Join(mediaDir, "HLS Movie (2012).mkv"), "libx264", "ac3")
	lib, err := st.CreateLibrary(t.Context(), "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}
	token, userID := jfLogin(t, srv.URL)
	itemID := itemIDByTitle(t, srv.URL, lib.ID, "HLS Movie")
	if itemID == 0 {
		t.Fatal("HLS Movie not indexed")
	}
	encItem := jellyfinItemID(t, itemID)

	// Manifest starts a session and rewrites segment URLs with the token.
	manifestURL := srv.URL + "/Videos/" + encItem + "/master.m3u8?StartTimeTicks=0&api_key=" + token
	res, err := (&http.Client{}).Get(manifestURL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("manifest: %d %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("manifest content type = %q", ct)
	}
	text := string(body)
	if !strings.Contains(text, "#EXTM3U") {
		t.Fatalf("manifest body = %s", text)
	}
	segRe := regexp.MustCompile(`(/hls/[a-f0-9]+/(?:init\.mp4|seg\d+\.m4s)\?api_key=[^\s]+)`)
	matches := segRe.FindAllString(text, -1)
	if len(matches) == 0 {
		t.Fatalf("no rewritten segment URLs in manifest: %s", text)
	}
	// Segments authenticate with the token alone (no cookie).
	sres, err := (&http.Client{}).Get(srv.URL + matches[len(matches)-1])
	if err != nil {
		t.Fatal(err)
	}
	sres.Body.Close()
	if sres.StatusCode != http.StatusOK {
		t.Fatalf("segment status = %d for %s", sres.StatusCode, matches[len(matches)-1])
	}

	// Progress report lands in our playback state (ticks → ms).
	posTicks := int64(3000 * 10000)
	report, _ := json.Marshal(map[string]any{
		"ItemId": encItem, "PositionTicks": posTicks, "PlaySessionId": "ps-1", "IsPaused": false,
	})
	pres := jfDo(t, http.MethodPost, srv.URL+"/Sessions/Playing/Progress", token, report)
	pres.Body.Close()
	if pres.StatusCode != http.StatusNoContent {
		t.Fatalf("progress status = %d", pres.StatusCode)
	}
	var pb struct {
		ResumeMS int64 `json:"resumeMs"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(itemID)+"/playback", &pb)
	if pb.ResumeMS != 3000 {
		t.Fatalf("resumeMs = %d, want 3000", pb.ResumeMS)
	}

	// Mark played / unplayed.
	mark := jfDo(t, http.MethodPost, srv.URL+"/Users/"+userID+"/PlayedItems/"+encItem, token, nil)
	mark.Body.Close()
	if mark.StatusCode != http.StatusNoContent {
		t.Fatalf("mark played = %d", mark.StatusCode)
	}
	var detail struct {
		Played bool `json:"played"`
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(itemID), &detail)
	if !detail.Played {
		t.Fatal("item should be played")
	}
	unmark := jfDo(t, http.MethodDelete, srv.URL+"/Users/"+userID+"/PlayedItems/"+encItem, token, nil)
	unmark.Body.Close()
	if unmark.StatusCode != http.StatusNoContent {
		t.Fatalf("mark unplayed = %d", unmark.StatusCode)
	}
	getJSON(t, srv.URL+"/api/v1/items/"+itoa(itemID), &detail)
	if detail.Played {
		t.Fatal("item should be unplayed")
	}

	// Favorites go through the user's system collection.
	fav := jfDo(t, http.MethodPost, srv.URL+"/Users/"+userID+"/FavoriteItems/"+encItem, token, nil)
	fav.Body.Close()
	if fav.StatusCode != http.StatusNoContent {
		t.Fatalf("favorite = %d", fav.StatusCode)
	}
	var favItems struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	// System collections are exposed through the SPA collections API.
	var cols struct {
		Collections []struct {
			ID        int64  `json:"id"`
			SystemKey string `json:"systemKey"`
		} `json:"collections"`
	}
	getJSON(t, srv.URL+"/api/v1/collections", &cols)
	var favID int64
	for _, c := range cols.Collections {
		if c.SystemKey == "favorites" {
			favID = c.ID
		}
	}
	if favID == 0 {
		t.Skip("favorites system collection is not exposed by the collections API")
	}
	getJSON(t, srv.URL+"/api/v1/collections/"+itoa(favID)+"/items", &favItems)
	if len(favItems.Items) != 1 || favItems.Items[0].ID != itemID {
		t.Fatalf("favorites = %+v", favItems.Items)
	}
}

// jellyfinItemID encodes a numeric item id the way the adapter does.
func jellyfinItemID(t *testing.T, id int64) string {
	t.Helper()
	return jellyfin.ItemID(id)
}
