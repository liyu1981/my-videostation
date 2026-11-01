package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var rePartFile = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:cd|disc|disk|dvd|part)[\s._-]*\d{1,2}(?:[\s._-]|$)`)

// FindDuplicateLeaf returns another movie/home-video in the same library with the
// same normalised title and (compatible) year, excluding keep-separate items.
func (s *Store) FindDuplicateLeaf(ctx context.Context, id int64) (*Item, error) {
	var libraryID, year int64
	var kind, title string
	if err := s.db.QueryRowContext(ctx,
		`SELECT library_id, kind, title, COALESCE(year,0) FROM media_items WHERE id = ?`, id).
		Scan(&libraryID, &kind, &title, &year); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap("duplicate load", err)
	}
	if kind != KindMovie && kind != KindHomeVideo {
		return nil, nil
	}
	var dupID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM media_items
		WHERE library_id = ? AND kind = ? AND id != ? AND keep_separate = 0
		  AND lower(trim(title)) = lower(trim(?))
		  AND (COALESCE(year,0) = ? OR COALESCE(year,0) = 0 OR ? = 0)
		ORDER BY CASE metadata_status WHEN 'manual' THEN 0 WHEN 'matched' THEN 1 ELSE 2 END, id
		LIMIT 1`, libraryID, kind, id, title, year, year).Scan(&dupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("duplicate find", err)
	}
	return s.GetItem(ctx, dupID)
}

// KeepSeparate marks both items so scans will not auto-merge them.
func (s *Store) KeepSeparate(ctx context.Context, aID, bID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE media_items SET keep_separate = 1 WHERE id IN (?, ?)`, aID, bID)
	return wrap("keep separate", err)
}

// MergeItems moves fromID (its files, credits, locks, progress and collections)
// into intoID, keeping intoID's metadata and filling its empty fields from fromID.
func (s *Store) MergeItems(ctx context.Context, intoID, fromID int64) error {
	if intoID == fromID {
		return nil
	}
	into, err := s.GetItem(ctx, intoID)
	if err != nil {
		return err
	}
	from, err := s.GetItem(ctx, fromID)
	if err != nil {
		return err
	}
	if into.LibraryID != from.LibraryID || into.Kind != from.Kind {
		return wrap("merge", fmt.Errorf("items must share library and kind"))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin merge", err)
	}
	defer tx.Rollback()

	stmts := []string{
		`UPDATE media_files SET item_id = ? WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_genres (item_id, genre_id) SELECT ?, genre_id FROM item_genres WHERE item_id = ?`,
		`DELETE FROM item_genres WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_people (item_id, person_id, role, character, ordering) SELECT ?, person_id, role, character, ordering FROM item_people WHERE item_id = ?`,
		`DELETE FROM item_people WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_locks (item_id, field) SELECT ?, field FROM item_locks WHERE item_id = ?`,
		`DELETE FROM item_locks WHERE item_id = ?`,
		`INSERT OR IGNORE INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at) SELECT user_id, ?, position_ms, duration_ms, played, updated_at FROM playback_progress WHERE item_id = ?`,
		`DELETE FROM playback_progress WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_collections (collection_id, item_id) SELECT collection_id, ? FROM item_collections WHERE item_id = ?`,
		`DELETE FROM item_collections WHERE item_id = ?`,
	}
	// The first statement moves files; the following pairs copy then clear child rows.
	args := [][]any{
		{intoID, fromID},
		{intoID, fromID}, {fromID},
		{intoID, fromID}, {fromID},
		{intoID, fromID}, {fromID},
		{intoID, fromID}, {fromID},
		{intoID, fromID}, {fromID},
	}
	for i, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, args[i]...); err != nil {
			return wrap("merge rows", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_items SET parent_id = ? WHERE parent_id = ? AND kind = 'extra'`, intoID, fromID); err != nil {
		return wrap("merge extras", err)
	}

	// Fill missing metadata on the keeper and keep the stronger status/provider.
	status := bestStatus(into.MetadataStatus, from.MetadataStatus)
	takeFrom := statusScore(from.MetadataStatus) > statusScore(into.MetadataStatus)
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_items SET
			original_title = COALESCE(NULLIF(original_title,''), ?),
			overview = COALESCE(NULLIF(overview,''), ?),
			tagline = COALESCE(NULLIF(tagline,''), ?),
			content_rating = COALESCE(NULLIF(content_rating,''), ?),
			release_date = COALESCE(NULLIF(release_date,''), ?),
			poster_path = COALESCE(NULLIF(poster_path,''), ?),
			backdrop_path = COALESCE(NULLIF(backdrop_path,''), ?),
			rating = CASE WHEN COALESCE(rating,0) > 0 THEN rating ELSE ? END,
			runtime_ms = CASE WHEN COALESCE(runtime_ms,0) > 0 THEN runtime_ms ELSE ? END,
			year = CASE WHEN COALESCE(year,0) > 0 THEN year ELSE ? END,
			provider = CASE WHEN ? = 1 THEN ? ELSE COALESCE(NULLIF(provider,''), ?) END,
			provider_id = CASE WHEN ? = 1 THEN ? ELSE COALESCE(NULLIF(provider_id,''), ?) END,
			metadata_status = ?,
			match_confidence = MAX(match_confidence, ?),
			locked = MAX(locked, ?),
			keep_separate = 0,
			updated_at = ?
		WHERE id = ?`,
		from.OriginalTitle, from.Overview, from.Tagline, from.ContentRating, from.ReleaseDate,
		from.PosterPath, from.BackdropPath, from.Rating, from.RuntimeMS, from.Year,
		boolInt(takeFrom), from.Provider, from.Provider,
		boolInt(takeFrom), from.ProviderID, from.ProviderID,
		status, from.MatchConfidence, boolInt(from.Locked), now(), intoID); err != nil {
		return wrap("merge metadata", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM media_fts WHERE rowid = ?`, fromID); err != nil {
		return wrap("merge fts", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, fromID); err != nil {
		return wrap("merge delete", err)
	}
	if err := refreshFTS(ctx, tx, intoID); err != nil {
		return err
	}
	return wrap("commit merge", tx.Commit())
}

