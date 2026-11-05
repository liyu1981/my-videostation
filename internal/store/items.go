package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/textutil"
)

// Item is a media entity: movie, show, season, episode or home video.
type Item struct {
	ID              int64     `json:"id"`
	LibraryID       int64     `json:"libraryId"`
	ParentID        int64     `json:"parentId"`
	Kind            string    `json:"kind"`
	Title           string    `json:"title"`
	SortTitle       string    `json:"sortTitle"`
	OriginalTitle   string    `json:"originalTitle,omitempty"`
	Year            int       `json:"year,omitempty"`
	ReleaseDate     string    `json:"releaseDate,omitempty"`
	Overview        string    `json:"overview,omitempty"`
	Tagline         string    `json:"tagline,omitempty"`
	RuntimeMS       int64     `json:"runtimeMs,omitempty"`
	Rating          float64   `json:"rating,omitempty"`
	ContentRating   string    `json:"contentRating,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	ProviderID      string    `json:"providerId,omitempty"`
	MetadataStatus  string    `json:"metadataStatus"`
	MatchConfidence float64   `json:"matchConfidence"`
	PosterPath      string    `json:"-"`
	BackdropPath    string    `json:"-"`
	SeasonNumber    int       `json:"seasonNumber,omitempty"`
	EpisodeNumber   int       `json:"episodeNumber,omitempty"`
	EpisodeEnd      int       `json:"episodeEnd,omitempty"`
	AddedAt         time.Time `json:"addedAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Locked          bool      `json:"locked"`
	Hidden          bool      `json:"hidden"`

	// user state & counts (computed)
	PositionMS int64 `json:"positionMs"`
	Played     bool  `json:"played"`
	DurationMS int64 `json:"durationMs,omitempty"`
	FileCount  int   `json:"fileCount"`
	ChildCount int   `json:"childCount,omitempty"`
}

