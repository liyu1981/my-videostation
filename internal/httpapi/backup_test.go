package httpapi_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupExportAPI(t *testing.T) {
	srv, _, dir := newTestServer(t)

	var archive struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodPost, srv.URL+"/api/v1/backup/export", nil, &archive); code != http.StatusOK {
		t.Fatalf("export = %d", code)
	}
	if archive.Name == "" || archive.Size == 0 {
		t.Fatalf("archive = %+v", archive)
	}
	if _, err := os.Stat(filepath.Join(dir, "export", archive.Name)); err != nil {
		t.Fatalf("export file: %v", err)
	}

	var exports struct {
		Exports []struct {
			Name string `json:"name"`
		} `json:"exports"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodGet, srv.URL+"/api/v1/backup/exports", nil, &exports); code != http.StatusOK || len(exports.Exports) != 1 {
		t.Fatalf("exports = %d %+v", code, exports)
	}

	if code := jsonDo(t, http.DefaultClient, http.MethodPost, srv.URL+"/api/v1/backup/import", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("empty import = %d, want 400", code)
	}
}

func TestBackupImportEntersMaintenance(t *testing.T) {
	srv, _, dir := newTestServer(t)
	var archive struct {
		Name string `json:"name"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodPost, srv.URL+"/api/v1/backup/export", nil, &archive); code != http.StatusOK {
		t.Fatalf("export = %d", code)
	}
	importDir := filepath.Join(dir, "import")
	if err := os.MkdirAll(importDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "export", archive.Name)
	dst := filepath.Join(importDir, archive.Name)
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatal(err)
	}

	var resp struct {
		Status string `json:"status"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodPost, srv.URL+"/api/v1/backup/import", map[string]string{
		"name": archive.Name,
	}, &resp); code != http.StatusAccepted || resp.Status != "restarting" {
		t.Fatalf("import = %d %+v", code, resp)
	}
	var status struct {
		Maintenance   bool `json:"maintenance"`
		PendingImport bool `json:"pendingImport"`
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodGet, srv.URL+"/api/v1/backup/status", nil, &status); code != http.StatusOK || !status.Maintenance || !status.PendingImport {
		t.Fatalf("status = %d %+v", code, status)
	}
	if code := jsonDo(t, http.DefaultClient, http.MethodGet, srv.URL+"/api/v1/libraries", nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("API during maintenance = %d, want 503", code)
	}
}
