package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
)

type jfTestEnv struct {
	srv      string
	movieLib int64
	tvLib    int64
	dramaID  int64
	comedyID int64
	showID   int64
	ep1      int64
	ep2      int64
	token    string
	userID   string
}

// setupJellyfinBrowse builds a movie + TV library with metadata, art and watch
// state, and returns a logged-in adapter environment.
func setupJellyfinBrowse(t *testing.T) *jfTestEnv {
	t.Helper()
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	env := &jfTestEnv{srv: srv.URL}

	movieDir := filepath.Join(dir, "movies")
	writeFile(t, filepath.Join(movieDir, "Drama Movie (2001)", "Drama Movie (2001).mkv"))
	if err := os.WriteFile(filepath.Join(movieDir, "Drama Movie (2001)", "Drama Movie (2001).nfo"), []byte(
		`<movie><title>Drama Movie</title><year>2001</year><genre>Drama</genre><mpaa>R</mpaa>
		 <actor><name>NFO Actor</name></actor></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(movieDir, "Drama Movie (2001)", "Drama Movie (2001)-trailer.mkv"))
	if err := os.WriteFile(filepath.Join(movieDir, "Drama Movie (2001)", "Drama Movie (2001).en.srt"),
		[]byte("1\n00:00:01,000 --> 00:00:03,000\nHello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(movieDir, "Comedy Movie (2002)", "Comedy Movie (2002).mkv"))
	if err := os.WriteFile(filepath.Join(movieDir, "Comedy Movie (2002)", "Comedy Movie (2002).nfo"), []byte(
		`<movie><title>Comedy Movie</title><year>2002</year><genre>Comedy</genre><mpaa>PG-13</mpaa></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}

	showDir := filepath.Join(dir, "tv", "Some Show (2020)")
	writeJPEG(t, filepath.Join(showDir, "poster.jpg"), 400, 600)
	writeJPEG(t, filepath.Join(showDir, "fanart.jpg"), 1920, 1080)
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(
		`<tvshow><title>Some Show</title><genre>Drama</genre>
		 <thumb>poster.jpg</thumb><fanart><thumb>fanart.jpg</thumb></fanart></tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(showDir, "Season 1", "Some Show.S01E01.mkv"))
	writeFile(t, filepath.Join(showDir, "Season 1", "Some Show.S01E02.mkv"))
	writeFile(t, filepath.Join(showDir, "Season 2", "Some Show.S02E01.mkv"))

	movieLib, err := st.CreateLibrary(ctx, "Movies", "movie", "", []string{movieDir})
	if err != nil {
		t.Fatal(err)
	}
	tvLib, err := st.CreateLibrary(ctx, "TV", "tv", "", []string{filepath.Join(dir, "tv")})
	if err != nil {
		t.Fatal(err)
	}
	env.movieLib, env.tvLib = movieLib.ID, tvLib.ID
	if _, err := scanLibrary(t, srv, env.movieLib); err != nil {
		t.Fatal(err)
	}
	if _, err := scanLibrary(t, srv, env.tvLib); err != nil {
		t.Fatal(err)
	}

	env.dramaID = itemIDByTitle(t, srv.URL, env.movieLib, "Drama Movie")
	env.comedyID = itemIDByTitle(t, srv.URL, env.movieLib, "Comedy Movie")
	env.showID = itemIDByTitle(t, srv.URL, env.tvLib, "Some Show")
	if env.dramaID == 0 || env.comedyID == 0 || env.showID == 0 {
		t.Fatalf("missing items: drama=%d comedy=%d show=%d", env.dramaID, env.comedyID, env.showID)
	}
	var episodes struct {
		Items []struct {
			ID      int64 `json:"id"`
			Episode int   `json:"episodeNumber"`
			Season  int   `json:"seasonNumber"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/v1/browse?libraryId="+itoa(env.tvLib)+"&kind=episode&perPage=100", &episodes)
	for _, e := range episodes.Items {
		if e.Season == 1 && e.Episode == 1 {
			env.ep1 = e.ID
		}
		if e.Season == 1 && e.Episode == 2 {
			env.ep2 = e.ID
		}
	}
	if env.ep1 == 0 || env.ep2 == 0 {
		t.Fatalf("episodes not indexed: %+v", episodes.Items)
	}
	env.token, env.userID = jfLogin(t, srv.URL)

	// S1E1 watched, S1E2 in progress, Drama resumable, Comedy watched.
	jfMarkPlayed(t, env, env.ep1)
	jfSetPosition(t, env, env.ep2, 5000)
	jfSetPosition(t, env, env.dramaID, 5000)
	jfMarkPlayed(t, env, env.comedyID)

	// Favorite the drama through the adapter.
	res := jfDo(t, http.MethodPost, srv.URL+"/Users/"+env.userID+"/FavoriteItems/"+jellyfinItemID(t, env.dramaID), env.token, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("favorite: %d", res.StatusCode)
	}
	return env
}

func jfSetPosition(t *testing.T, env *jfTestEnv, itemID, positionMS int64) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"ItemId": jellyfinItemID(t, itemID), "PositionTicks": positionMS * 10000, "IsPaused": false,
	})
	res := jfDo(t, http.MethodPost, env.srv+"/Sessions/Playing/Progress", env.token, body)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("progress: %d", res.StatusCode)
	}
}