// AudioStream / SubtitleStream mirror the fields we keep from ffprobe.
type AudioStream struct {
	Index      int    `json:"index"`
	Codec      string `json:"codec"`
	Channels   int    `json:"channels"`
	Layout     string `json:"layout,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Bitrate    int64  `json:"bitrate,omitempty"`
	Language   string `json:"language,omitempty"`
	Title      string `json:"title,omitempty"`
	Default    bool   `json:"default,omitempty"`
}

// SubtitleStream describes one subtitle track.
type SubtitleStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
	External bool   `json:"external,omitempty"`
}

// File is one physical file backing an item.
type File struct {
	ID              int64            `json:"id"`
	LibraryID       int64            `json:"libraryId"`
	ItemID          int64            `json:"itemId"`
	Path            string           `json:"path"`
	RelPath         string           `json:"relPath"`
	Size            int64            `json:"size"`
	Mtime           int64            `json:"mtime"`
	Inode           int64            `json:"inode"`
	Container       string           `json:"container,omitempty"`
	DurationMS      int64            `json:"durationMs,omitempty"`
	Bitrate         int64            `json:"bitrate,omitempty"`
	Width           int              `json:"width,omitempty"`
	Height          int              `json:"height,omitempty"`
	VideoCodec      string           `json:"videoCodec,omitempty"`
	HDR             string           `json:"hdr,omitempty"`
	AudioStreams    []AudioStream    `json:"audioStreams"`
	SubtitleStreams []SubtitleStream `json:"subtitleStreams"`
	ProbedAt        *time.Time       `json:"probedAt,omitempty"`
	Missing         bool             `json:"missing"`
	AddedAt         time.Time        `json:"addedAt"`
}

// Person is a cast/crew credit.
type Person struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Character string `json:"character,omitempty"`
	Ordering  int    `json:"ordering"`
}

const itemColumns = `
	m.id, m.library_id, m.parent_id, m.kind, m.title, m.sort_title,
	COALESCE(m.original_title,''), COALESCE(m.year,0), COALESCE(m.release_date,''), COALESCE(m.overview,''),
	COALESCE(m.tagline,''), COALESCE(m.runtime_ms,0), COALESCE(m.rating,0),
	COALESCE(m.content_rating,''), COALESCE(m.poster_path,''), COALESCE(m.backdrop_path,''),
	COALESCE(m.provider,''), COALESCE(m.provider_id,''), m.metadata_status, m.match_confidence,
	COALESCE(m.season_number,0), COALESCE(m.episode_number,0), COALESCE(m.episode_end,0),
	m.added_at, m.updated_at, m.locked, m.hidden`

const itemStateColumns = `
	COALESCE(p.position_ms,0), COALESCE(p.played,0), COALESCE(p.duration_ms,0),
	(SELECT COUNT(*) FROM media_files f WHERE f.item_id = m.id AND f.missing = 0)`

const itemJoin = `
	LEFT JOIN playback_progress p ON p.item_id = m.id AND p.user_id = %d`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(r rowScanner, it *Item) error {
	var added, updated string
	var locked, hidden, played int
	var state [3]int64
	if err := r.Scan(
		&it.ID, &it.LibraryID, &it.ParentID, &it.Kind, &it.Title, &it.SortTitle,
		&it.OriginalTitle, &it.Year, &it.ReleaseDate, &it.Overview, &it.Tagline, &it.RuntimeMS, &it.Rating,
		&it.ContentRating, &it.PosterPath, &it.BackdropPath,
		&it.Provider, &it.ProviderID, &it.MetadataStatus, &it.MatchConfidence,
		&it.SeasonNumber, &it.EpisodeNumber, &it.EpisodeEnd, &added, &updated, &locked, &hidden,
		&state[0], &played, &state[2], &it.FileCount,
	); err != nil {
		return err
	}
	it.AddedAt, it.UpdatedAt = parseTime(added), parseTime(updated)
	it.Locked, it.Hidden, it.Played = locked != 0, hidden != 0, played != 0
	it.PositionMS, it.DurationMS = state[0], state[2]
	if it.DurationMS == 0 {
		it.DurationMS = it.RuntimeMS
	}
	return nil
}

// ItemFilter describes a browse/list request.
type ItemFilter struct {
	LibraryID      int64
	Kind           string   // movie|show|season|episode|home_video|extra|"" (any top-level) | "all"
	Kinds          []string // explicit kind set (overrides Kind)
	ExcludeKinds   []string
	ParentID       int64 // 0 = any
	Query          string
	NamePrefix     string // sort title starts with (Jellyfin NameStartsWith)
	NameLessThan   string // sort title before (Jellyfin NameLessThan)
	Sort           string // added|title|year|rating|runtime
	Page           int
	PerPage        int
	OnlyMissing    bool
	MetadataStatus string // "" | unmatched | matched | manual
	Category       string // recent_added|recent_watched|recent_released|all|timeline
	Genre          string
	Genres         []string // names (IN)
	GenreIDs       []int64
	Year           int
	Years          []int
	Ratings        []string // content ratings (IN)
	PersonIDs      []int64
	ExcludeIDs     []int64
	Watched        *bool
	Resumable      bool   // position > 0 and not played
	Favorites      bool   // in the current user's favorites collection
	Folders        *bool  // true = shows/seasons only, false = playable kinds only
	Folder         string // rel_path prefix
}

// ListItems returns a page of items plus the total count.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (s *Store) ListItems(ctx context.Context, f ItemFilter) ([]Item, int, error) {
	if f.PerPage <= 0 || f.PerPage > 200 {
		f.PerPage = 60
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	where := []string{"m.hidden = 0"}
	args := []any{}
	if scope, scopeArgs := accessScope(ctx, "m"); scope != "" {
		where = append(where, strings.TrimPrefix(scope, " AND "))
		args = append(args, scopeArgs...)
	}
	if f.LibraryID > 0 {
		where = append(where, "m.library_id = ?")
		args = append(args, f.LibraryID)
	}
	switch {
	case len(f.Kinds) > 0:
		where = append(where, "m.kind IN ("+placeholders(len(f.Kinds))+")")
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	case f.Kind != "" && f.Kind != "all":
		where = append(where, "m.kind = ?")
		args = append(args, f.Kind)
	case f.Kind == "":
		where = append(where, "m.parent_id = 0", "m.kind IN ('movie','show','home_video')")
	}
	if len(f.ExcludeKinds) > 0 {
		where = append(where, "m.kind NOT IN ("+placeholders(len(f.ExcludeKinds))+")")
		for _, k := range f.ExcludeKinds {
			args = append(args, k)
		}
	}
	if f.Folders != nil {
		if *f.Folders {
			where = append(where, "m.kind IN ('show','season')")
		} else {
			where = append(where, "m.kind NOT IN ('show','season')")
		}
	}
	if f.ParentID > 0 {
		where = append(where, "m.parent_id = ?")
		args = append(args, f.ParentID)
	}
	if f.MetadataStatus != "" {
		where = append(where, "m.metadata_status = ?")
		args = append(args, f.MetadataStatus)
		if f.Kind == "" || f.Kind == "all" {
			where = append(where, "m.kind IN ('movie','show')")
		}
	}
	if f.Genre != "" {
		where = append(where, `EXISTS (SELECT 1 FROM item_genres ig JOIN genres g ON g.id = ig.genre_id
			WHERE ig.item_id = m.id AND g.name = ?)`)
		args = append(args, f.Genre)
	}
	if len(f.Genres) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM item_genres ig JOIN genres g ON g.id = ig.genre_id
			WHERE ig.item_id = m.id AND g.name IN (`+placeholders(len(f.Genres))+`))`)
		for _, g := range f.Genres {
			args = append(args, g)
		}
	}
	if len(f.GenreIDs) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM item_genres ig
			WHERE ig.item_id = m.id AND ig.genre_id IN (`+placeholders(len(f.GenreIDs))+`))`)
		for _, id := range f.GenreIDs {
			args = append(args, id)
		}
	}
	if f.Year > 0 {
		where = append(where, "m.year = ?")
		args = append(args, f.Year)
	}
	if len(f.Years) > 0 {
		where = append(where, "m.year IN ("+placeholders(len(f.Years))+")")
		for _, y := range f.Years {
			args = append(args, y)
		}
	}
	if len(f.Ratings) > 0 {
		where = append(where, "COALESCE(m.content_rating,'') IN ("+placeholders(len(f.Ratings))+")")
		for _, r := range f.Ratings {
			args = append(args, r)
		}
	}
	if len(f.PersonIDs) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM item_people ip
			WHERE ip.item_id = m.id AND ip.person_id IN (`+placeholders(len(f.PersonIDs))+`))`)
		for _, id := range f.PersonIDs {
			args = append(args, id)
		}
	}
	if len(f.ExcludeIDs) > 0 {
		where = append(where, "m.id NOT IN ("+placeholders(len(f.ExcludeIDs))+")")
		for _, id := range f.ExcludeIDs {
			args = append(args, id)
		}
	}
	if f.Resumable {
		where = append(where, "COALESCE(p.position_ms,0) > 0", "COALESCE(p.played,0) = 0")
	}
	if f.Favorites {
		where = append(where, `EXISTS (SELECT 1 FROM item_collections ic JOIN collections c ON c.id = ic.collection_id
			WHERE ic.item_id = m.id AND c.user_id = ? AND c.system_key = 'favorites')`)
		args = append(args, UserID(ctx))
	}
	if f.Watched != nil {
		if *f.Watched {
			where = append(where, "COALESCE(p.played,0) = 1")
		} else {
			where = append(where, "COALESCE(p.played,0) = 0")
		}
	}
	if f.NamePrefix != "" {
		where = append(where, "lower(m.sort_title) LIKE lower(?) || '%'")
		args = append(args, f.NamePrefix)
	}
	if f.NameLessThan != "" {
		where = append(where, "lower(m.sort_title) < lower(?)")
		args = append(args, f.NameLessThan)
	}
	if f.Folder != "" {
		where = append(where, `EXISTS (SELECT 1 FROM media_files mf WHERE mf.item_id = m.id AND mf.rel_path LIKE ?)`)
		args = append(args, strings.TrimSuffix(f.Folder, "/")+"/%")
	}
	if f.Category == "recent_watched" {
		where = append(where, "p.updated_at IS NOT NULL")
	}
	join := fmt.Sprintf(itemJoin, UserID(ctx))
	if q := strings.TrimSpace(f.Query); q != "" {
		join += " JOIN media_fts ft ON ft.rowid = m.id"
		where = append(where, "media_fts MATCH ?")
		args = append(args, ftsQuery(q))
	}

	order := "m.added_at DESC, m.id DESC"
	switch f.Category {
	case "all":
		order = "m.sort_title ASC, m.id ASC"
	case "recent_watched":
		order = "p.updated_at DESC, m.id DESC"
	case "recent_released":
		order = "COALESCE(m.year,0) DESC, m.added_at DESC"
	}
	switch f.Sort {
	case "title":
		order = "m.sort_title ASC, m.id ASC"
	case "year":
		order = "COALESCE(m.year,0) DESC, m.sort_title ASC"
	case "rating":
		order = "COALESCE(m.rating,0) DESC, m.sort_title ASC"
	case "runtime":
		order = "COALESCE(m.runtime_ms,0) DESC, m.sort_title ASC"
	}

	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM media_items m "+join+" WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, wrap("count items", err)
	}

	q := `SELECT ` + itemColumns + `, ` + itemStateColumns + `
		FROM media_items m ` + join + `
		WHERE ` + whereSQL + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, f.PerPage, (f.Page-1)*f.PerPage)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, wrap("list items", err)
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := scanItem(rows, &it); err != nil {
			return nil, 0, wrap("scan item", err)
		}
		out = append(out, it)
	}
	return out, total, wrap("iterate items", rows.Err())
}

