package store

import (
	"context"
	"database/sql"
	"errors"
)

// mergeShows moves the from show's seasons/episodes (and show-level credits,
// locks, collections and progress) into the into show, then deletes the from show.
func (s *Store) mergeShows(ctx context.Context, intoID, fromID int64) error {
	into, err := s.GetItem(ctx, intoID)
	if err != nil {
		return err
	}
	from, err := s.GetItem(ctx, fromID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin show merge", err)
	}
	defer tx.Rollback()

	// Show-level rows (no media files: episodes own those).
	for _, q := range []string{
		`INSERT OR IGNORE INTO item_genres (item_id, genre_id) SELECT ?, genre_id FROM item_genres WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_people (item_id, person_id, role, character, ordering) SELECT ?, person_id, role, character, ordering FROM item_people WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_locks (item_id, field) SELECT ?, field FROM item_locks WHERE item_id = ?`,
		`INSERT OR IGNORE INTO item_collections (collection_id, item_id) SELECT collection_id, ? FROM item_collections WHERE item_id = ?`,
		`INSERT OR IGNORE INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at) SELECT user_id, ?, position_ms, duration_ms, played, updated_at FROM playback_progress WHERE item_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, intoID, fromID); err != nil {
			return wrap("show merge rows", err)
		}
	}
	for _, q := range []string{
		`DELETE FROM item_genres WHERE item_id = ?`,
		`DELETE FROM item_people WHERE item_id = ?`,
		`DELETE FROM item_locks WHERE item_id = ?`,
		`DELETE FROM item_collections WHERE item_id = ?`,
		`DELETE FROM playback_progress WHERE item_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, fromID); err != nil {
			return wrap("show merge clear", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_items SET parent_id = ? WHERE parent_id = ? AND kind = 'extra'`, intoID, fromID); err != nil {
		return wrap("show merge extras", err)
	}

	// Seasons: move or merge by season number.
	seasons, err := tx.QueryContext(ctx, `SELECT id, season_number FROM media_items WHERE parent_id = ? AND kind = 'season'`, fromID)
	if err != nil {
		return wrap("show merge seasons", err)
	}
	type seasonRow struct {
		id     int64
		number int
	}
	var seasonList []seasonRow
	for seasons.Next() {
		var sr seasonRow
		if err := seasons.Scan(&sr.id, &sr.number); err != nil {
			seasons.Close()
			return wrap("show merge season scan", err)
		}
		seasonList = append(seasonList, sr)
	}
	seasons.Close()
	if err := seasons.Err(); err != nil {
		return wrap("show merge seasons iter", err)
	}

	for _, sr := range seasonList {
		var keeperSeason int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM media_items WHERE parent_id = ? AND kind = 'season' AND season_number = ?`, intoID, sr.number).
			Scan(&keeperSeason)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `UPDATE media_items SET parent_id = ? WHERE id = ?`, intoID, sr.id); err != nil {
				return wrap("show merge move season", err)
			}
			continue
		}
		if err != nil {
			return wrap("show merge find season", err)
		}
		if err := mergeSeasonInto(ctx, tx, keeperSeason, sr.id); err != nil {
			return err
		}
	}

	status := bestStatus(into.MetadataStatus, from.MetadataStatus)
	takeFrom := statusScore(from.MetadataStatus) > statusScore(into.MetadataStatus)
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_items SET
			original_title = COALESCE(NULLIF(original_title,''), ?),
			overview = COALESCE(NULLIF(overview,''), ?),
			tagline = COALESCE(NULLIF(tagline,''), ?),
			content_rating = COALESCE(NULLIF(content_rating,''), ?),
			poster_path = COALESCE(NULLIF(poster_path,''), ?),
			backdrop_path = COALESCE(NULLIF(backdrop_path,''), ?),
			rating = CASE WHEN COALESCE(rating,0) > 0 THEN rating ELSE ? END,
			year = CASE WHEN COALESCE(year,0) > 0 THEN year ELSE ? END,
			provider = CASE WHEN ? = 1 THEN ? ELSE COALESCE(NULLIF(provider,''), ?) END,
			provider_id = CASE WHEN ? = 1 THEN ? ELSE COALESCE(NULLIF(provider_id,''), ?) END,
			metadata_status = ?,
			match_confidence = MAX(match_confidence, ?),
			keep_separate = 0,
			updated_at = ?
		WHERE id = ?`,
		from.OriginalTitle, from.Overview, from.Tagline, from.ContentRating,
		from.PosterPath, from.BackdropPath, from.Rating, from.Year,
		boolInt(takeFrom), from.Provider, from.Provider,
		boolInt(takeFrom), from.ProviderID, from.ProviderID,
		status, from.MatchConfidence, now(), intoID); err != nil {
		return wrap("show merge metadata", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM media_fts WHERE rowid = ?`, fromID); err != nil {
		return wrap("show merge fts", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, fromID); err != nil {
		return wrap("show merge delete", err)
	}
	if err := refreshFTS(ctx, tx, intoID); err != nil {
		return err
	}
	return wrap("commit show merge", tx.Commit())
}

