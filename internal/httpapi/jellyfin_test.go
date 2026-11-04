package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
)

// jfDo performs a Jellyfin-style request with a token in the MediaBrowser
// authorization header.
func jfDo(t *testing.T, method, url, token string, body []byte) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-Emby-Authorization",
			`MediaBrowser Client="Test", Device="Test Device", DeviceId="device-1", Version="1.0.0", Token="`+token+`"`)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func jfGetJSON(t *testing.T, url, token string, dst any) {
	t.Helper()
	res := jfDo(t, http.MethodGet, url, token, nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("GET %s: %d %s", url, res.StatusCode, b)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func TestJellyfinDiscoveryAndAuth(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Jelly Movie (2010)", "Jelly Movie (2010).mkv"))
	writeJPEG(t, filepath.Join(mediaDir, "Jelly Movie (2010)", "poster.jpg"), 400, 600)
	if err := os.WriteFile(filepath.Join(mediaDir, "Jelly Movie (2010)", "Jelly Movie (2010).nfo"),
		[]byte(`<movie><title>Jelly Movie</title><year>2010</year><thumb>poster.jpg</thumb></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}

	// Public discovery needs no auth.
	var pub struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
		Product    string `json:"ProductName"`
		ID         string `json:"Id"`
	}
	jfGetJSON(t, srv.URL+"/System/Info/Public", "", &pub)
	if pub.ServerName == "" || pub.Version != jellyfin.ServerVersion || len(pub.ID) != 32 {
		t.Fatalf("public info = %+v", pub)
	}

	// Public user list for the login screen.
	var users []struct {
		Name string `json:"Name"`
		ID   string `json:"Id"`
	}
	jfGetJSON(t, srv.URL+"/Users/Public", "", &users)
	if len(users) != 1 || users[0].Name != "Admin" || len(users[0].ID) != 32 {
		t.Fatalf("public users = %+v", users)
	}

	// Login.
	body, _ := json.Marshal(map[string]string{"Username": "admin", "Pw": "test-password-123"})
	res := jfDo(t, http.MethodPost, srv.URL+"/Users/AuthenticateByName", "", body)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("authenticate: %d %s", res.StatusCode, raw)
	}
	var auth struct {
		AccessToken string `json:"AccessToken"`
		ServerID    string `json:"ServerId"`
		User        struct {
			ID   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"User"`
		SessionInfo struct {
			IsActive bool `json:"IsActive"`
		} `json:"SessionInfo"`
	}
	if err := json.NewDecoder(res.Body).Decode(&auth); err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken == "" || auth.User.ID == "" || !auth.SessionInfo.IsActive {
		t.Fatalf("auth = %+v", auth)
	}
	token := auth.AccessToken

	// Wrong password is a bare 401.
	bad, _ := json.Marshal(map[string]string{"Username": "admin", "Pw": "nope"})
	badRes := jfDo(t, http.MethodPost, srv.URL+"/Users/AuthenticateByName", "", bad)
	badRes.Body.Close()
	if badRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", badRes.StatusCode)
	}

	// The token authenticates subsequent calls…
	var me struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	jfGetJSON(t, srv.URL+"/Users/Me", token, &me)
	if me.ID != auth.User.ID {
		t.Fatalf("me = %+v, want id %s", me, auth.User.ID)
	}
	// … and a request with neither a token nor the SPA cookie is rejected.
	noAuthReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/Users/Me", nil)
	noAuthReq.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Test"`)
	noAuth, err := (&http.Client{}).Do(noAuthReq) // no cookie jar
	if err != nil {
		t.Fatal(err)
	}
	noAuth.Body.Close()
	if noAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth /Users/Me = %d", noAuth.StatusCode)
	}

	// Views = libraries.
	var views struct {
		Items []struct {
			ID             string `json:"Id"`
			Name           string `json:"Name"`
			Type           string `json:"Type"`
			CollectionType string `json:"CollectionType"`
			IsFolder       bool   `json:"IsFolder"`
		} `json:"Items"`
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	jfGetJSON(t, srv.URL+"/Users/"+auth.User.ID+"/Views", token, &views)
	if views.TotalRecordCount != 1 || len(views.Items) != 1 {
		t.Fatalf("views = %+v", views)
	}
	view := views.Items[0]
	if view.Name != "Movies" || view.Type != "CollectionFolder" || view.CollectionType != "movies" || !view.IsFolder {
		t.Fatalf("view = %+v", view)
	}

	// Browse the library.
	var list struct {
		Items []struct {
			ID        string `json:"Id"`
			Name      string `json:"Name"`
			Type      string `json:"Type"`
			ImageTags map[string]string
		} `json:"Items"`
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	jfGetJSON(t, srv.URL+"/Users/"+auth.User.ID+"/Items?ParentId="+view.ID+"&IncludeItemTypes=Movie&Recursive=true", token, &list)
	if list.TotalRecordCount != 1 || len(list.Items) != 1 {
		t.Fatalf("items = %+v", list)
	}
	movie := list.Items[0]
	if movie.Name != "Jelly Movie" || movie.Type != "Movie" {
		t.Fatalf("movie = %+v", movie)
	}
	if movie.ImageTags["Primary"] == "" {
		t.Fatalf("movie image tags = %+v", movie.ImageTags)
	}

	// Item detail includes media sources and provider/user data.
	var detail struct {
		ID           string `json:"Id"`
		Type         string `json:"Type"`
		RunTimeTicks int64  `json:"RunTimeTicks"`
		UserData     struct {
			Played bool `json:"Played"`
		} `json:"UserData"`
		MediaSources []struct {
			ID                 string `json:"Id"`
			Container          string `json:"Container"`
			SupportsDirectPlay bool   `json:"SupportsDirectPlay"`
			MediaStreams       []struct {
				Type  string `json:"Type"`
				Index int    `json:"Index"`
			} `json:"MediaStreams"`
		} `json:"MediaSources"`
	}
	jfGetJSON(t, srv.URL+"/Items/"+movie.ID, token, &detail)
	if detail.Type != "Movie" || len(detail.MediaSources) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.MediaSources[0].ID == "" || detail.MediaSources[0].Container != "matroska" {
		t.Fatalf("media source = %+v", detail.MediaSources[0])
	}

	// Image endpoint (token in the api_key query, as clients do).
	imgRes, err := http.Get(srv.URL + "/Items/" + movie.ID + "/Images/Primary?api_key=" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer imgRes.Body.Close()
	if imgRes.StatusCode != http.StatusOK {
		t.Fatalf("image status = %d", imgRes.StatusCode)
	}
	if ct := imgRes.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Fatalf("image content type = %q", ct)
	}
	if _, err := io.Copy(io.Discard, imgRes.Body); err != nil {
		t.Fatal(err)
	}

	// Search hints.
	var hints struct {
		SearchHints []struct {
			ItemID string `json:"ItemId"`
			Name   string `json:"Name"`
			Type   string `json:"Type"`
		} `json:"SearchHints"`
	}
	jfGetJSON(t, srv.URL+"/Search/Hints?searchTerm=Jelly", token, &hints)
	if len(hints.SearchHints) == 0 || hints.SearchHints[0].Type != "Movie" {
		t.Fatalf("hints = %+v", hints)
	}
}

func TestJellyfinBrowseTree(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()

	tvDir := filepath.Join(dir, "tv", "Some Show (2020)", "Season 1")
	writeFile(t, filepath.Join(tvDir, "Some Show.S01E01.mkv"))
	writeFile(t, filepath.Join(tvDir, "Some Show.S01E02.mkv"))
	writeFile(t, filepath.Join(dir, "tv", "Some Show (2020)", "Season 2", "Some Show.S02E01.mkv"))
	lib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{filepath.Join(dir, "tv")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, lib.ID); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"Username": "admin", "Pw": "test-password-123"})
	res := jfDo(t, http.MethodPost, srv.URL+"/Users/AuthenticateByName", "", body)
	defer res.Body.Close()
	var auth struct {
		AccessToken string `json:"AccessToken"`
	}
	_ = json.NewDecoder(res.Body).Decode(&auth)
	token := auth.AccessToken

	type item struct {
		ID                string `json:"Id"`
		Name              string `json:"Name"`
		Type              string `json:"Type"`
		ParentID          string `json:"ParentId"`
		SeriesID          string `json:"SeriesId"`
		IndexNumber       *int   `json:"IndexNumber"`
		ParentIndexNumber *int   `json:"ParentIndexNumber"`
	}
	var shows struct {
		Items []item `json:"Items"`
	}
	jfGetJSON(t, srv.URL+"/Items?IncludeItemTypes=Series&Recursive=true", token, &shows)
	if len(shows.Items) != 1 {
		t.Fatalf("shows = %+v", shows)
	}
	show := shows.Items[0]

	var seasons struct {
		Items []item `json:"Items"`
	}
	jfGetJSON(t, srv.URL+"/Items?ParentId="+show.ID, token, &seasons)
	if len(seasons.Items) != 2 {
		t.Fatalf("seasons = %+v", seasons)
	}
	for i, s := range seasons.Items {
		if s.Type != "Season" || s.ParentID != show.ID || s.IndexNumber == nil || *s.IndexNumber != i+1 {
			t.Fatalf("season %d = %+v", i, s)
		}
	}

	var episodes struct {
		Items []item `json:"Items"`
	}
	jfGetJSON(t, srv.URL+"/Items?ParentId="+show.ID+"&IncludeItemTypes=Episode&Recursive=true", token, &episodes)
	if len(episodes.Items) != 3 {
		t.Fatalf("episodes = %+v", episodes)
	}
	if episodes.Items[0].ParentIndexNumber == nil || *episodes.Items[0].ParentIndexNumber != 1 {
		t.Fatalf("episode ordering = %+v", episodes.Items[0])
	}

	// Episode detail resolves the season/series parents.
	var ep item
	jfGetJSON(t, srv.URL+"/Items/"+episodes.Items[0].ID, token, &ep)
	if ep.SeriesID != show.ID || ep.ParentID == "" {
		t.Fatalf("episode detail = %+v", ep)
	}

	// Unknown/foreign ids are 404s, not 500s.
	bad := jfDo(t, http.MethodGet, srv.URL+"/Items/not-an-id", token, nil)
	bad.Body.Close()
	if bad.StatusCode != http.StatusNotFound {
		t.Fatalf("bad item id status = %d", bad.StatusCode)
	}
}
