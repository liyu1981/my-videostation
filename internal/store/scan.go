package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/liyu1981/myvideostation/internal/textutil"
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

	// Local sidecar metadata (.nfo / .vsmeta) to apply to this item and (for
	// episodes) to its parent show.
	LocalMeta *LocalMetadata
	ShowMeta  *LocalMetadata
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

// ResetMissingUnder flags the files below one folder as missing, leaving the rest
// of the library alone. Folder-scoped rescans use it so a show/season rescan can
// notice deletions inside its own tree without touching sibling folders.
func (s *Store) ResetMissingUnder(ctx context.Context, libraryID int64, prefix string) error {
	prefix = filepath.Clean(prefix)
	_, err := s.db.ExecContext(ctx,
		`UPDATE media_files SET missing = 1
		 WHERE library_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')`,
		libraryID, prefix, escapeLike(prefix)+"/%")
	return wrap("reset missing under", err)
}

// escapeLike escapes LIKE wildcards so a path containing % or _ matches literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
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
		if err := applyLocalMeta(ctx, tx, itemID, f.LocalMeta); err != nil {
			return 0, err
		}
		if f.Kind == KindEpisode && f.ShowMeta != nil {
			if showID, err := showIDForEpisode(ctx, tx, itemID); err == nil && showID > 0 {
				if err := applyLocalMeta(ctx, tx, showID, f.ShowMeta); err != nil {
					return 0, err
				}
			}
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
	// Identity follows the file path, not the (mutable) title: user edits and
	// sidecar metadata renames must not create duplicates on the next scan.
	var existingID int64
	var existingKind string
	var kindLocked int
	if err := tx.QueryRowContext(ctx, `
		SELECT m.id, m.kind, m.kind_locked FROM media_files mf JOIN media_items m ON m.id = mf.item_id
		WHERE mf.path = ? AND m.library_id = ?`, f.Path, f.LibraryID).Scan(&existingID, &existingKind, &kindLocked); err == nil && (existingKind == f.Kind || kindLocked != 0) {
		if err := touchItem(ctx, tx, existingID, f); err != nil {
			return 0, false, err
		}
		// A manual type assignment wins over the filename; only repair episode
		// titles when the file is still parsed as that episode.
		if existingKind == KindEpisode && f.Kind == KindEpisode {
			if err := repairEpisodeTitle(ctx, tx, existingID, f); err != nil {
				return 0, false, err
			}
		}
		return existingID, false, nil
	}

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
		err := tx.QueryRowContext(ctx, `
			SELECT m.id FROM media_items m
			JOIN media_files mf ON mf.item_id = m.id
			WHERE m.library_id = ? AND mf.path LIKE ? AND m.kind IN ('movie','show')
			ORDER BY LENGTH(mf.path) LIMIT 1`, f.LibraryID, prefix).Scan(&parentID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, false, wrap("find extra parent", err)
		}
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
	if want == "" || looksLikeJunkTitle(want) {
		want = fmt.Sprintf("Episode %d", f.EpisodeNumber)
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM media_items
		WHERE parent_id = ? AND kind = 'episode' AND season_number = ? AND episode_number = ?`,
		seasonID, f.SeasonNumber, f.EpisodeNumber).Scan(&id)
	if err == nil {
		if err := repairEpisodeTitle(ctx, tx, id, f); err != nil {
			return 0, false, err
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

// LocalMetadata is metadata imported from a local sidecar file.
type LocalMetadata struct {
	Source        string
	Title         string
	OriginalTitle string
	SortTitle     string
	Tagline       string
	Overview      string
	Certification string
	Year          int
	ReleaseDate   string
	Rating        float64
	RuntimeMS     int64
	Genres        []string
	Cast          []string
	Directors     []string
	Writers       []string
	PosterPath    string
	BackdropPath  string
}

// applyLocalMeta writes sidecar metadata, respecting locks and manual matches.
func applyLocalMeta(ctx context.Context, tx *sql.Tx, itemID int64, m *LocalMetadata) error {
	if m == nil || (m.Title == "" && m.Overview == "") {
		return nil
	}
	var locked int
	var status, provider, kind string
	if err := tx.QueryRowContext(ctx,
		`SELECT locked, metadata_status, COALESCE(provider,''), kind FROM media_items WHERE id = ?`,
		itemID).Scan(&locked, &status, &provider, &kind); err != nil {
		return wrap("local meta load item", err)
	}
	// Local sidecars are the import-time source. Once an item was matched from
	// TMDb ("matched") or edited/identified by the user ("manual"), the DB wins
	// and a rescan must not revert it.
	if locked != 0 || status == "manual" || status == "matched" {
		return nil
	}
	locks := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT field FROM item_locks WHERE item_id = ?`, itemID)
	if err != nil {
		return wrap("local meta locks", err)
	}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return wrap("scan lock", err)
		}
		locks[f] = true
	}
	rows.Close()

	sets := []string{"provider = ?", "metadata_status = 'local'", "match_confidence = 0",
		"last_refresh_at = ?", "updated_at = ?"}
	args := []any{m.Source, now(), now()}
	add := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }

	if !locks["title"] && m.Title != "" && !(kind == KindEpisode && looksLikeJunkTitle(m.Title)) {
		add("title", m.Title)
		sortT := m.SortTitle
		if sortT == "" {
			sortT = sortTitle(m.Title)
		}
		add("sort_title", sortT)
	}
	if !locks["original_title"] && m.OriginalTitle != "" {
		add("original_title", m.OriginalTitle)
	}
	if !locks["tagline"] && m.Tagline != "" {
		add("tagline", m.Tagline)
	}
	if !locks["overview"] && m.Overview != "" {
		add("overview", textutil.NormalizeOverview(m.Overview))
	}
	if !locks["content_rating"] && m.Certification != "" {
		add("content_rating", m.Certification)
	}
	if !locks["year"] && m.Year > 0 {
		add("year", m.Year)
	}
	if !locks["release_date"] && m.ReleaseDate != "" {
		add("release_date", m.ReleaseDate)
	}
	if !locks["rating"] && m.Rating > 0 {
		add("rating", m.Rating)
	}
	if m.RuntimeMS > 0 {
		add("runtime_ms", m.RuntimeMS)
	}
	if !locks["poster"] && m.PosterPath != "" {
		add("poster_path", m.PosterPath)
	}
	if !locks["backdrop"] && m.BackdropPath != "" {
		add("backdrop_path", m.BackdropPath)
	}

	args = append(args, itemID)
	if _, err := tx.ExecContext(ctx,
		`UPDATE media_items SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return wrap("apply local meta", err)
	}

	if !locks["genres"] && len(m.Genres) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_genres WHERE item_id = ?`, itemID); err != nil {
			return wrap("clear genres", err)
		}
		for _, name := range m.Genres {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO genres (name) VALUES (?)`, name); err != nil {
				return wrap("insert genre", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO item_genres (item_id, genre_id) SELECT ?, id FROM genres WHERE name = ?`,
				itemID, name); err != nil {
				return wrap("link genre", err)
			}
		}
	}
	if !locks["cast"] && len(m.Cast) > 0 {
		people := make([]Person, 0, len(m.Cast))
		for i, name := range m.Cast {
			people = append(people, Person{Name: name, Role: "cast", Ordering: i})
		}
		if err := replaceCredits(ctx, tx, itemID, "cast", people); err != nil {
			return err
		}
	}
	if !locks["crew"] && (len(m.Directors) > 0 || len(m.Writers) > 0) {
		people := make([]Person, 0, len(m.Directors)+len(m.Writers))
		for i, name := range m.Directors {
			people = append(people, Person{Name: name, Role: "director", Ordering: i})
		}
		for i, name := range m.Writers {
			people = append(people, Person{Name: name, Role: "writer", Ordering: len(m.Directors) + i})
		}
		if err := replaceCredits(ctx, tx, itemID, "crew", people); err != nil {
			return err
		}
	}
	if err := refreshFTS(ctx, tx, itemID); err != nil {
		return err
	}
	return nil
}

