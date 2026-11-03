package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/backup"
	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/store"
)

func TestExportStageApplyWithRealDatabase(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	dbPath := filepath.Join(data, "mvs.db")
	d, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	if err := st.PutSettings(ctx, map[string]string{"test.key": "before"}); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(data, "cache", "images")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "poster.jpg"), []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}

	arch, err := backup.Export(ctx, data, "test", st.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	importDir := filepath.Join(data, "import")
	if err := os.MkdirAll(importDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(arch.Path, filepath.Join(importDir, arch.Name)); err != nil {
		t.Fatal(err)
	}

	// Change the live DB so the import must restore the exported value.
	d2, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	st2 := store.New(d2)
	if err := st2.PutSettings(ctx, map[string]string{"test.key": "after"}); err != nil {
		t.Fatal(err)
	}
	d2.Close()

	if err := backup.StageImport(ctx, data, arch.Name); err != nil {
		t.Fatal(err)
	}
	if err := backup.MarkPending(data, arch.Name); err != nil {
		t.Fatal(err)
	}
	applied, err := backup.ApplyPending(data)
	if err != nil || !applied {
		t.Fatalf("apply = %v, %v", applied, err)
	}

	d3, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d3.Close()
	st3 := store.New(d3)
	value, ok, err := st3.Setting(ctx, "test.key")
	if err != nil || !ok || value != "before" {
		t.Fatalf("setting = %q, %v, %v; want before", value, ok, err)
	}
	if body, err := os.ReadFile(filepath.Join(cache, "poster.jpg")); err != nil || string(body) != "poster" {
		t.Fatalf("cache = %q, %v", body, err)
	}
	if backup.Pending(data) {
		t.Fatal("pending marker was not cleared")
	}
}
