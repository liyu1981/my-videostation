package store

import (
	"context"

	"github.com/liyu1981/myvideostation/internal/textutil"
)

// NormalizeOverviews rewrites provider/NFO overviews that use ideographic spaces
// or HTML breaks as paragraph separators into real blank lines. Runs at startup so
// existing rows are fixed without a refresh.
func (s *Store) NormalizeOverviews(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(overview,'') FROM media_items WHERE COALESCE(overview,'') <> ''`)
	if err != nil {
		return 0, wrap("normalize overviews", err)
	}
	type update struct {
		id       int64
		overview string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var overview string
		if err := rows.Scan(&id, &overview); err != nil {
			rows.Close()
			return 0, wrap("normalize overview scan", err)
		}
		if next := textutil.NormalizeOverview(overview); next != overview {
			updates = append(updates, update{id: id, overview: next})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, wrap("normalize overview iterate", err)
	}
	if len(updates) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, wrap("begin normalize overviews", err)
	}
	defer tx.Rollback()
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx,
			`UPDATE media_items SET overview = ?, updated_at = ? WHERE id = ?`, u.overview, now(), u.id); err != nil {
			return 0, wrap("normalize overview update", err)
		}
		if err := refreshFTS(ctx, tx, u.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, wrap("commit normalize overviews", err)
	}
	return len(updates), nil
}
