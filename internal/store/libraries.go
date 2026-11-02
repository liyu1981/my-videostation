package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Library is a media library with its configured paths.
type Library struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	Type            string        `json:"type"`
	Language        string        `json:"language"`
	Paths           []LibraryPath `json:"paths"`
	ItemCount       int64         `json:"itemCount"`
	AutoScanMinutes int           `json:"autoScanMinutes"`
	WatchEnabled    bool          `json:"watchEnabled"`
	LastAutoScanAt  *time.Time    `json:"lastAutoScanAt,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

// LibraryPath is one root folder of a library.
type LibraryPath struct {
	ID         int64      `json:"id"`
	Path       string     `json:"path"`
	Recursive  bool       `json:"recursive"`
	LastScanAt *time.Time `json:"lastScanAt,omitempty"`
}

func validLibraryType(t string) bool {
	switch t {
	case "movie", "tv", "home_video", "tv_recording":
		return true
	}
	return false
}

// ListLibraries returns all libraries with item counts.
func (s *Store) ListLibraries(ctx context.Context) ([]Library, error) {
	scope, scopeArgs := libraryScope(ctx, "l")
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.name, l.type, l.language, l.created_at, l.updated_at,
		       (SELECT COUNT(*) FROM media_items m
		         WHERE m.library_id = l.id AND m.kind IN ('movie','show','home_video')) AS item_count,
		       l.auto_scan_minutes, l.watch_enabled, l.last_auto_scan_at
		FROM libraries l WHERE 1=1`+scope+` ORDER BY l.name COLLATE NOCASE`, scopeArgs...)
	if err != nil {
		return nil, wrap("list libraries", err)
	}
	defer rows.Close()

	var out []Library
	for rows.Next() {
		var l Library
		var created, updated string
		var lastAuto sql.NullString
		var watch int
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.Language, &created, &updated, &l.ItemCount,
			&l.AutoScanMinutes, &watch, &lastAuto); err != nil {
			return nil, wrap("scan library", err)
		}
		l.CreatedAt, l.UpdatedAt = parseTime(created), parseTime(updated)
		l.WatchEnabled = watch != 0
		if lastAuto.Valid {
			t := parseTime(lastAuto.String)
			l.LastAutoScanAt = &t
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, wrap("iterate libraries", err)
	}
	for i := range out {
		paths, err := s.libraryPaths(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Paths = paths
	}
	if out == nil {
		out = []Library{}
	}
	return out, nil
}