func jfMarkPlayed(t *testing.T, env *jfTestEnv, itemID int64) {
	t.Helper()
	res := jfDo(t, http.MethodPost,
		env.srv+"/Users/"+env.userID+"/PlayedItems/"+jellyfinItemID(t, itemID), env.token, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("mark played: %d", res.StatusCode)
	}
}

func (e *jfTestEnv) get(t *testing.T, url string, dst any) {
	t.Helper()
	jfGetJSON(t, url, e.token, dst)
}

func TestJellyfinBrowseFilters(t *testing.T) {
	env := setupJellyfinBrowse(t)
	base := env.srv + "/Items?ParentId=" + jellyfinLibraryID(env.movieLib)
	type list struct {
		Items []struct {
			Name string `json:"Name"`
		} `json:"Items"`
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	names := func(l list) string {
		out := make([]string, 0, len(l.Items))
		for _, i := range l.Items {
			out = append(out, i.Name)
		}
		return strings.Join(out, ",")
	}

	var all list
	env.get(t, base+"&IncludeItemTypes=Movie", &all)
	if all.TotalRecordCount != 2 {
		t.Fatalf("all movies = %+v", all)
	}

	cases := []struct {
		query string
		want  string
	}{
		{"&Genres=Drama", "Drama Movie"},
		{"&Genres=Drama|Comedy", "Drama Movie,Comedy Movie"},
		{"&NameStartsWith=Drama", "Drama Movie"},
		{"&NameLessThan=Drama", "Comedy Movie"},
		{"&Years=2001", "Drama Movie"},
		{"&OfficialRatings=R", "Drama Movie"},
		{"&Filters=IsFavorite", "Drama Movie"},
		{"&Filters=IsResumable", "Drama Movie"},
		{"&Filters=IsPlayed", "Comedy Movie"},
	}
	for _, tc := range cases {
		var got list
		env.get(t, base+tc.query, &got)
		if names(got) != tc.want {
			t.Errorf("%s -> %q, want %q", tc.query, names(got), tc.want)
		}
	}

	var mixed list
	env.get(t, env.srv+"/Items?IncludeItemTypes=Movie,Series&ExcludeItemTypes=Movie", &mixed)
	if names(mixed) != "Some Show" {
		t.Fatalf("exclude filter = %s", names(mixed))
	}

	// Genre ids from /Genres round-trip into the filter.
	var genres struct {
		Items []struct {
			Name string `json:"Name"`
			ID   string `json:"Id"`
			Type string `json:"Type"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Genres?ParentId="+jellyfinLibraryID(env.movieLib), &genres)
	var dramaGenreID string
	for _, g := range genres.Items {
		if g.Name == "Drama" {
			dramaGenreID = g.ID
			if g.Type != "Genre" {
				t.Fatalf("genre type = %q", g.Type)
			}
		}
	}
	if dramaGenreID == "" {
		t.Fatalf("genres = %+v", genres)
	}
	var byGenreID list
	env.get(t, base+"&GenreIds="+dramaGenreID, &byGenreID)
	if names(byGenreID) != "Drama Movie" {
		t.Fatalf("genre id filter = %s", names(byGenreID))
	}

	// Person ids from /Persons round-trip into the filter.
	var persons struct {
		Items []struct {
			Name string `json:"Name"`
			ID   string `json:"Id"`
			Type string `json:"Type"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Persons", &persons)
	var actorID string
	for _, p := range persons.Items {
		if p.Name == "NFO Actor" {
			actorID = p.ID
			if p.Type != "Person" {
				t.Fatalf("person type = %q", p.Type)
			}
		}
	}
	if actorID == "" {
		t.Fatalf("persons = %+v", persons)
	}
	var byActor list
	env.get(t, base+"&PersonIds="+actorID, &byActor)
	if names(byActor) != "Drama Movie" {
		t.Fatalf("person filter = %s", names(byActor))
	}
}

func TestJellyfinMoonfinPaths(t *testing.T) {
	env := setupJellyfinBrowse(t)
	drama := jellyfinItemID(t, env.dramaID)
	episode := jellyfinItemID(t, env.ep1)

	// Moonfin reads detail through /Users/{userId}/Items/{itemId}.
	var detail struct {
		ID           string `json:"Id"`
		Type         string `json:"Type"`
		MediaSources []struct {
			ID string `json:"Id"`
		} `json:"MediaSources"`
	}
	env.get(t, env.srv+"/Users/"+env.userID+"/Items/"+drama, &detail)
	if detail.ID != drama || detail.Type != "Movie" || len(detail.MediaSources) == 0 {
		t.Fatalf("user-scoped detail = %+v", detail)
	}

	// Ancestors are root-first: series then season for an episode.
	var ancestors []struct {
		ID   string `json:"Id"`
		Type string `json:"Type"`
	}
	env.get(t, env.srv+"/Items/"+episode+"/Ancestors", &ancestors)
	if len(ancestors) != 2 || ancestors[0].Type != "Series" || ancestors[1].Type != "Season" {
		t.Fatalf("ancestors = %+v", ancestors)
	}

	// User-scoped trailer/intro paths and the rating endpoints.
	for _, tc := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/Users/" + env.userID + "/Items/" + drama + "/LocalTrailers", http.StatusOK},
		{http.MethodGet, "/Users/" + env.userID + "/Items/" + drama + "/Intros", http.StatusOK},
		{http.MethodPost, "/UserItems/" + drama + "/Rating?Likes=true", http.StatusNoContent},
		{http.MethodDelete, "/UserItems/" + drama + "/Rating", http.StatusNoContent},
		{http.MethodPost, "/Users/Configuration", http.StatusNoContent},
		{http.MethodGet, "/System/Ping", http.StatusOK},
		{http.MethodGet, "/Artists?Limit=5", http.StatusOK},
	} {
		var body []byte
		if tc.method == http.MethodPost && strings.HasSuffix(tc.path, "/Configuration") {
			body = []byte(`{"GroupedFolders":[]}`)
		}
		res := jfDo(t, tc.method, env.srv+tc.path, env.token, body)
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, res.StatusCode, tc.want)
		}
	}

	// Quick Connect is advertised as disabled with Jellyfin's 401.
	res := jfDo(t, http.MethodPost, env.srv+"/QuickConnect/Initiate", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("QuickConnect initiate = %d", res.StatusCode)
	}

	// Logout revokes the token. The test admin login installed a cookie jar on
	// http.DefaultClient; clear it so the cookie does not mask the revoked token.
	jar := http.DefaultClient.Jar
	http.DefaultClient.Jar = nil
	defer func() { http.DefaultClient.Jar = jar }()

	res = jfDo(t, http.MethodPost, env.srv+"/Sessions/Logout", env.token, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", res.StatusCode)
	}
	res = jfDo(t, http.MethodGet, env.srv+"/Users/Me", env.token, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token after logout = %d", res.StatusCode)
	}
}

