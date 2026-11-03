// Package backup creates and applies portable MyVideoStation data bundles.
//
// A bundle contains a consistent SQLite snapshot (mvs.db), the artwork/subtitle
// cache and a manifest. It deliberately does not contain media files.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/db"
)

const (
	FormatName    = "myvideostation-export"
	FormatVersion = 1
	pendingName   = "pending.json"
	stagingDir    = ".import-staging"
)

// Archive is an export or import bundle.
type Archive struct {
	Name      string    `json:"name"`
	Path      string    `json:"path,omitempty"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

// Manifest is stored inside a bundle.
type Manifest struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	AppVersion string    `json:"appVersion"`
	CreatedAt  time.Time `json:"createdAt"`
}

type pendingImport struct {
	Archive   string    `json:"archive"`
	CreatedAt time.Time `json:"createdAt"`
}

// Snapshot writes a consistent SQLite copy of the live database to dst.
type Snapshot func(ctx context.Context, dst string) error

// Export writes a timestamped .tar.gz bundle into <data>/export.
func Export(ctx context.Context, dataDir, appVersion string, snapshot Snapshot) (*Archive, error) {
	exportDir := filepath.Join(dataDir, "export")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}
	now := time.Now().UTC()
	name := "mvs-export-" + now.Format("20060102-150405") + ".tar.gz"
	if _, err := os.Stat(filepath.Join(exportDir, name)); err == nil {
		name = "mvs-export-" + now.Format("20060102-150405.000000000") + ".tar.gz"
	}
	final := filepath.Join(exportDir, name)
	tmp := final + ".tmp"
	_ = os.Remove(final)
	_ = os.Remove(tmp)

	snapshotPath := filepath.Join(exportDir, "."+name+".db")
	_ = os.Remove(snapshotPath)
	if err := snapshot(ctx, snapshotPath); err != nil {
		return nil, err
	}
	defer os.Remove(snapshotPath)

	f, err := os.Create(tmp)
	if err != nil {
		return nil, fmt.Errorf("create export: %w", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	writeErr := func() error {
		if err := writeJSON(tw, "manifest.json", Manifest{
			Format: FormatName, Version: FormatVersion, AppVersion: appVersion, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := writeFile(tw, "mvs.db", snapshotPath); err != nil {
			return err
		}
		cacheDir := filepath.Join(dataDir, "cache")
		if st, err := os.Stat(cacheDir); err == nil && st.IsDir() {
			if err := writeTree(tw, cacheDir, "cache"); err != nil {
				return err
			}
		}
		return nil
	}()
	if writeErr == nil {
		writeErr = tw.Close()
	} else {
		_ = tw.Close()
	}
	if err := gz.Close(); err != nil && writeErr == nil {
		writeErr = err
	}
	if err := f.Close(); err != nil && writeErr == nil {
		writeErr = err
	}
	if writeErr != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("write export: %w", writeErr)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("finalize export: %w", err)
	}
	st, err := os.Stat(final)
	if err != nil {
		return nil, err
	}
	return &Archive{Name: name, Path: final, Size: st.Size(), CreatedAt: now}, nil
}

// ListExports lists bundles in <data>/export, newest first.
func ListExports(dataDir string) ([]Archive, error) {
	return listArchives(filepath.Join(dataDir, "export"))
}

// ListImports lists bundles in <data>/import, newest first.
func ListImports(dataDir string) ([]Archive, error) {
	return listArchives(filepath.Join(dataDir, "import"))
}

func listArchives(dir string) ([]Archive, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Archive{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Archive{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, Archive{Name: entry.Name(), Path: filepath.Join(dir, entry.Name()), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Name > out[j].Name
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// StageImport extracts and validates a bundle into <data>/.import-staging.
func StageImport(ctx context.Context, dataDir, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".tar.gz") {
		return errors.New("invalid import bundle name")
	}
	archive := filepath.Join(dataDir, "import", name)
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("import bundle: %w", err)
	}
	staging := filepath.Join(dataDir, stagingDir)
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(archive, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	dbPath := filepath.Join(staging, "mvs.db")
	info, err := os.Stat(dbPath)
	if err != nil || info.Size() == 0 {
		_ = os.RemoveAll(staging)
		return errors.New("import bundle has no valid mvs.db")
	}
	// Verify that the snapshot opens and passes SQLite's integrity check.
	d, err := db.Open(ctx, dbPath)
	if err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("open imported database: %w", err)
	}
	var check string
	err = d.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&check)
	_ = d.Close()
	if err != nil || !strings.EqualFold(check, "ok") {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("imported database failed integrity check: %s", check)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil {
		_ = os.RemoveAll(staging)
		return errors.New("import bundle has no manifest.json")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || manifest.Format != FormatName {
		_ = os.RemoveAll(staging)
		return errors.New("import bundle has an invalid manifest")
	}
	if manifest.Version <= 0 || manifest.Version > FormatVersion {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("unsupported import format version %d", manifest.Version)
	}
	return nil
}

// MarkPending records the staged bundle for application on the next startup.
func MarkPending(dataDir, archiveName string) error {
	importDir := filepath.Join(dataDir, "import")
	if err := os.MkdirAll(importDir, 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(pendingImport{Archive: archiveName, CreatedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(importDir, pendingName), body, 0o644)
}

// Pending reports whether an import is staged for the next startup.
func Pending(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, "import", pendingName))
	return err == nil
}

// ApplyPending swaps the staged database/cache into place. It must run before
// the database is opened. It is safe to call when no import is pending.
func ApplyPending(dataDir string) (bool, error) {
	marker := filepath.Join(dataDir, "import", pendingName)
	body, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var pending pendingImport
	if err := json.Unmarshal(body, &pending); err != nil {
		return false, fmt.Errorf("parse pending import: %w", err)
	}
	staging := filepath.Join(dataDir, stagingDir)
	stagedDB := filepath.Join(staging, "mvs.db")
	if _, err := os.Stat(stagedDB); err != nil {
		return false, fmt.Errorf("staged database missing: %w", err)
	}

	dbPath := filepath.Join(dataDir, "mvs.db")
	oldDB := dbPath + ".pre-import"
	_ = os.Remove(oldDB)
	hadDB := false
	if _, err := os.Stat(dbPath); err == nil {
		hadDB = true
		if err := os.Rename(dbPath, oldDB); err != nil {
			return false, fmt.Errorf("move current database: %w", err)
		}
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(stagedDB, dbPath); err != nil {
		if hadDB {
			_ = os.Rename(oldDB, dbPath)
		}
		return false, fmt.Errorf("install imported database: %w", err)
	}

	cacheDir := filepath.Join(dataDir, "cache")
	stagedCache := filepath.Join(staging, "cache")
	if st, err := os.Stat(stagedCache); err == nil && st.IsDir() {
		oldCache := cacheDir + ".pre-import"
		_ = os.RemoveAll(oldCache)
		if _, err := os.Stat(cacheDir); err == nil {
			if err := os.Rename(cacheDir, oldCache); err != nil {
				_ = os.Rename(oldDB, dbPath)
				return false, fmt.Errorf("move current cache: %w", err)
			}
		}
		if err := os.Rename(stagedCache, cacheDir); err != nil {
			if _, statErr := os.Stat(oldCache); statErr == nil {
				_ = os.Rename(oldCache, cacheDir)
			}
			_ = os.Rename(oldDB, dbPath)
			return false, fmt.Errorf("install imported cache: %w", err)
		}
		_ = os.RemoveAll(oldCache)
	}
	_ = os.RemoveAll(staging)
	_ = os.Remove(marker)
	_ = os.Remove(oldDB)
	return true, nil
}

func writeJSON(tw *tar.Writer, name string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), ModTime: time.Now().UTC()}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(body)
	return err
}

func writeFile(tw *tar.Writer, name, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

func writeTree(tw *tar.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if entry.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func extractTarGz(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
			continue
		}
		target := filepath.Join(dst, name)
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(out, tr, hdr.Size)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
