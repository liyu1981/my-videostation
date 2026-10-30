package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ScannedFile is one discovered video file ready to be persisted.
type ScannedFile struct {
	LibraryID int64
	Path      string
	RelPath   string
	Size      int64
	Mtime     int64
	Inode     int64

	// Kind is KindMovie, KindHomeVideo or KindEpisode.
	Kind  string
	Title string // movie / home-video title (ignored for episodes)
	Year  int

	// Episodes: the show and season are created on demand.
	ShowTitle     string
	SeasonNumber  int
	EpisodeNumber int
	EpisodeEnd    int
	EpisodeTitle  string

	// Extras: attached to whichever library item lives in ParentDir ("" = unattached).
	ParentDir string
	ExtraType string

	PosterPath   string
	BackdropPath string

	Container  string
	DurationMS int64
	Bitrate    int64
	Width      int
	Height     int
	VideoCodec string
	HDR        string
	Audio      []AudioStream
	Subtitle   []SubtitleStream
	Probed     bool

	// Sidecar subtitles found next to the file (nil = leave untouched).
	Subtitles []SubtitleFile
}

// FileStat is the cheap fingerprint used for incremental scans.
type FileStat struct {
	Size  int64
	Mtime int64
}

// FileStats returns known files for a library keyed by absolute path.
func (s *Store) FileStats(ctx context.Context, libraryID int64) (map[string]FileStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, size, mtime FROM media_files WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, wrap("file stats", err)
	}
	defer rows.Close()
	out := map[string]FileStat{}
	for rows.Next() {
		var p string
		var st FileStat
		if err := rows.Scan(&p, &st.Size, &st.Mtime); err != nil {
			return nil, wrap("scan file stat", err)
		}
		out[p] = st
	}
	return out, wrap("iterate file stats", rows.Err())
}