func (s *Store) libraryPaths(ctx context.Context, id int64) ([]LibraryPath, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, path, recursive, last_scan_at FROM library_paths WHERE library_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, wrap("library paths", err)
	}
	defer rows.Close()
	out := []LibraryPath{}
	for rows.Next() {
		var p LibraryPath
		var recursive int
		var last sql.NullString
		if err := rows.Scan(&p.ID, &p.Path, &recursive, &last); err != nil {
			return nil, wrap("scan library path", err)
		}
		p.Recursive = recursive != 0
		if last.Valid {
			t := parseTime(last.String)
			p.LastScanAt = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetLibrary loads one library.
func (s *Store) GetLibrary(ctx context.Context, id int64) (*Library, error) {
	var l Library
	var created, updated string
	var lastAuto sql.NullString
	var watch int
	scope, scopeArgs := libraryScope(ctx, "l")
	err := s.db.QueryRowContext(ctx, `
		SELECT l.id, l.name, l.type, l.language, l.created_at, l.updated_at,
		       (SELECT COUNT(*) FROM media_items m
		         WHERE m.library_id = l.id AND m.kind IN ('movie','show','home_video')),
		       l.auto_scan_minutes, l.watch_enabled, l.last_auto_scan_at
		FROM libraries l WHERE l.id = ?`+scope, append([]any{id}, scopeArgs...)...).
		Scan(&l.ID, &l.Name, &l.Type, &l.Language, &created, &updated, &l.ItemCount,
			&l.AutoScanMinutes, &watch, &lastAuto)
	if err != nil {
		return nil, wrap("get library", err)
	}
	l.CreatedAt, l.UpdatedAt = parseTime(created), parseTime(updated)
	l.WatchEnabled = watch != 0
	if lastAuto.Valid {
		t := parseTime(lastAuto.String)
		l.LastAutoScanAt = &t
	}
	if l.Paths, err = s.libraryPaths(ctx, id); err != nil {
		return nil, err
	}
	return &l, nil
}

// CreateLibrary inserts a library with default options (auto-scan off, watcher on).
func (s *Store) CreateLibrary(ctx context.Context, name, typ, language string, paths []string) (*Library, error) {
	return s.CreateLibraryWithOptions(ctx, name, typ, language, paths, 0, true)
}

// CreateLibraryWithOptions inserts a library and its paths.
func (s *Store) CreateLibraryWithOptions(ctx context.Context, name, typ, language string, paths []string, autoScanMinutes int, watchEnabled bool) (*Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, wrap("create library", errInvalid("name is required"))
	}
	if !validLibraryType(typ) {
		return nil, wrap("create library", errInvalid("invalid library type"))
	}
	if language == "" {
		language = "en-US"
	}
	if len(paths) == 0 {
		return nil, wrap("create library", errInvalid("at least one path is required"))
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback()

	if autoScanMinutes < 0 {
		autoScanMinutes = 0
	}
	watch := 0
	if watchEnabled {
		watch = 1
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO libraries (name, type, language, auto_scan_minutes, watch_enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		name, typ, language, autoScanMinutes, watch, ts, ts)
	if err != nil {
		if sqliteErrIsUnique(err) {
			return nil, wrap("create library", errInvalid("a library with that name already exists"))
		}
		return nil, wrap("insert library", err)
	}
	id, _ := res.LastInsertId()
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO library_paths (library_id, path, recursive) VALUES (?,?,1)`, id, p); err != nil {
			return nil, wrap("insert library path", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap("commit", err)
	}
	return s.GetLibrary(ctx, id)
}

// UpdateLibrary renames a library, replaces its paths and/or sets the auto-scan interval.
func (s *Store) UpdateLibrary(ctx context.Context, id int64, name, language string, paths []string, autoScanMinutes *int, watchEnabled *bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT id FROM libraries WHERE id = ?`, id); err != nil {
		return wrap("get library", err)
	}
	var cur struct {
		name, lang string
		autoScan   int
		watch      int
	}
	if err := tx.QueryRowContext(ctx, `SELECT name, language, auto_scan_minutes, watch_enabled FROM libraries WHERE id = ?`, id).
		Scan(&cur.name, &cur.lang, &cur.autoScan, &cur.watch); err != nil {
		return wrap("get library", err)
	}
	if autoScanMinutes != nil {
		v := *autoScanMinutes
		if v < 0 {
			v = 0
		}
		cur.autoScan = v
	}
	if strings.TrimSpace(name) != "" {
		cur.name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(language) != "" {
		cur.lang = strings.TrimSpace(language)
	}
	if watchEnabled != nil {
		if *watchEnabled {
			cur.watch = 1
		} else {
			cur.watch = 0
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE libraries SET name = ?, language = ?, auto_scan_minutes = ?, watch_enabled = ?, updated_at = ? WHERE id = ?`,
		cur.name, cur.lang, cur.autoScan, cur.watch, now(), id); err != nil {
		if sqliteErrIsUnique(err) {
			return wrap("update library", errInvalid("a library with that name already exists"))
		}
		return wrap("update library", err)
	}
	if paths != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM library_paths WHERE library_id = ?`, id); err != nil {
			return wrap("clear paths", err)
		}
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO library_paths (library_id, path, recursive) VALUES (?,?,1)`, id, p); err != nil {
				return wrap("insert library path", err)
			}
		}
	}
	return wrap("commit", tx.Commit())
}

// DeleteLibrary removes a library and everything derived from it.
func (s *Store) DeleteLibrary(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id FROM media_items WHERE library_id = ?`, id)
	if err != nil {
		return wrap("list items", err)
	}
	var ids []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return wrap("scan item id", err)
		}
		ids = append(ids, v)
	}
	rows.Close()
	for _, itemID := range ids {
		if err := deleteFTS(ctx, tx, itemID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id); err != nil {
		return wrap("delete library", err)
	}
	return wrap("commit", tx.Commit())
}

// MarkScanned records that a library path was just scanned.
func (s *Store) MarkScanned(ctx context.Context, pathID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE library_paths SET last_scan_at = ? WHERE id = ?`, now(), pathID)
	return wrap("mark scanned", err)
}

type errInvalid string

func (e errInvalid) Error() string { return string(e) }

// IsInvalid reports whether err is a validation error (HTTP 400).
func IsInvalid(err error) bool {
	var e errInvalid
	return err != nil && (asErr(err, &e))
}

func asErr[T error](err error, target *T) bool {
	for err != nil {
		if v, ok := err.(T); ok {
			*target = v
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// Invalid builds a validation error surfaced to API clients as HTTP 400.
func Invalid(msg string) error { return errInvalid(msg) }