// GetItem loads a single item with user state.
func (s *Store) GetItem(ctx context.Context, id int64) (*Item, error) {
	scope, scopeArgs := accessScope(ctx, "m")
	q := `SELECT ` + itemColumns + `, ` + itemStateColumns + `
		FROM media_items m ` + fmt.Sprintf(itemJoin, UserID(ctx)) + ` WHERE m.id = ?` + scope
	var it Item
	if err := scanItem(s.db.QueryRowContext(ctx, q, append([]any{id}, scopeArgs...)...), &it); err != nil {
		return nil, wrap("get item", err)
	}
	return &it, nil
}

// Neighbors returns the previous/next item in the natural browse order.
// Movies/shows/home videos: title order within the library. Episodes: within the
// season. Seasons: within the show.
func (s *Store) Neighbors(ctx context.Context, itemID int64) (*Item, *Item, error) {
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return nil, nil, err
	}

	var scopeSQL string
	var args []any
	switch item.Kind {
	case KindEpisode:
		scopeSQL = "m.parent_id = ? AND m.kind = 'episode'"
		args = []any{item.ParentID}
	case KindSeason:
		scopeSQL = "m.parent_id = ? AND m.kind = 'season'"
		args = []any{item.ParentID}
	default:
		scopeSQL = "m.library_id = ? AND m.kind = ?"
		args = []any{item.LibraryID, item.Kind}
	}

	orderKey := "m.sort_title"
	curValue := any(item.SortTitle)
	if item.Kind == KindEpisode {
		orderKey = "m.episode_number"
		curValue = item.EpisodeNumber
	} else if item.Kind == KindSeason {
		orderKey = "COALESCE(m.season_number,0)"
		curValue = item.SeasonNumber
	}

	fetch := func(dir string, cmp string, limit int) (*Item, error) {
		access, accessArgs := accessScope(ctx, "m")
		q := `SELECT ` + itemColumns + `, ` + itemStateColumns + `
			FROM media_items m ` + fmt.Sprintf(itemJoin, UserID(ctx)) + `
			WHERE ` + scopeSQL + ` AND m.hidden = 0 AND (` + orderKey + `, m.id) ` + cmp + ` (?, ?)` + access + `
			ORDER BY ` + orderKey + ` ` + dir + `, m.id ` + dir + ` LIMIT 1`
		qargs := append(append([]any{}, args...), curValue, item.ID)
		qargs = append(qargs, accessArgs...)
		rows, err := s.db.QueryContext(ctx, q, qargs...)
		if err != nil {
			return nil, wrap("neighbors", err)
		}
		var out []Item
		if err := collectItems(rows, &out); err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return nil, nil
		}
		return &out[0], nil
	}

	prev, err := fetch("DESC", "<", 1)
	if err != nil {
		return nil, nil, err
	}
	next, err := fetch("ASC", ">", 1)
	if err != nil {
		return nil, nil, err
	}
	return prev, next, nil
}