func TestJellyfinHomeRows(t *testing.T) {
	env := setupJellyfinBrowse(t)

	var resume struct {
		Items []struct {
			Name string `json:"Name"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Items/Resume", &resume)
	found := false
	for _, i := range resume.Items {
		if i.Name == "Drama Movie" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resume = %+v", resume.Items)
	}

	// Latest for a TV library returns episodes (array, not a QueryResult).
	var latest []struct {
		Name string `json:"Name"`
		Type string `json:"Type"`
	}
	env.get(t, env.srv+"/Items/Latest?ParentId="+jellyfinLibraryID(env.tvLib), &latest)
	if len(latest) == 0 {
		t.Fatal("latest returned no items")
	}
	for _, i := range latest {
		if i.Type != "Episode" {
			t.Fatalf("latest item type = %s (%s)", i.Type, i.Name)
		}
	}

	// Global /Shows/NextUp (home row) returns the in-progress episode.
	var homeNext struct {
		Items []struct {
			IndexNumber *int `json:"IndexNumber"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Shows/NextUp?limit=10&enableResumable=false", &homeNext)
	if len(homeNext.Items) != 1 || homeNext.Items[0].IndexNumber == nil || *homeNext.Items[0].IndexNumber != 2 {
		t.Fatalf("home next up = %+v", homeNext.Items)
	}

	// NextUp resumes the in-progress episode, not the watched one.
	var next struct {
		Items []struct {
			IndexNumber       *int `json:"IndexNumber"`
			ParentIndexNumber *int `json:"ParentIndexNumber"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Shows/"+jellyfinItemID(t, env.showID)+"/NextUp", &next)
	if len(next.Items) != 1 || next.Items[0].IndexNumber == nil || *next.Items[0].IndexNumber != 2 {
		t.Fatalf("next up = %+v", next.Items)
	}
}

func TestJellyfinFacetsAndStubs(t *testing.T) {
	env := setupJellyfinBrowse(t)
	lib := jellyfinLibraryID(env.movieLib)

	var filters struct {
		Genres []struct {
			Name string `json:"Name"`
			ID   string `json:"Id"`
		} `json:"Genres"`
		OfficialRatings []string `json:"OfficialRatings"`
		Years           []int    `json:"Years"`
	}
	env.get(t, env.srv+"/Items/Filters2?ParentId="+lib, &filters)
	if len(filters.Genres) != 2 || len(filters.Years) != 2 || len(filters.OfficialRatings) != 2 {
		t.Fatalf("filters2 = %+v", filters)
	}

	var legacy struct {
		Genres []string `json:"Genres"`
		Years  []int    `json:"Years"`
	}
	env.get(t, env.srv+"/Items/Filters?ParentId="+lib, &legacy)
	if len(legacy.Genres) != 2 || len(legacy.Years) != 2 {
		t.Fatalf("legacy filters = %+v", legacy)
	}

	drama := jellyfinItemID(t, env.dramaID)
	var similar struct {
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	env.get(t, env.srv+"/Items/"+drama+"/Similar", &similar)
	if similar.TotalRecordCount != 0 {
		t.Fatalf("similar = %+v", similar)
	}

	var theme map[string]json.RawMessage
	env.get(t, env.srv+"/Items/"+drama+"/ThemeMedia", &theme)
	if len(theme) != 3 {
		t.Fatalf("theme media = %+v", theme)
	}

	var images []struct {
		ImageType string `json:"ImageType"`
	}
	env.get(t, env.srv+"/Items/"+jellyfinItemID(t, env.showID)+"/Images", &images)
	if len(images) == 0 || images[0].ImageType != "Primary" {
		t.Fatalf("images = %+v", images)
	}

	var trailers []struct {
		Name string `json:"Name"`
	}
	env.get(t, env.srv+"/Items/"+drama+"/LocalTrailers", &trailers)
	if len(trailers) != 1 || trailers[0].Name != "Trailer" {
		t.Fatalf("local trailers = %+v", trailers)
	}
	var specials []struct {
		Name string `json:"Name"`
	}
	env.get(t, env.srv+"/Items/"+drama+"/SpecialFeatures", &specials)
	if len(specials) != 1 {
		t.Fatalf("special features = %+v", specials)
	}

	// Seasons list for the series detail page.
	var seasons struct {
		Items []struct {
			IndexNumber *int   `json:"IndexNumber"`
			Type        string `json:"Type"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Shows/"+jellyfinItemID(t, env.showID)+"/Seasons", &seasons)
	if len(seasons.Items) != 2 || *seasons.Items[0].IndexNumber != 1 || *seasons.Items[1].IndexNumber != 2 {
		t.Fatalf("seasons = %+v", seasons.Items)
	}

	// Empty relation endpoints the TV client probes.
	for _, path := range []string{
		"/Shows/Upcoming",
		"/Videos/" + drama + "/AdditionalParts",
		"/Items/" + drama + "/Intros",
	} {
		var empty struct {
			Items            []json.RawMessage `json:"Items"`
			TotalRecordCount int               `json:"TotalRecordCount"`
		}
		env.get(t, env.srv+path, &empty)
		if len(empty.Items) != 0 || empty.TotalRecordCount != 0 {
			t.Fatalf("%s = %+v", path, empty)
		}
	}

	// Display preferences are accepted and ignored.
	pres := jfDo(t, http.MethodPost, env.srv+"/DisplayPreferences/usersettings?userId="+env.userID+"&client=test", env.token,
		[]byte(`{"SortBy":"SortName"}`))
	pres.Body.Close()
	if pres.StatusCode != http.StatusNoContent {
		t.Fatalf("save display preferences = %d", pres.StatusCode)
	}

	// /UserItems/Resume is the SDK's other resume path.
	var userResume struct {
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	env.get(t, env.srv+"/UserItems/Resume", &userResume)
	if userResume.TotalRecordCount == 0 {
		t.Fatal("user resume row is empty")
	}

	// Subtitle side-load URLs carry the token (ExoPlayer fetches them itself).
	var info struct {
		MediaSources []struct {
			MediaStreams []struct {
				Type        string `json:"Type"`
				DeliveryURL string `json:"DeliveryUrl"`
			} `json:"MediaStreams"`
		} `json:"MediaSources"`
	}
	env.get(t, env.srv+"/Items/"+drama+"/PlaybackInfo", &info)
	foundSub := false
	for _, src := range info.MediaSources {
		for _, st := range src.MediaStreams {
			if st.Type == "Subtitle" && strings.Contains(st.DeliveryURL, "api_key=") {
				foundSub = true
			}
		}
	}
	if !foundSub {
		t.Fatalf("subtitle delivery URL missing api_key: %+v", info.MediaSources)
	}

	// Episode list rows carry the series context and artwork fallback.
	var episodes struct {
		Items []struct {
			SeriesID                string   `json:"SeriesId"`
			SeriesName              string   `json:"SeriesName"`
			SeriesPrimaryImageTag   string   `json:"SeriesPrimaryImageTag"`
			ParentBackdropImageTags []string `json:"ParentBackdropImageTags"`
		} `json:"Items"`
	}
	env.get(t, env.srv+"/Items?ParentId="+jellyfinItemID(t, env.showID)+"&IncludeItemTypes=Episode&Recursive=true", &episodes)
	if len(episodes.Items) != 3 {
		t.Fatalf("episodes = %+v", episodes.Items)
	}
	for _, e := range episodes.Items {
		if e.SeriesID != jellyfinItemID(t, env.showID) || e.SeriesName != "Some Show" {
			t.Fatalf("episode series context = %+v", e)
		}
		if e.SeriesPrimaryImageTag == "" || len(e.ParentBackdropImageTags) == 0 {
			t.Fatalf("episode image fallback missing = %+v", e)
		}
	}
}

// jellyfinLibraryID encodes a library id the way the adapter does.
func jellyfinLibraryID(id int64) string { return jellyfin.LibraryID(id) }

var _ = bytes.NewReader

func TestJellyfinUserDataAndAnonymousMedia(t *testing.T) {
	env := setupJellyfinBrowse(t)
	drama := jellyfinItemID(t, env.dramaID)

	// GET /UserItems/{id}/UserData reflects position + favorite state.
	var userData struct {
		Played                bool  `json:"Played"`
		PlaybackPositionTicks int64 `json:"PlaybackPositionTicks"`
		IsFavorite            bool  `json:"IsFavorite"`
	}
	env.get(t, env.srv+"/UserItems/"+drama+"/UserData", &userData)
	if userData.PlaybackPositionTicks != 5000*10000 || !userData.IsFavorite || userData.Played {
		t.Fatalf("user data = %+v", userData)
	}

	// POST /UserItems/{id}/UserData marks it played.
	body := []byte(`{"Played":true}`)
	res := jfDo(t, http.MethodPost, env.srv+"/UserItems/"+drama+"/UserData?api_key="+env.token, env.token, body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update user data = %d", res.StatusCode)
	}
	env.get(t, env.srv+"/UserItems/"+drama+"/UserData", &userData)
	if !userData.Played {
		t.Fatalf("user data after update = %+v", userData)
	}

	// /UserPlayedItems/{id} (SDK path) toggles watched state.
	comedy := jellyfinItemID(t, env.comedyID)
	unmark := jfDo(t, http.MethodDelete, env.srv+"/UserPlayedItems/"+comedy+"?api_key="+env.token, env.token, nil)
	unmark.Body.Close()
	if unmark.StatusCode != http.StatusNoContent {
		t.Fatalf("unmark played = %d", unmark.StatusCode)
	}
	mark := jfDo(t, http.MethodPost, env.srv+"/UserPlayedItems/"+comedy+"?api_key="+env.token, env.token, nil)
	mark.Body.Close()
	if mark.StatusCode != http.StatusNoContent {
		t.Fatalf("mark played = %d", mark.StatusCode)
	}

	// Media segments are empty (no intro/outro detection).
	var segments struct {
		Items            []json.RawMessage `json:"Items"`
		TotalRecordCount int               `json:"TotalRecordCount"`
	}
	env.get(t, env.srv+"/MediaSegments/"+drama, &segments)
	if len(segments.Items) != 0 || segments.TotalRecordCount != 0 {
		t.Fatalf("media segments = %+v", segments)
	}

	// Device options are accepted.
	dev := jfDo(t, http.MethodPost, env.srv+"/Devices/Options?api_key="+env.token, env.token, []byte(`{"CustomName":"Findroid"}`))
	dev.Body.Close()
	if dev.StatusCode != http.StatusNoContent {
		t.Fatalf("device options = %d", dev.StatusCode)
	}

	// Artwork and static streams are anonymous (clients fetch them without
	// headers); opaque ids make them non-enumerable.
	bare := &http.Client{}
	imgURL := env.srv + "/Items/" + jellyfinItemID(t, env.showID) + "/Images/Primary"
	imgRes, err := bare.Get(imgURL)
	if err != nil {
		t.Fatal(err)
	}
	imgRes.Body.Close()
	if imgRes.StatusCode != http.StatusOK {
		t.Fatalf("anonymous image = %d", imgRes.StatusCode)
	}
	streamURL := env.srv + "/Videos/" + drama + "/stream?static=true"
	streamRes, err := bare.Get(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	streamRes.Body.Close()
	if streamRes.StatusCode != http.StatusOK {
		t.Fatalf("anonymous stream = %d", streamRes.StatusCode)
	}
}