// ResetMissing flags every file of a library as missing; the scan clears flags for
// files it sees again.
func (s *Store) ResetMissing(ctx context.Context, libraryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE media_files SET missing = 1 WHERE library_id = ?`, libraryID)
	return wrap("reset missing", err)
}

// ApplyScanBatch persists a batch of scanned files in one transaction.
func (s *Store) ApplyScanBatch(ctx context.Context, files []ScannedFile) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, wrap("begin scan batch", err)
	}
	defer tx.Rollback()

	created := 0
	touched := map[int64]struct{}{}

	for i := range files {
		f := &files[i]
		itemID, isNew, err := ensureItem(ctx, tx, f)
		if err != nil {
			return 0, err
		}
		if isNew {
			created++
		}
		if err := ensureFile(ctx, tx, f, itemID); err != nil {
			return 0, err
		}
		touched[itemID] = struct{}{}
	}

	for id := range touched {
		if err := refreshFTS(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, wrap("commit scan batch", err)
	}
	return created, nil
}

func ensureItem(ctx context.Context, tx *sql.Tx, f *ScannedFile) (int64, bool, error) {
	switch f.Kind {
	case KindMovie, KindHomeVideo:
		return ensureLeafItem(ctx, tx, f.LibraryID, f.Kind, f.Title, f.Year, f)

	case KindExtra:
		return ensureExtra(ctx, tx, f)

	case KindEpisode:
		showID, _, err := ensureShow(ctx, tx, f.LibraryID, f.ShowTitle, f.Year, f.PosterPath, f.BackdropPath)
		if err != nil {
			return 0, false, err
		}
		seasonID, err := ensureSeason(ctx, tx, f.LibraryID, showID, f.SeasonNumber, f.PosterPath)
		if err != nil {
			return 0, false, err
		}
		return ensureEpisode(ctx, tx, f, showID, seasonID)
	}
	return 0, false, wrap("ensure item", errInvalid("unsupported kind: "+f.Kind))
}

// ensureExtra indexes a trailer/featurette, attached to the item in ParentDir when found.
func ensureExtra(ctx context.Context, tx *sql.Tx, f *ScannedFile) (int64, bool, error) {
	parentID := int64(0)
	if f.ParentDir != "" {
		prefix := strings.TrimSuffix(f.ParentDir, "/") + "/%"
		_ = tx.QueryRowContext(ctx, `
			SELECT m.id FROM media_items m
			JOIN media_files mf ON mf.item_id = m.id
			WHERE m.library_id = ? AND mf.path LIKE ? AND m.kind IN ('movie','show')
			ORDER BY LENGTH(mf.path) LIMIT 1`, f.LibraryID, prefix).Scan(&parentID)
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM media_items WHERE library_id = ? AND kind = 'extra'
		  AND parent_id = ? AND title = ?`, f.LibraryID, parentID, f.Title).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, wrap("find extra", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_items
			(library_id, parent_id, kind, title, sort_title, runtime_ms, hidden, added_at, updated_at)
		VALUES (?,?,'extra',?,?,?,1,?,?)`,
		f.LibraryID, parentID, f.Title, sortTitle(f.Title), f.DurationMS, now(), now())
	if err != nil {
		return 0, false, wrap("insert extra", err)
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

// ensureLeafItem finds or inserts a movie / home video.
func ensureLeafItem(ctx context.Context, tx *sql.Tx, libraryID int64, kind, title string, year int, f *ScannedFile) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM media_items
		WHERE library_id = ? AND kind = ? AND title = ? AND COALESCE(year,0) = ?`,
		libraryID, kind, title, year).Scan(&id)
	if err == nil {
		if err := touchItem(ctx, tx, id, f); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, wrap("find item", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_items
			(library_id, parent_id, kind, title, sort_title, year, runtime_ms,
			 poster_path, backdrop_path, added_at, updated_at)
		VALUES (?,0,?,?,?,?,?,?,?,?,?)`,
		libraryID, kind, title, sortTitle(title), nullInt(year),
		f.DurationMS, nullStr(f.PosterPath), nullStr(f.BackdropPath), now(), now())
	if err != nil {
		return 0, false, wrap("insert item", err)
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

func ensureShow(ctx context.Context, tx *sql.Tx, libraryID int64, title string, year int, poster, backdrop string) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM media_items WHERE library_id = ? AND kind = 'show' AND title = ?`,
		libraryID, title).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, wrap("find show", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_items
			(library_id, parent_id, kind, title, sort_title, year, poster_path, backdrop_path, added_at, updated_at)
		VALUES (?,0,'show',?,?,?,?,?,?,?)`,
		libraryID, title, sortTitle(title), nullInt(year),
		nullStr(poster), nullStr(backdrop), now(), now())
	if err != nil {
		return 0, false, wrap("insert show", err)
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

func ensureSeason(ctx context.Context, tx *sql.Tx, libraryID, showID int64, number int, poster string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM media_items WHERE parent_id = ? AND kind = 'season' AND season_number = ?`,
		showID, number).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, wrap("find season", err)
	}
	title := fmt.Sprintf("Season %d", number)
	if number == 0 {
		title = "Specials"
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_items
			(library_id, parent_id, kind, title, sort_title, season_number, poster_path, added_at, updated_at)
		VALUES (?,?,'season',?,?,?,?,?,?)`,
		libraryID, showID, title, sortTitle(title), number, nullStr(poster), now(), now())
	if err != nil {
		return 0, wrap("insert season", err)
	}
	return res.LastInsertId()
}

func ensureEpisode(ctx context.Context, tx *sql.Tx, f *ScannedFile, showID, seasonID int64) (int64, bool, error) {
	want := f.EpisodeTitle
	if want == "" {
		want = fmt.Sprintf("Episode %d", f.EpisodeNumber)
	}
	var id int64
	var curTitle string
	err := tx.QueryRowContext(ctx, `
		SELECT id, title FROM media_items
		WHERE parent_id = ? AND kind = 'episode' AND season_number = ? AND episode_number = ?`,
		seasonID, f.SeasonNumber, f.EpisodeNumber).Scan(&id, &curTitle)
	if err == nil {
		if f.EpisodeTitle != "" && strings.HasPrefix(curTitle, "Episode ") {
			if _, err := tx.ExecContext(ctx,
				`UPDATE media_items SET title = ?, sort_title = ?, updated_at = ? WHERE id = ?`,
				want, sortTitle(want), now(), id); err != nil {
				return 0, false, wrap("update episode title", err)
			}
		}
		if err := touchItem(ctx, tx, id, f); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, wrap("find episode", err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_items
			(library_id, parent_id, kind, title, sort_title, season_number, episode_number,
			 episode_end, runtime_ms, poster_path, added_at, updated_at)
		VALUES (?,?,'episode',?,?,?,?,?,?,?,?,?)`,
		f.LibraryID, seasonID, want, sortTitle(want), f.SeasonNumber, f.EpisodeNumber,
		nullInt(f.EpisodeEnd), f.DurationMS, nullStr(f.PosterPath), now(), now())
	if err != nil {
		return 0, false, wrap("insert episode", err)
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

// touchItem updates probe-derived fields without clobbering user edits.
func touchItem(ctx context.Context, tx *sql.Tx, id int64, f *ScannedFile) error {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	if f.DurationMS > 0 {
		sets = append(sets, "runtime_ms = MAX(COALESCE(runtime_ms,0), ?)")
		args = append(args, f.DurationMS)
	}
	if f.PosterPath != "" {
		sets = append(sets, "poster_path = COALESCE(NULLIF(poster_path,''), ?)")
		args = append(args, f.PosterPath)
	}
	if f.BackdropPath != "" {
		sets = append(sets, "backdrop_path = COALESCE(NULLIF(backdrop_path,''), ?)")
		args = append(args, f.BackdropPath)
	}
	args = append(args, id)
	_, err := tx.ExecContext(ctx, `UPDATE media_items SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return wrap("touch item", err)
}

// SubtitleFile is a sidecar subtitle stored for a media file.
type SubtitleFile struct {
	Path     string
	Language string
	Label    string
	Format   string
}

// ExternalSubtitle is a persisted sidecar subtitle.
type ExternalSubtitle struct {
	ID       int64  `json:"id"`
	FileID   int64  `json:"fileId"`
	Path     string `json:"-"`
	Language string `json:"language,omitempty"`
	Label    string `json:"label"`
	Format   string `json:"format"`
}

func ensureFile(ctx context.Context, tx *sql.Tx, f *ScannedFile, itemID int64) error {
	audio := mustJSON(f.Audio)
	subs := mustJSON(f.Subtitle)

	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM media_files WHERE path = ?`, f.Path).Scan(&id)
	if err == nil {
		sets := []string{"item_id = ?", "missing = 0"}
		args := []any{itemID}
		if f.Probed {
			sets = append(sets,
				"size = ?", "mtime = ?", "inode = ?", "container = ?", "duration_ms = ?",
				"bitrate = ?", "width = ?", "height = ?", "video_codec = ?", "hdr = ?",
				"audio_streams = ?", "subtitle_streams = ?", "probed_at = ?")
			args = append(args, f.Size, f.Mtime, f.Inode, f.Container, f.DurationMS,
				f.Bitrate, f.Width, f.Height, f.VideoCodec, f.HDR, audio, subs, now())
		} else {
			sets = append(sets, "size = ?", "mtime = ?", "inode = ?")
			args = append(args, f.Size, f.Mtime, f.Inode)
		}
		args = append(args, id)
		if _, err := tx.ExecContext(ctx,
			`UPDATE media_files SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return wrap("update file", err)
		}
		return replaceExternalSubtitles(ctx, tx, id, f.Subtitles)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return wrap("find file", err)
	}

	var probedAt any
	if f.Probed {
		probedAt = now()
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media_files
			(library_id, item_id, path, rel_path, size, mtime, inode, container, duration_ms,
			 bitrate, width, height, video_codec, hdr, audio_streams, subtitle_streams,
			 probed_at, missing, added_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?)`,
		f.LibraryID, itemID, f.Path, f.RelPath, f.Size, f.Mtime, f.Inode, f.Container,
		f.DurationMS, f.Bitrate, f.Width, f.Height, f.VideoCodec, f.HDR, audio, subs, probedAt, now())
	if err != nil {
		return wrap("insert file", err)
	}
	id, _ = res.LastInsertId()
	return replaceExternalSubtitles(ctx, tx, id, f.Subtitles)
}

// replaceExternalSubtitles syncs sidecar subtitle rows for a file.
func replaceExternalSubtitles(ctx context.Context, tx *sql.Tx, fileID int64, subs []SubtitleFile) error {
	if subs == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM external_subtitles WHERE file_id = ?`, fileID); err != nil {
		return wrap("clear subtitles", err)
	}
	for _, sub := range subs {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO external_subtitles (file_id, path, language, label, format)
			VALUES (?,?,?,?,?)`, fileID, sub.Path, sub.Language, sub.Label, sub.Format); err != nil {
			return wrap("insert subtitle", err)
		}
	}
	return nil
}