// repairEpisodeTitle fixes episode titles stored as raw filenames (older scans) and
// upgrades placeholder titles when a real one is available.
func repairEpisodeTitle(ctx context.Context, tx *sql.Tx, id int64, f *ScannedFile) error {
	var cur string
	if err := tx.QueryRowContext(ctx, `SELECT title FROM media_items WHERE id = ?`, id).Scan(&cur); err != nil {
		return nil
	}
	want := f.EpisodeTitle
	if want == "" || looksLikeJunkTitle(want) {
		want = fmt.Sprintf("Episode %d", f.EpisodeNumber)
	}
	if want == cur {
		return nil
	}
	if !looksLikeJunkTitle(cur) && !strings.HasPrefix(cur, "Episode ") {
		return nil // a real, user/provider-provided title wins
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE media_items SET title = ?, sort_title = ?, updated_at = ? WHERE id = ?`,
		want, sortTitle(want), now(), id)
	return wrap("repair episode title", err)
}

var (
	reEpisodeCode = regexp.MustCompile(`(?i)s\d{1,2}[\s._-]*e\d{1,3}`)
	reReleaseJunk = regexp.MustCompile(`(?i)(1080p|720p|2160p|480p|web[- .]?dl|webrip|bluray|blu-ray|bdrip|hdtv|x ?26[45]|h ?26[45]|hevc|ddp? ?5|ac3|dts|remux|proper|repack|hdr)`)
)

// looksLikeJunkTitle reports filename-ish episode titles ("Show.S01E01.X", "Chi_Eng").
func looksLikeJunkTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return true
	}
	if reEpisodeCode.MatchString(t) {
		return true
	}
	if reReleaseJunk.MatchString(t) {
		return true
	}
	if strings.ContainsAny(t, "/\\") {
		return true
	}
	if (strings.Contains(t, "_") || strings.Contains(t, ".")) && !strings.Contains(t, " ") {
		return true
	}
	return false
}

// showIDForEpisode resolves the show that owns an episode.
func showIDForEpisode(ctx context.Context, tx *sql.Tx, episodeID int64) (int64, error) {
	var showID int64
	err := tx.QueryRowContext(ctx, `
		SELECT s.parent_id FROM media_items e
		JOIN media_items s ON s.id = e.parent_id
		WHERE e.id = ? AND e.kind = 'episode' AND s.kind = 'season'`, episodeID).Scan(&showID)
	if err != nil {
		return 0, nil // not an episode or no show; not fatal
	}
	return showID, nil
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
		sets := []string{"item_id = ?", "library_id = ?", "missing = 0"}
		args := []any{itemID, f.LibraryID}
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
		`SELECT id FROM media_items m WHERE m.library_id = ? AND m.kind = 'extra'
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

// FileRef is the minimal file row needed to reconcile scan-rule exclusions
// with item visibility. It is intentionally read-only bookkeeping.
type FileRef struct {
	ItemID  int64
	Kind    string
	Path    string
	Size    int64
	Missing bool
	Hidden  bool
}

// LibraryFileRefs returns every file of a library with its item visibility.
func (s *Store) LibraryFileRefs(ctx context.Context, libraryID int64) ([]FileRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.item_id, m.kind, f.path, f.size, f.missing, m.hidden
		FROM media_files f JOIN media_items m ON m.id = f.item_id
		WHERE f.library_id = ?`, libraryID)
	if err != nil {
		return nil, wrap("library file refs", err)
	}
	defer rows.Close()
	out := []FileRef{}
	for rows.Next() {
		var r FileRef
		var missing, hidden int
		if err := rows.Scan(&r.ItemID, &r.Kind, &r.Path, &r.Size, &missing, &hidden); err != nil {
			return nil, wrap("scan file ref", err)
		}
		r.Missing, r.Hidden = missing != 0, hidden != 0
		out = append(out, r)
	}
	return out, wrap("iterate file refs", rows.Err())
}

// SetItemHidden toggles the internal visibility flag (scan rules only).
func (s *Store) SetItemHidden(ctx context.Context, id int64, hidden bool) error {
	v := 0
	if hidden {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE media_items SET hidden = ?, updated_at = ? WHERE id = ?`, v, now(), id)
	return wrap("set item hidden", err)
}