// MergePartDuplicates merges same-title items whose files are CD1/CD2/Part N of
// one movie. Items marked keep_separate are left alone.
func (s *Store) MergePartDuplicates(ctx context.Context, libraryID int64) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.title, COALESCE(m.year,0), m.keep_separate, m.metadata_status, mf.path
		FROM media_items m JOIN media_files mf ON mf.item_id = m.id
		WHERE m.library_id = ? AND m.kind IN ('movie','home_video')
		ORDER BY m.id`, libraryID)
	if err != nil {
		return 0, wrap("part duplicates", err)
	}
	defer rows.Close()

	type group struct {
		ids      []int64
		statuses map[int64]string
		hasPart  bool
		optOut   bool
	}
	groups := map[string]*group{}
	for rows.Next() {
		var id, year int64
		var title, status, path string
		var keep int
		if err := rows.Scan(&id, &title, &year, &keep, &status, &path); err != nil {
			return 0, wrap("part scan", err)
		}
		key := strings.ToLower(strings.TrimSpace(title)) + "|" + fmt.Sprint(year)
		g := groups[key]
		if g == nil {
			g = &group{statuses: map[int64]string{}}
			groups[key] = g
		}
		g.ids = append(g.ids, id)
		g.statuses[id] = status
		if rePartFile.MatchString(path) {
			g.hasPart = true
		}
		if keep != 0 {
			g.optOut = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, wrap("part iterate", err)
	}

	merged := 0
	for _, g := range groups {
		if len(g.ids) < 2 || !g.hasPart || g.optOut {
			continue
		}
		keeper := g.ids[0]
		best := 0
		for _, id := range g.ids {
			if sc := statusScore(g.statuses[id]); sc > best {
				best, keeper = sc, id
			}
		}
		for _, id := range g.ids {
			if id == keeper {
				continue
			}
			if err := s.MergeItems(ctx, keeper, id); err != nil {
				return merged, err
			}
			merged++
		}
	}
	return merged, nil
}

func statusScore(status string) int {
	switch status {
	case "manual":
		return 3
	case "matched":
		return 2
	case "local":
		return 1
	default:
		return 0
	}
}

func bestStatus(a, b string) string {
	if statusScore(b) > statusScore(a) {
		return b
	}
	return a
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
