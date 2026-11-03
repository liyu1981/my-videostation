package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExportAndApply(t *testing.T) {
	data := t.TempDir()
	if err := os.MkdirAll(filepath.Join(data, "cache", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "cache", "images", "poster.jpg"), []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	arch, err := Export(context.Background(), data, "test", func(_ context.Context, dst string) error {
		return os.WriteFile(dst, []byte("database"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if arch.Size == 0 {
		t.Fatal("empty archive")
	}
	if err := StageImport(context.Background(), data, arch.Name); err == nil {
		// The snapshot is intentionally not a real SQLite DB; validation must fail.
		t.Fatal("invalid database accepted")
	}
	_ = os.RemoveAll(filepath.Join(data, stagingDir))
}

func TestExtractRejectsTraversal(t *testing.T) {
	src := filepath.Join(t.TempDir(), "bad.tar.gz")
	f, _ := os.Create(src)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	if err := extractTarGz(src, t.TempDir()); err == nil {
		t.Fatal("path traversal accepted")
	}
}