// DeleteOrphanItems removes episodes, seasons and shows that lost all files.
func (s *Store) DeleteOrphanItems(ctx context.Context, libraryID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin cleanup", err)
	}
	defer tx.Rollback()

	queries := []string{
		`SELECT id FROM media_items m WHERE m.library_id = ? AND m.kind = 'episode'
		   AND NOT EXISTS (SELECT 1 FROM media_files f WHERE f.item_id = m.id AND f.missing = 0)`,
		`SELECT id FROM media_items m WHERE m.library_id = ? AND m.kind = 'season'
		   AND NOT EXISTS (SELECT 1 FROM media_items c WHERE c.parent_id = m.id)`,
		`SELECT id FROM media_items m WHERE m.library_id = ? AND m.kind = 'show'
		   AND NOT EXISTS (SELECT 1 FROM media_items c WHERE c.parent_id = m.id)`,
	}
	for _, q := range queries {
		rows, err := tx.QueryContext(ctx, q, libraryID)
		if err != nil {
			return wrap("find orphans", err)
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return wrap("scan orphan", err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			if err := deleteFTS(ctx, tx, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, id); err != nil {
				return wrap("delete orphan", err)
			}
		}
	}
	return wrap("commit cleanup", tx.Commit())
}

// TouchItemRefresh reindexes the search row for an item (used after bulk edits).
func (s *Store) TouchItemRefresh(ctx context.Context, itemID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()
	if err := refreshFTS(ctx, tx, itemID); err != nil {
		return err
	}
	return wrap("commit", tx.Commit())
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