// ListExtras returns trailers/featurettes attached to an item (hidden from browse).
func (s *Store) ListExtras(ctx context.Context, itemID int64) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`
		FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE m.parent_id = ? AND m.kind = 'extra'
		ORDER BY m.title, m.id`, itemID)
	if err != nil {
		return nil, wrap("list extras", err)
	}
	var out []Item
	if err := collectItems(rows, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Item{}
	}
	return out, nil
}

// ItemFiles returns the physical files of an item.
func (s *Store) ItemFiles(ctx context.Context, itemID int64) ([]File, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, library_id, item_id, path, rel_path, size, mtime, inode,
		       COALESCE(container,''), COALESCE(duration_ms,0), COALESCE(bitrate,0),
		       COALESCE(width,0), COALESCE(height,0), COALESCE(video_codec,''), COALESCE(hdr,''),
		       audio_streams, subtitle_streams, probed_at, missing, added_at
		FROM media_files WHERE item_id = ? ORDER BY id`, itemID)
	if err != nil {
		return nil, wrap("item files", err)
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		var audio, subs string
		var probed sql.NullString
		var missing int
		var added string
		if err := rows.Scan(&f.ID, &f.LibraryID, &f.ItemID, &f.Path, &f.RelPath, &f.Size, &f.Mtime, &f.Inode,
			&f.Container, &f.DurationMS, &f.Bitrate, &f.Width, &f.Height, &f.VideoCodec, &f.HDR,
			&audio, &subs, &probed, &missing, &added); err != nil {
			return nil, wrap("scan file", err)
		}
		f.Missing = missing != 0
		f.AddedAt = parseTime(added)
		if probed.Valid {
			t := parseTime(probed.String)
			f.ProbedAt = &t
		}
		_ = json.Unmarshal([]byte(audio), &f.AudioStreams)
		_ = json.Unmarshal([]byte(subs), &f.SubtitleStreams)
		if f.AudioStreams == nil {
			f.AudioStreams = []AudioStream{}
		}
		if f.SubtitleStreams == nil {
			f.SubtitleStreams = []SubtitleStream{}
		}
		out = append(out, f)
	}
	return out, wrap("iterate files", rows.Err())
}

// FileByID loads a file for streaming.
func (s *Store) FileByID(ctx context.Context, id int64) (*File, error) {
	var f File
	var audio, subs, added string
	var missing int
	scope, scopeArgs := accessScope(ctx, "m")
	err := s.db.QueryRowContext(ctx, `
		SELECT f.id, f.library_id, f.item_id, f.path, f.rel_path, f.size, f.mtime, f.inode,
		       COALESCE(f.container,''), COALESCE(f.duration_ms,0), COALESCE(f.bitrate,0),
		       COALESCE(f.width,0), COALESCE(f.height,0), COALESCE(f.video_codec,''), COALESCE(f.hdr,''),
		       f.audio_streams, f.subtitle_streams, f.missing, f.added_at
		FROM media_files f JOIN media_items m ON m.id = f.item_id
		WHERE f.id = ?`+scope, append([]any{id}, scopeArgs...)...).
		Scan(&f.ID, &f.LibraryID, &f.ItemID, &f.Path, &f.RelPath, &f.Size, &f.Mtime, &f.Inode,
			&f.Container, &f.DurationMS, &f.Bitrate, &f.Width, &f.Height, &f.VideoCodec, &f.HDR,
			&audio, &subs, &missing, &added)
	if err != nil {
		return nil, wrap("get file", err)
	}
	f.Missing = missing != 0
	f.AddedAt = parseTime(added)
	_ = json.Unmarshal([]byte(audio), &f.AudioStreams)
	_ = json.Unmarshal([]byte(subs), &f.SubtitleStreams)
	return &f, nil
}

// ExternalSubtitles lists sidecar subtitles for a file.
func (s *Store) ExternalSubtitles(ctx context.Context, fileID int64) ([]ExternalSubtitle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_id, path, language, label, format
		FROM external_subtitles WHERE file_id = ? ORDER BY language, path`, fileID)
	if err != nil {
		return nil, wrap("external subtitles", err)
	}
	defer rows.Close()
	out := []ExternalSubtitle{}
	for rows.Next() {
		var e ExternalSubtitle
		if err := rows.Scan(&e.ID, &e.FileID, &e.Path, &e.Language, &e.Label, &e.Format); err != nil {
			return nil, wrap("scan subtitle", err)
		}
		out = append(out, e)
	}
	return out, wrap("iterate subtitles", rows.Err())
}

