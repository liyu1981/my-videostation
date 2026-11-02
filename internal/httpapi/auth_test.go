package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
)

func jsonDo(t *testing.T, c *http.Client, method, url string, body any, out any) int {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, url, err)
		}
	} else {
		_, _ = io.Copy(io.Discard, res.Body)
	}
	return res.StatusCode
}

func TestAccountsAndPermissions(t *testing.T) {
	srv, st, dir := newRawTestServer(t)
	anon := &http.Client{}
	adminJar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: adminJar}
	userJar, _ := cookiejar.New(nil)
	user := &http.Client{Jar: userJar}

	var status struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	if code := jsonDo(t, anon, http.MethodGet, srv.URL+"/api/v1/auth/status", nil, &status); code != 200 || !status.NeedsSetup {
		t.Fatalf("initial status = %d %+v", code, status)
	}
	if code := jsonDo(t, anon, http.MethodGet, srv.URL+"/api/v1/home", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated home = %d, want 401", code)
	}
	if code := jsonDo(t, anon, http.MethodPost, srv.URL+"/api/v1/auth/setup", map[string]string{
		"username": "admin", "password": "short",
	}, nil); code != http.StatusBadRequest {
		t.Fatalf("short setup password = %d, want 400", code)
	}

	var setup struct {
		User store.User `json:"user"`
	}
	if code := jsonDo(t, admin, http.MethodPost, srv.URL+"/api/v1/auth/setup", map[string]string{
		"username": "admin", "displayName": "Admin", "password": "correct-horse-123",
	}, &setup); code != http.StatusOK {
		t.Fatalf("setup = %d", code)
	}
	if setup.User.Role != store.RoleAdmin || setup.User.PasswordHash != "" {
		t.Fatalf("setup user = %+v", setup.User)
	}
	if code := jsonDo(t, admin, http.MethodGet, srv.URL+"/api/v1/auth/me", nil, nil); code != http.StatusOK {
		t.Fatalf("admin me = %d", code)
	}
	if code := jsonDo(t, anon, http.MethodPost, srv.URL+"/api/v1/auth/setup", map[string]string{
		"username": "other", "password": "correct-horse-123",
	}, nil); code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", code)
	}

	var created store.User
	if code := jsonDo(t, admin, http.MethodPost, srv.URL+"/api/v1/users", map[string]string{
		"username": "viewer", "displayName": "Viewer", "password": "viewer-pass-123", "role": "user",
	}, &created); code != http.StatusCreated {
		t.Fatalf("create user = %d", code)
	}
	if created.Role != store.RoleUser {
		t.Fatalf("created role = %q", created.Role)
	}

	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "viewer", "password": "wrong-password",
	}, nil); code != http.StatusUnauthorized {
		t.Fatalf("wrong login = %d, want 401", code)
	}
	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "viewer", "password": "viewer-pass-123",
	}, nil); code != http.StatusOK {
		t.Fatalf("viewer login = %d", code)
	}

	if code := jsonDo(t, user, http.MethodGet, srv.URL+"/api/v1/users", nil, nil); code != http.StatusForbidden {
		t.Fatalf("viewer list users = %d, want 403", code)
	}
	if code := jsonDo(t, user, http.MethodGet, srv.URL+"/api/v1/libraries", nil, nil); code != http.StatusOK {
		t.Fatalf("viewer list libraries = %d, want 200", code)
	}
	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/libraries", map[string]any{
		"name": "Nope", "type": "movie", "paths": []string{dir},
	}, nil); code != http.StatusForbidden {
		t.Fatalf("viewer create library = %d, want 403", code)
	}
	if code := jsonDo(t, user, http.MethodGet, srv.URL+"/api/v1/settings", nil, nil); code != http.StatusForbidden {
		t.Fatalf("viewer settings = %d, want 403", code)
	}

	// Last-admin and self-protection rules.
	if code := jsonDo(t, admin, http.MethodPatch, srv.URL+"/api/v1/users/1", map[string]any{"role": "user"}, nil); code != http.StatusBadRequest {
		t.Fatalf("demote last admin = %d, want 400", code)
	}
	if code := jsonDo(t, admin, http.MethodDelete, srv.URL+"/api/v1/users/1", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("delete self = %d, want 400", code)
	}

	// Collections are per user.
	var adminCols, userCols struct {
		Collections []store.Collection `json:"collections"`
	}
	if code := jsonDo(t, admin, http.MethodPost, srv.URL+"/api/v1/collections", map[string]string{"name": "Admin list"}, nil); code != http.StatusCreated {
		t.Fatalf("admin collection = %d", code)
	}
	if code := jsonDo(t, user, http.MethodGet, srv.URL+"/api/v1/collections", nil, &userCols); code != http.StatusOK {
		t.Fatalf("viewer collections = %d", code)
	}
	for _, c := range userCols.Collections {
		if c.Name == "Admin list" {
			t.Fatalf("viewer sees admin collection: %+v", userCols.Collections)
		}
	}
	if code := jsonDo(t, admin, http.MethodGet, srv.URL+"/api/v1/collections", nil, &adminCols); code != http.StatusOK {
		t.Fatalf("admin collections = %d", code)
	}
	found := false
	for _, c := range adminCols.Collections {
		if c.Name == "Admin list" {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin collection missing: %+v", adminCols.Collections)
	}
	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/collections", map[string]string{"name": "Viewer list"}, nil); code != http.StatusCreated {
		t.Fatalf("viewer collection = %d", code)
	}
	if code := jsonDo(t, admin, http.MethodGet, srv.URL+"/api/v1/collections", nil, &adminCols); code != http.StatusOK {
		t.Fatalf("admin collections after viewer = %d", code)
	}
	for _, c := range adminCols.Collections {
		if c.Name == "Viewer list" {
			t.Fatalf("admin sees viewer collection: %+v", adminCols.Collections)
		}
	}

	// Playback progress is per user.
	mediaDir := filepath.Join(dir, "media")
	writeFile(t, filepath.Join(mediaDir, "Test Movie (2020)", "Test.Movie.2020.mkv"))
	lib, err := st.CreateLibrary(context.Background(), "Movies", "movie", "", []string{mediaDir})
	if err != nil {
		t.Fatal(err)
	}
	sc := scanner.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := sc.ScanLibrary(context.Background(), lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListItems(context.Background(), store.ItemFilter{LibraryID: lib.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %+v err=%v", items, err)
	}
	itemURL := srv.URL + "/api/v1/items/" + itoa(items[0].ID)
	if code := jsonDo(t, admin, http.MethodPost, itemURL+"/progress", map[string]any{
		"positionMs": 5000, "durationMs": 10000, "played": false,
	}, nil); code != http.StatusOK {
		t.Fatalf("admin progress = %d", code)
	}
	if code := jsonDo(t, user, http.MethodPost, itemURL+"/progress", map[string]any{
		"positionMs": 7000, "durationMs": 10000, "played": false,
	}, nil); code != http.StatusOK {
		t.Fatalf("viewer progress = %d", code)
	}
	var adminDetail, userDetail struct {
		PositionMS int64 `json:"positionMs"`
	}
	if code := jsonDo(t, admin, http.MethodGet, itemURL, nil, &adminDetail); code != 200 || adminDetail.PositionMS != 5000 {
		t.Fatalf("admin position = %d %+v", code, adminDetail)
	}
	if code := jsonDo(t, user, http.MethodGet, itemURL, nil, &userDetail); code != 200 || userDetail.PositionMS != 7000 {
		t.Fatalf("viewer position = %d %+v", code, userDetail)
	}

	// Self-service password change keeps the current session and invalidates old credentials.
	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/auth/password", map[string]string{
		"currentPassword": "wrong", "newPassword": "viewer-pass-456",
	}, nil); code != http.StatusBadRequest {
		t.Fatalf("wrong current password = %d, want 400", code)
	}
	if code := jsonDo(t, user, http.MethodPost, srv.URL+"/api/v1/auth/password", map[string]string{
		"currentPassword": "viewer-pass-123", "newPassword": "viewer-pass-456",
	}, nil); code != http.StatusNoContent {
		t.Fatalf("change password = %d, want 204", code)
	}
	if code := jsonDo(t, anon, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "viewer", "password": "viewer-pass-123",
	}, nil); code != http.StatusUnauthorized {
		t.Fatalf("old password login = %d, want 401", code)
	}
	if code := jsonDo(t, anon, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "viewer", "password": "viewer-pass-456",
	}, nil); code != http.StatusOK {
		t.Fatalf("new password login = %d, want 200", code)
	}

	// Logout invalidates the session.
	if code := jsonDo(t, admin, http.MethodPost, srv.URL+"/api/v1/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout = %d", code)
	}
	if code := jsonDo(t, admin, http.MethodGet, srv.URL+"/api/v1/auth/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", code)
	}
}
