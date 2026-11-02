package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
)

func TestLibraryPermissionsAndRatingCeiling(t *testing.T) {
	srv, st, dir := newTestServer(t)
	ctx := context.Background()
	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Private Movie (2020)", "Private.Movie.2020.mkv"))
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

	var created store.User
	if code := jsonDo(t, http.DefaultClient, http.MethodPost, srv.URL+"/api/v1/users", map[string]string{
		"username": "viewer", "displayName": "Viewer", "password": "viewer-pass-123", "role": "user",
	}, &created); code != http.StatusCreated {
		t.Fatalf("create viewer = %d", code)
	}

	// Deny the only library.
	if code := jsonDo(t, http.DefaultClient, http.MethodPut, srv.URL+"/api/v1/users/"+itoa(created.ID)+"/libraries", map[string]any{
		"libraryIds": []int64{},
	}, nil); code != http.StatusOK {
		t.Fatalf("deny libraries = %d", code)
	}

	jar, _ := cookiejar.New(nil)
	viewer := &http.Client{Jar: jar}
	if code := jsonDo(t, viewer, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "viewer", "password": "viewer-pass-123",
	}, nil); code != http.StatusOK {
		t.Fatalf("viewer login = %d", code)
	}
	var libs struct {
		Libraries []store.Library `json:"libraries"`
	}
	if code := jsonDo(t, viewer, http.MethodGet, srv.URL+"/api/v1/libraries", nil, &libs); code != 200 || len(libs.Libraries) != 0 {
		t.Fatalf("viewer libraries = %d %+v", code, libs.Libraries)
	}
	var browse struct {
		Total int `json:"total"`
	}
	if code := jsonDo(t, viewer, http.MethodGet, srv.URL+"/api/v1/browse?kind=movie&libraryId="+itoa(lib.ID), nil, &browse); code != 200 || browse.Total != 0 {
		t.Fatalf("viewer browse = %d total=%d", code, browse.Total)
	}
	itemURL := srv.URL + "/api/v1/items/" + itoa(items[0].ID)
	if code := jsonDo(t, viewer, http.MethodGet, itemURL, nil, nil); code != http.StatusNotFound {
		t.Fatalf("viewer item = %d, want 404", code)
	}
	if code := jsonDo(t, viewer, http.MethodGet, srv.URL+"/api/v1/media/"+itoa(files[0].ID), nil, nil); code != http.StatusNotFound {
		t.Fatalf("viewer media = %d, want 404", code)
	}

	// Allow it again.
	if code := jsonDo(t, http.DefaultClient, http.MethodPut, srv.URL+"/api/v1/users/"+itoa(created.ID)+"/libraries", map[string]any{
		"libraryIds": []int64{lib.ID},
	}, nil); code != http.StatusOK {
		t.Fatalf("allow library = %d", code)
	}
	if code := jsonDo(t, viewer, http.MethodGet, itemURL, nil, nil); code != http.StatusOK {
		t.Fatalf("viewer item after allow = %d, want 200", code)
	}

	// Rating ceiling hides an R title from the user but not the admin.
	if _, err := st.DB().ExecContext(ctx, `UPDATE media_items SET content_rating = 'R' WHERE id = ?`, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodPatch, srv.URL+"/api/v1/users/"+itoa(created.ID), map[string]any{
		"ratingCeiling": "G",
	}, nil); code != http.StatusOK {
		t.Fatalf("set rating ceiling = %d", code)
	}
	if code := jsonDo(t, viewer, http.MethodGet, itemURL, nil, nil); code != http.StatusNotFound {
		t.Fatalf("viewer R item = %d, want 404", code)
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodGet, itemURL, nil, nil); code != http.StatusOK {
		t.Fatalf("admin R item = %d, want 200", code)
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodPatch, srv.URL+"/api/v1/users/"+itoa(created.ID), map[string]any{
		"ratingCeiling": "",
	}, nil); code != http.StatusOK {
		t.Fatalf("clear rating ceiling = %d", code)
	}

	// Admin audit log contains the account and login events.
	var audit struct {
		Events []store.AuditEvent `json:"events"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodGet, srv.URL+"/api/v1/audit", nil, &audit); code != 200 {
		t.Fatalf("audit = %d", code)
	}
	foundCreate, foundLogin := false, false
	for _, e := range audit.Events {
		if e.Action == "user.create" {
			foundCreate = true
		}
		if e.Action == "auth.login" && e.Username == "viewer" {
			foundLogin = true
		}
	}
	if !foundCreate || !foundLogin {
		t.Fatalf("audit events missing: %+v", audit.Events)
	}
}