// ExternalSubtitleByID loads one sidecar subtitle.
func (s *Store) ExternalSubtitleByID(ctx context.Context, id int64) (*ExternalSubtitle, error) {
	var e ExternalSubtitle
	err := s.db.QueryRowContext(ctx, `
		SELECT id, file_id, path, language, label, format
		FROM external_subtitles WHERE id = ?`, id).Scan(&e.ID, &e.FileID, &e.Path, &e.Language, &e.Label, &e.Format)
	if err != nil {
		return nil, wrap("get subtitle", err)
	}
	return &e, nil
}

// NextEpisode returns the next episode in the same season, or the first
// episode of the next season when the current season is finished.
func (s *Store) NextEpisode(ctx context.Context, itemID int64) (*Item, error) {
	var seasonID, showID int64
	var number, seasonNumber int
	err := s.db.QueryRowContext(ctx, `
		SELECT e.parent_id, COALESCE(e.episode_number,0), COALESCE(s.parent_id,0), COALESCE(s.season_number,0)
		FROM media_items e
		JOIN media_items s ON s.id = e.parent_id AND s.kind = 'season'
		WHERE e.id = ? AND e.kind = 'episode'`, itemID).
		Scan(&seasonID, &number, &showID, &seasonNumber)
	if err != nil {
		return nil, nil // not an episode
	}
	first := func(season int64) (*Item, error) {
		access, accessArgs := accessScope(ctx, "m")
		rows, err := s.db.QueryContext(ctx, `
			SELECT `+itemColumns+`, `+itemStateColumns+`
			FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
			WHERE m.parent_id = ? AND m.kind = 'episode' AND m.hidden = 0`+access+`
			ORDER BY COALESCE(m.episode_number,0), m.id LIMIT 1`,
			append([]any{season}, accessArgs...)...)
		if err != nil {
			return nil, wrap("next episode", err)
		}
		defer rows.Close()
		var out []Item
		if err := collectItems(rows, &out); err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return nil, nil
		}
		return &out[0], nil
	}

	// Same season first.
	access, accessArgs := accessScope(ctx, "m")
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`
		FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE m.parent_id = ? AND m.kind = 'episode' AND m.hidden = 0 AND m.episode_number > ?`+access+`
		ORDER BY m.episode_number, m.id LIMIT 1`,
		append([]any{seasonID, number}, accessArgs...)...)
	if err != nil {
		return nil, wrap("next episode", err)
	}
	var out []Item
	if err := collectItems(rows, &out); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return &out[0], nil
	}
	if showID == 0 {
		return nil, nil
	}

	// End of season: first episode of the next season in the same show.
	var nextSeasonID int64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM media_items
		WHERE parent_id = ? AND kind = 'season' AND COALESCE(season_number,0) > ?
		ORDER BY season_number, id LIMIT 1`, showID, seasonNumber).Scan(&nextSeasonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("next season", err)
	}
	return first(nextSeasonID)
}

// ItemPeople returns cast/crew, cast first.
func (s *Store) ItemPeople(ctx context.Context, itemID int64) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, ip.role, COALESCE(ip.character,''), ip.ordering
		FROM item_people ip JOIN people p ON p.id = ip.person_id
		WHERE ip.item_id = ?
		ORDER BY CASE ip.role WHEN 'cast' THEN 0 WHEN 'director' THEN 1 WHEN 'writer' THEN 2 ELSE 3 END, ip.ordering, p.name`,
		itemID)
	if err != nil {
		return nil, wrap("item people", err)
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.Role, &p.Character, &p.Ordering); err != nil {
			return nil, wrap("scan person", err)
		}
		out = append(out, p)
	}
	return out, wrap("iterate people", rows.Err())
}