// mergeSeasonInto moves the from season's episodes into the keeper season and
// deletes the from season. Duplicate episode numbers share one episode item.
func mergeSeasonInto(ctx context.Context, tx *sql.Tx, intoSeason, fromSeason int64) error {
	eps, err := tx.QueryContext(ctx, `SELECT id, episode_number FROM media_items WHERE parent_id = ? AND kind = 'episode'`, fromSeason)
	if err != nil {
		return wrap("season merge episodes", err)
	}
	type epRow struct {
		id     int64
		number int
	}
	var list []epRow
	for eps.Next() {
		var e epRow
		if err := eps.Scan(&e.id, &e.number); err != nil {
			eps.Close()
			return wrap("season merge scan", err)
		}
		list = append(list, e)
	}
	eps.Close()
	if err := eps.Err(); err != nil {
		return wrap("season merge iter", err)
	}

	for _, e := range list {
		var keeper int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM media_items WHERE parent_id = ? AND kind = 'episode' AND episode_number = ?`, intoSeason, e.number).
			Scan(&keeper)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `UPDATE media_items SET parent_id = ? WHERE id = ?`, intoSeason, e.id); err != nil {
				return wrap("season merge move episode", err)
			}
			continue
		}
		if err != nil {
			return wrap("season merge find episode", err)
		}
		for _, q := range []string{
			`UPDATE media_files SET item_id = ? WHERE item_id = ?`,
			`INSERT OR IGNORE INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at) SELECT user_id, ?, position_ms, duration_ms, played, updated_at FROM playback_progress WHERE item_id = ?`,
			`DELETE FROM playback_progress WHERE item_id = ?`,
			`INSERT OR IGNORE INTO item_collections (collection_id, item_id) SELECT collection_id, ? FROM item_collections WHERE item_id = ?`,
			`DELETE FROM item_collections WHERE item_id = ?`,
			`INSERT OR IGNORE INTO item_locks (item_id, field) SELECT ?, field FROM item_locks WHERE item_id = ?`,
			`DELETE FROM item_locks WHERE item_id = ?`,
		} {
			var args []any
			switch q {
			case `UPDATE media_files SET item_id = ? WHERE item_id = ?`,
				`INSERT OR IGNORE INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at) SELECT user_id, ?, position_ms, duration_ms, played, updated_at FROM playback_progress WHERE item_id = ?`,
				`INSERT OR IGNORE INTO item_collections (collection_id, item_id) SELECT collection_id, ? FROM item_collections WHERE item_id = ?`,
				`INSERT OR IGNORE INTO item_locks (item_id, field) SELECT ?, field FROM item_locks WHERE item_id = ?`:
				args = []any{keeper, e.id}
			default:
				args = []any{e.id}
			}
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return wrap("season merge episode rows", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_fts WHERE rowid = ?`, e.id); err != nil {
			return wrap("season merge episode fts", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, e.id); err != nil {
			return wrap("season merge delete episode", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, fromSeason); err != nil {
		return wrap("season merge delete", err)
	}
	return nil
}
