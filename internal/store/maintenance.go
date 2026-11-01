package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// RebaseCachePaths repairs cached artwork paths after the data directory moved.
// It only touches rows whose path looks like a cache image and whose file no
// longer exists at the stored location, re-pointing them at
// <cacheDir>/images/<basename> when that file is present. Media-folder artwork
// (NFO thumbnails) is left alone.
func (s *Store) RebaseCachePaths(ctx context.Context, cacheDir string) (int, error) {
	imgDir := filepath.Join(cacheDir, "images")
	if st, err := os.Stat(imgDir); err != nil || !st.IsDir() {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(poster_path,''), COALESCE(backdrop_path,'')
		FROM media_items
		WHERE COALESCE(poster_path,'') <> '' OR COALESCE(backdrop_path,'') <> ''`)
	if err != nil {
		return 0, wrap("rebase cache paths", err)
	}
	defer rows.Close()

	fix := func(p string) string {
		if p == "" || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			return p
		}
		if !strings.Contains(filepath.ToSlash(p), "cache/images/") {
			return p
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
		base := filepath.Base(p)
		if base == "" || base == "." || base == string(filepath.Separator) {
			return p
		}
		cand := filepath.Join(imgDir, base)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		return p
	}

	type update struct {
		id       int64
		poster   string
		backdrop string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var poster, backdrop string
		if err := rows.Scan(&id, &poster, &backdrop); err != nil {
			return 0, wrap("rebase scan", err)
		}
		np, nb := fix(poster), fix(backdrop)
		if np != poster || nb != backdrop {
			updates = append(updates, update{id: id, poster: np, backdrop: nb})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, wrap("rebase iterate", err)
	}
	if len(updates) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, wrap("begin rebase", err)
	}
	defer tx.Rollback()
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx, `
			UPDATE media_items SET poster_path = ?, backdrop_path = ?, updated_at = ?
			WHERE id = ?`, nullStr(u.poster), nullStr(u.backdrop), now(), u.id); err != nil {
			return 0, wrap("rebase update", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, wrap("commit rebase", err)
	}
	return len(updates), nil
}