// ItemGenres returns genre names.
func (s *Store) ItemGenres(ctx context.Context, itemID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.name FROM item_genres ig JOIN genres g ON g.id = ig.genre_id
		WHERE ig.item_id = ? ORDER BY g.name`, itemID)
	if err != nil {
		return nil, wrap("item genres", err)
	}
	return scanStrings(rows)
}

// ItemLocks returns the set of locked metadata fields.
func (s *Store) ItemLocks(ctx context.Context, itemID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT field FROM item_locks WHERE item_id = ? ORDER BY field`, itemID)
	if err != nil {
		return nil, wrap("item locks", err)
	}
	return scanStrings(rows)
}

// ItemPatch is a partial metadata update.
type ItemPatch struct {
	Title         *string   `json:"title"`
	OriginalTitle *string   `json:"originalTitle"`
	Overview      *string   `json:"overview"`
	Tagline       *string   `json:"tagline"`
	ContentRating *string   `json:"contentRating"`
	ReleaseDate   *string   `json:"releaseDate"`
	Year          *int      `json:"year"`
	Cast          *[]Person `json:"cast"`
	Rating        *float64  `json:"rating"`
	Genres        *[]string `json:"genres"`
	Locked        *bool     `json:"locked"`
	Locks         *[]string `json:"locks"` // full replacement of per-field locks
}

var lockableFields = map[string]bool{
	"title": true, "original_title": true, "overview": true, "tagline": true,
	"year": true, "release_date": true, "rating": true, "content_rating": true, "genres": true,
	"poster": true, "backdrop": true, "cast": true, "crew": true,
}

