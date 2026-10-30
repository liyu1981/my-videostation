package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Library is a media library with its configured paths.
type Library struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Language  string        `json:"language"`
	Paths     []LibraryPath `json:"paths"`
	ItemCount int64         `json:"itemCount"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.name, l.type, l.language, l.created_at, l.updated_at,
		       (SELECT COUNT(*) FROM media_items m
		         WHERE m.library_id = l.id AND m.kind IN ('movie','show','home_video')) AS item_count
		FROM libraries l ORDER BY l.name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap("list libraries", err)
	}
	defer rows.Close()

	var out []Library
	for rows.Next() {
		var l Library
		var created, updated string
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.Language, &created, &updated, &l.ItemCount); err != nil {
			return nil, wrap("scan library", err)
		}
		l.CreatedAt, l.UpdatedAt = parseTime(created), parseTime(updated)
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
	err := s.db.QueryRowContext(ctx, `
		SELECT l.id, l.name, l.type, l.language, l.created_at, l.updated_at,
		       (SELECT COUNT(*) FROM media_items m
		         WHERE m.library_id = l.id AND m.kind IN ('movie','show','home_video'))
		FROM libraries l WHERE l.id = ?`, id).
		Scan(&l.ID, &l.Name, &l.Type, &l.Language, &created, &updated, &l.ItemCount)
	if err != nil {
		return nil, wrap("get library", err)
	}
	l.CreatedAt, l.UpdatedAt = parseTime(created), parseTime(updated)
	if l.Paths, err = s.libraryPaths(ctx, id); err != nil {
		return nil, err
	}
	return &l, nil
}

// CreateLibrary inserts a library and its paths.
func (s *Store) CreateLibrary(ctx context.Context, name, typ, language string, paths []string) (*Library, error) {
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

	res, err := tx.ExecContext(ctx,
		`INSERT INTO libraries (name, type, language, created_at, updated_at) VALUES (?,?,?,?,?)`,
		name, typ, language, ts, ts)
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

// UpdateLibrary renames a library and/or replaces its paths.
func (s *Store) UpdateLibrary(ctx context.Context, id int64, name, language string, paths []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT id FROM libraries WHERE id = ?`, id); err != nil {
		return wrap("get library", err)
	}
	var cur struct{ name, lang string }
	if err := tx.QueryRowContext(ctx, `SELECT name, language FROM libraries WHERE id = ?`, id).
		Scan(&cur.name, &cur.lang); err != nil {
		return wrap("get library", err)
	}
	if strings.TrimSpace(name) != "" {
		cur.name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(language) != "" {
		cur.lang = strings.TrimSpace(language)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE libraries SET name = ?, language = ?, updated_at = ? WHERE id = ?`,
		cur.name, cur.lang, now(), id); err != nil {
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