// UpdateItem applies a metadata patch and refreshes the search index.
func (s *Store) UpdateItem(ctx context.Context, id int64, p ItemPatch) (*Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback()

	sets := []string{}
	args := []any{}
	add := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }

	// Any metadata edit makes the DB authoritative: a later rescan must not
	// re-apply local .nfo/.vsmeta over it.
	if p.Title != nil || p.OriginalTitle != nil || p.Overview != nil || p.Tagline != nil ||
		p.ContentRating != nil || p.ReleaseDate != nil || p.Year != nil || p.Rating != nil ||
		p.Genres != nil || p.Cast != nil {
		add("metadata_status", "manual")
		add("match_confidence", 1)
	}

	if p.Title != nil && strings.TrimSpace(*p.Title) != "" {
		t := strings.TrimSpace(*p.Title)
		add("title", t)
		add("sort_title", sortTitle(t))
	}
	if p.OriginalTitle != nil {
		add("original_title", strings.TrimSpace(*p.OriginalTitle))
	}
	if p.Overview != nil {
		add("overview", textutil.NormalizeOverview(*p.Overview))
	}
	if p.Tagline != nil {
		add("tagline", strings.TrimSpace(*p.Tagline))
	}
	if p.ContentRating != nil {
		add("content_rating", strings.TrimSpace(*p.ContentRating))
	}
	if p.ReleaseDate != nil {
		add("release_date", nullStr(strings.TrimSpace(*p.ReleaseDate)))
	}
	if p.Year != nil {
		y := *p.Year
		if y < 1800 || y > time.Now().Year()+5 {
			y = 0
		}
		if y == 0 {
			add("year", nil)
		} else {
			add("year", y)
		}
	}
	if p.Rating != nil {
		add("rating", *p.Rating)
	}
	if p.Locked != nil {
		b := 0
		if *p.Locked {
			b = 1
		}
		add("locked", b)
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = ?")
		args = append(args, now(), id)
		if _, err := tx.ExecContext(ctx,
			"UPDATE media_items SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
			return nil, wrap("update item", err)
		}
	}

	if p.Cast != nil {
		if err := replaceCredits(ctx, tx, id, "cast", *p.Cast); err != nil {
			return nil, err
		}
	}

	if p.Genres != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_genres WHERE item_id = ?`, id); err != nil {
			return nil, wrap("clear genres", err)
		}
		for _, name := range *p.Genres {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO genres (name) VALUES (?)`, name); err != nil {
				return nil, wrap("insert genre", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO item_genres (item_id, genre_id) SELECT ?, id FROM genres WHERE name = ?`,
				id, name); err != nil {
				return nil, wrap("link genre", err)
			}
		}
	}

	if p.Locks != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_locks WHERE item_id = ?`, id); err != nil {
			return nil, wrap("clear locks", err)
		}
		for _, f := range *p.Locks {
			f = strings.ToLower(strings.TrimSpace(f))
			if !lockableFields[f] {
				return nil, wrap("update item", errInvalid("unknown lock field: "+f))
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO item_locks (item_id, field) VALUES (?, ?)`, id, f); err != nil {
				return nil, wrap("insert lock", err)
			}
		}
	}

	if err := refreshFTS(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap("commit", err)
	}
	return s.GetItem(ctx, id)
}

func sortTitle(t string) string {
	low := strings.ToLower(t)
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(low, a) {
			return strings.TrimPrefix(low, a)
		}
	}
	return low
}

// SetProgress records playback position (0-duration is ignored).
func (s *Store) SetProgress(ctx context.Context, itemID, positionMS, durationMS int64, played bool) error {
	if positionMS < 0 {
		positionMS = 0
	}
	p := 0
	if played {
		p = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET
			position_ms = excluded.position_ms,
			duration_ms = excluded.duration_ms,
			played = excluded.played,
			updated_at = excluded.updated_at`,
		UserID(ctx), itemID, positionMS, durationMS, p, now())
	return wrap("set progress", err)
}

// Home is the landing payload.
type Home struct {
	Continue []Item  `json:"continue"`
	Recent   []Item  `json:"recent"`
	Shelves  []Shelf `json:"shelves"`
}

// Shelf is a home row.
type Shelf struct {
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Items []Item `json:"items"`
}

// GetHome builds the home page payload.
func (s *Store) GetHome(ctx context.Context) (*Home, error) {
	h := &Home{Continue: []Item{}, Recent: []Item{}, Shelves: []Shelf{}}
	scope, scopeArgs := accessScope(ctx, "m")

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`
		FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE m.hidden = 0 AND p.position_ms > 0 AND p.played = 0`+scope+`
		ORDER BY p.updated_at DESC LIMIT 12`, scopeArgs...)
	if err != nil {
		return nil, wrap("continue watching", err)
	}
	if err := collectItems(rows, &h.Continue); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`
		FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE m.hidden = 0 AND m.kind IN ('movie','show','home_video')`+scope+`
		ORDER BY m.added_at DESC, m.id DESC LIMIT 24`, scopeArgs...)
	if err != nil {
		return nil, wrap("recent items", err)
	}
	if err := collectItems(rows, &h.Recent); err != nil {
		return nil, err
	}

	libs, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	for _, lib := range libs {
		kind := KindMovie
		switch lib.Type {
		case "tv":
			kind = KindShow
		case "home_video", "tv_recording":
			kind = KindHomeVideo
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT `+itemColumns+`, `+itemStateColumns+`
			FROM media_items m `+fmt.Sprintf(itemJoin, UserID(ctx))+`
			WHERE m.hidden = 0 AND m.library_id = ? AND m.kind = ?`+scope+`
			ORDER BY m.added_at DESC, m.id DESC LIMIT 24`, append([]any{lib.ID, kind}, scopeArgs...)...)
		if err != nil {
			return nil, wrap("library shelf", err)
		}
		var items []Item
		if err := collectItems(rows, &items); err != nil {
			return nil, err
		}
		if len(items) > 0 {
			h.Shelves = append(h.Shelves, Shelf{Title: lib.Name, Kind: kind, Items: items})
		}
	}
	return h, nil
}

func collectItems(rows *sql.Rows, dst *[]Item) error {
	defer rows.Close()
	for rows.Next() {
		var it Item
		if err := scanItem(rows, &it); err != nil {
			return wrap("scan item", err)
		}
		*dst = append(*dst, it)
	}
	return wrap("iterate items", rows.Err())
}

// UnmatchedItemIDs lists ids of movies/shows with no metadata match.
func (s *Store) UnmatchedItemIDs(ctx context.Context, libraryID int64) ([]int64, error) {
	q := `SELECT id FROM media_items WHERE kind IN ('movie','show') AND metadata_status = 'unmatched' AND hidden = 0`
	args := []any{}
	if libraryID > 0 {
		q += ` AND library_id = ?`
		args = append(args, libraryID)
	}
	q += ` ORDER BY added_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, wrap("unmatched ids", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("scan unmatched id", err)
		}
		ids = append(ids, id)
	}
	return ids, wrap("iterate unmatched", rows.Err())
}

// Search performs full-text search across titles, overviews and people.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]Item, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Item{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	scope, scopeArgs := accessScope(ctx, "m")
	args := append([]any{ftsQuery(query)}, scopeArgs...)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`, bm25(media_fts) AS rank
		FROM media_items m
		JOIN media_fts ft ON ft.rowid = m.id
		`+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE media_fts MATCH ? AND m.hidden = 0 AND m.kind != 'season'`+scope+`
		ORDER BY rank, m.sort_title LIMIT ?`, args...)
	if err != nil {
		return nil, wrap("search", err)
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		var rank float64
		var added, updated string
		var locked, hidden, played int
		var st [3]int64
		if err := rows.Scan(
			&it.ID, &it.LibraryID, &it.ParentID, &it.Kind, &it.Title, &it.SortTitle,
			&it.OriginalTitle, &it.Year, &it.ReleaseDate, &it.Overview, &it.Tagline, &it.RuntimeMS, &it.Rating,
			&it.ContentRating, &it.PosterPath, &it.BackdropPath,
			&it.Provider, &it.ProviderID, &it.MetadataStatus, &it.MatchConfidence,
			&it.SeasonNumber, &it.EpisodeNumber, &it.EpisodeEnd, &added, &updated, &locked, &hidden,
			&st[0], &played, &st[2], &it.FileCount, &rank); err != nil {
			return nil, wrap("scan search result", err)
		}
		it.AddedAt, it.UpdatedAt = parseTime(added), parseTime(updated)
		it.Locked, it.Hidden, it.Played = locked != 0, hidden != 0, played != 0
		it.PositionMS, it.DurationMS = st[0], st[2]
		out = append(out, it)
	}
	return out, wrap("iterate search", rows.Err())
}

// ftsQuery turns user input into a safe FTS5 prefix query.
func ftsQuery(q string) string {
	parts := strings.Fields(q)
	for i, p := range parts {
		p = strings.Trim(p, `"'*()[]{}:^,-`)
		if p == "" {
			p = "*"
		}
		parts[i] = `"` + p + `"*`
	}
	return strings.Join(parts, " AND ")
}

// refreshFTS rebuilds the search row for one item.
func refreshFTS(ctx context.Context, tx *sql.Tx, itemID int64) error {
	var title, original, overview, kind string
	if err := tx.QueryRowContext(ctx,
		`SELECT title, COALESCE(original_title,''), COALESCE(overview,''), kind FROM media_items WHERE id = ?`,
		itemID).Scan(&title, &original, &overview, &kind); err != nil {
		return wrap("fts load item", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT p.name FROM item_people ip JOIN people p ON p.id = ip.person_id WHERE ip.item_id = ?`, itemID)
	if err != nil {
		return wrap("fts people", err)
	}
	names, err := scanStrings(rows)
	if err != nil {
		return wrap("fts people", err)
	}
	if err := deleteFTS(ctx, tx, itemID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO media_fts (rowid, title, original_title, overview, people) VALUES (?,?,?,?,?)`,
		itemID, title, original, overview, strings.Join(names, " "))
	return wrap("fts insert", err)
}

func deleteFTS(ctx context.Context, tx *sql.Tx, itemID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM media_fts WHERE rowid = ?`, itemID)
	return wrap("fts delete", err)
}
