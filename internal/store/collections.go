package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Collection is a named list of items; Favorites and Watchlist are system collections.
type Collection struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SystemKey string    `json:"systemKey,omitempty"`
	ItemCount int       `json:"itemCount"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// System collection keys.
const (
	SystemFavorites = "favorites"
	SystemWatchlist = "watchlist"
)

// ListCollections returns collections with item counts, system collections first.
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.name, COALESCE(c.system_key,''), c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM item_collections ic WHERE ic.collection_id = c.id) AS items
		FROM collections c
		WHERE c.user_id = ?
		ORDER BY CASE WHEN c.system_key IS NULL THEN 1 ELSE 0 END, c.name COLLATE NOCASE`, UserID(ctx))
	if err != nil {
		return nil, wrap("list collections", err)
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		var c Collection
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Name, &c.SystemKey, &created, &updated, &c.ItemCount); err != nil {
			return nil, wrap("scan collection", err)
		}
		c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(updated)
		out = append(out, c)
	}
	return out, wrap("iterate collections", rows.Err())
}

// GetCollection loads one collection.
func (s *Store) GetCollection(ctx context.Context, id int64) (*Collection, error) {
	var c Collection
	var created, updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.name, COALESCE(c.system_key,''), c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM item_collections ic WHERE ic.collection_id = c.id)
		FROM collections c WHERE c.id = ? AND c.user_id = ?`, id, UserID(ctx)).
		Scan(&c.ID, &c.Name, &c.SystemKey, &created, &updated, &c.ItemCount)
	if err != nil {
		return nil, wrap("get collection", err)
	}
	c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(updated)
	return &c, nil
}

// CreateCollection adds a user collection.
func (s *Store) CreateCollection(ctx context.Context, name string) (*Collection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, wrap("create collection", errInvalid("name is required"))
	}
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO collections (user_id, name, created_at, updated_at) VALUES (?,?,?,?)`,
		UserID(ctx), name, ts, ts)
	if err != nil {
		if sqliteErrIsUnique(err) {
			return nil, wrap("create collection", errInvalid("a collection with that name already exists"))
		}
		return nil, wrap("create collection", err)
	}
	id, _ := res.LastInsertId()
	return s.GetCollection(ctx, id)
}

// RenameCollection renames a user collection (system collections are fixed).
func (s *Store) RenameCollection(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return wrap("rename collection", errInvalid("name is required"))
	}
	c, err := s.GetCollection(ctx, id)
	if err != nil {
		return err
	}
	if c.SystemKey != "" {
		return wrap("rename collection", errInvalid("system collections cannot be renamed"))
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE collections SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		name, now(), id, UserID(ctx)); err != nil {
		if sqliteErrIsUnique(err) {
			return wrap("rename collection", errInvalid("a collection with that name already exists"))
		}
		return wrap("rename collection", err)
	}
	return nil
}

// DeleteCollection removes a user collection (items are untouched).
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	c, err := s.GetCollection(ctx, id)
	if err != nil {
		return err
	}
	if c.SystemKey != "" {
		return wrap("delete collection", errInvalid("system collections cannot be deleted"))
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM collections WHERE id = ? AND user_id = ?`, id, UserID(ctx)); err != nil {
		return wrap("delete collection", err)
	}
	return nil
}

// SystemCollection returns the id of a system collection, creating it if needed.
func (s *Store) SystemCollection(ctx context.Context, key string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM collections WHERE user_id = ? AND system_key = ?`, UserID(ctx), key).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, wrap("system collection", err)
	}
	name := map[string]string{SystemFavorites: "Favorites", SystemWatchlist: "Watchlist"}[key]
	if name == "" {
		return 0, wrap("system collection", errInvalid("unknown system collection"))
	}
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO collections (user_id, name, system_key, created_at, updated_at)
		VALUES (?,?,?,?,?)`, UserID(ctx), name, key, ts, ts)
	if err != nil {
		return 0, wrap("create system collection", err)
	}
	if id, _ := res.LastInsertId(); id > 0 {
		return id, nil
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM collections WHERE user_id = ? AND system_key = ?`, UserID(ctx), key).Scan(&id); err != nil {
		return 0, wrap("load system collection", err)
	}
	return id, nil
}

// AddToCollection inserts items (idempotent).
func (s *Store) AddToCollection(ctx context.Context, collectionID int64, itemIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}
	if _, err := s.GetCollection(ctx, collectionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()
	for _, id := range itemIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO item_collections (collection_id, item_id, added_at) VALUES (?,?,?)`,
			collectionID, id, now()); err != nil {
			return wrap("add to collection", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collections SET updated_at = ? WHERE id = ? AND user_id = ?`,
		now(), collectionID, UserID(ctx)); err != nil {
		return wrap("touch collection", err)
	}
	return wrap("commit", tx.Commit())
}

// RemoveFromCollection removes one item.
func (s *Store) RemoveFromCollection(ctx context.Context, collectionID, itemID int64) error {
	if _, err := s.GetCollection(ctx, collectionID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM item_collections WHERE collection_id = ? AND item_id = ?`, collectionID, itemID); err != nil {
		return wrap("remove from collection", err)
	}
	return nil
}

// ToggleCollectionItem flips membership and reports the new state.
func (s *Store) ToggleCollectionItem(ctx context.Context, collectionID, itemID int64) (bool, error) {
	var exists int
	if _, err := s.GetCollection(ctx, collectionID); err != nil {
		return false, err
	}
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM item_collections WHERE collection_id = ? AND item_id = ?`, collectionID, itemID).Scan(&exists)
	switch {
	case err == nil:
		return false, s.RemoveFromCollection(ctx, collectionID, itemID)
	case errors.Is(err, sql.ErrNoRows):
		return true, s.AddToCollection(ctx, collectionID, []int64{itemID})
	default:
		return false, wrap("toggle collection item", err)
	}
}

// ItemCollectionIDs lists collections containing an item.
func (s *Store) ItemCollectionIDs(ctx context.Context, itemID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ic.collection_id FROM item_collections ic
		JOIN collections c ON c.id = ic.collection_id
		WHERE ic.item_id = ? AND c.user_id = ?
		ORDER BY ic.collection_id`, itemID, UserID(ctx))
	if err != nil {
		return nil, wrap("item collections", err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("scan item collection", err)
		}
		out = append(out, id)
	}
	return out, wrap("iterate item collections", rows.Err())
}

// ListCollectionItems pages through a collection.
func (s *Store) ListCollectionItems(ctx context.Context, collectionID int64, page, perPage int) ([]Item, int, error) {
	if perPage <= 0 || perPage > 200 {
		perPage = 60
	}
	if page <= 0 {
		page = 1
	}
	if _, err := s.GetCollection(ctx, collectionID); err != nil {
		return nil, 0, err
	}
	scope, scopeArgs := accessScope(ctx, "m")
	var total int
	countArgs := append([]any{collectionID}, scopeArgs...)
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM item_collections ic
		JOIN media_items m ON m.id = ic.item_id
		WHERE ic.collection_id = ? AND m.hidden = 0`+scope, countArgs...).Scan(&total); err != nil {
		return nil, 0, wrap("count collection items", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+itemColumns+`, `+itemStateColumns+`
		FROM media_items m
		JOIN item_collections ic ON ic.item_id = m.id
		`+fmt.Sprintf(itemJoin, UserID(ctx))+`
		WHERE ic.collection_id = ? AND m.hidden = 0`+scope+`
		ORDER BY ic.added_at DESC, m.id DESC LIMIT ? OFFSET ?`,
		append(append([]any{collectionID}, scopeArgs...), perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, wrap("collection items", err)
	}
	var out []Item
	if err := collectItems(rows, &out); err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []Item{}
	}
	return out, total, nil
}

// MarkPlayed sets the watched flag for many items at once.
func (s *Store) MarkPlayed(ctx context.Context, itemIDs []int64, played bool) error {
	if len(itemIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()
	p := 0
	if played {
		p = 1
	}
	for _, id := range itemIDs {
		var duration int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(runtime_ms,0) FROM media_items WHERE id = ?`, id).Scan(&duration); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return wrap("bulk progress duration", err)
		}
		position := int64(0)
		if played {
			position = duration
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO playback_progress (user_id, item_id, position_ms, duration_ms, played, updated_at)
			VALUES (?,?,?,?,?,?)
			ON CONFLICT(user_id, item_id) DO UPDATE SET
				position_ms = excluded.position_ms, duration_ms = excluded.duration_ms,
				played = excluded.played, updated_at = excluded.updated_at`,
			UserID(ctx), id, position, duration, p, now()); err != nil {
			return wrap("bulk progress", err)
		}
	}
	return wrap("commit", tx.Commit())
}

// Folder is one directory (relative to a library root) and its item count.
type Folder struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// Folders aggregates media files by their parent directory.
func (s *Store) Folders(ctx context.Context, libraryID int64) ([]Folder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT rel_path FROM media_files WHERE library_id = ? AND missing = 0`, libraryID)
	if err != nil {
		return nil, wrap("folders", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			return nil, wrap("scan rel path", err)
		}
		dir := ""
		if i := strings.LastIndex(rel, "/"); i >= 0 {
			dir = rel[:i]
		}
		counts[dir]++
	}
	out := make([]Folder, 0, len(counts))
	for path, count := range counts {
		out = append(out, Folder{Path: path, Count: count})
	}
	sortFolders(out)
	return out, wrap("iterate folders", rows.Err())
}

// Facet is a filter value with a count. ID is populated for genres (year
// facets have no stable row id).
type Facet struct {
	ID    int64  `json:"id,omitempty"`
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Facets groups genres and years for a library/kind.
type Facets struct {
	Genres []Facet `json:"genres"`
	Years  []Facet `json:"years"`
}

// Facets computes available filter values for a library.
func (s *Store) Facets(ctx context.Context, libraryID int64, kind string) (*Facets, error) {
	var kinds []string
	if kind != "" && kind != "all" {
		kinds = []string{kind}
	}
	return s.FacetsForKinds(ctx, libraryID, kinds)
}

// FacetsForKinds computes facets across one or more item kinds (empty = all).
func (s *Store) FacetsForKinds(ctx context.Context, libraryID int64, kinds []string) (*Facets, error) {
	out := &Facets{Genres: []Facet{}, Years: []Facet{}}
	libClause, libArgs := "", []any{}
	if libraryID > 0 {
		libClause = " AND m.library_id = ?"
		libArgs = append(libArgs, libraryID)
	}
	kindClause, kindArgs := "", []any{}
	if len(kinds) > 0 {
		kindClause = " AND m.kind IN (" + placeholders(len(kinds)) + ")"
		for _, k := range kinds {
			kindArgs = append(kindArgs, k)
		}
	}
	args := append(append([]any{}, kindArgs...), libArgs...)
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.name, COUNT(*) FROM item_genres ig
		JOIN genres g ON g.id = ig.genre_id
		JOIN media_items m ON m.id = ig.item_id
		WHERE m.hidden = 0`+kindClause+libClause+`
		GROUP BY g.id, g.name ORDER BY COUNT(*) DESC, g.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, wrap("genre facets", err)
	}
	for rows.Next() {
		var f Facet
		if err := rows.Scan(&f.ID, &f.Value, &f.Count); err != nil {
			rows.Close()
			return nil, wrap("scan genre facet", err)
		}
		out.Genres = append(out.Genres, f)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `
		SELECT year, COUNT(*) FROM media_items m
		WHERE m.year IS NOT NULL AND m.hidden = 0`+kindClause+libClause+`
		GROUP BY year ORDER BY year DESC`, args...)
	if err != nil {
		return nil, wrap("year facets", err)
	}
	defer rows.Close()
	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			return nil, wrap("scan year facet", err)
		}
		out.Years = append(out.Years, Facet{Value: fmt.Sprintf("%d", year), Count: count})
	}
	return out, wrap("iterate year facets", rows.Err())
}

func sortFolders(fs []Folder) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Path < fs[j-1].Path; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}

// LibrariesDueForScan returns libraries whose auto-scan interval has elapsed.
func (s *Store) LibrariesDueForScan(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM libraries
		WHERE auto_scan_minutes > 0
		  AND (last_auto_scan_at IS NULL
		       OR datetime(last_auto_scan_at) <= datetime('now', '-' || auto_scan_minutes || ' minutes'))`)
	if err != nil {
		return nil, wrap("libraries due", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("scan due library", err)
		}
		ids = append(ids, id)
	}
	return ids, wrap("iterate due libraries", rows.Err())
}

// MarkAutoScan records that an automatic scan was started for a library.
func (s *Store) MarkAutoScan(ctx context.Context, libraryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE libraries SET last_auto_scan_at = ? WHERE id = ?`, now(), libraryID)
	return wrap("mark auto scan", err)
}

// HasActiveJob reports whether a queued/running job of this type already targets a library.
func (s *Store) HasActiveJob(ctx context.Context, jobType string, libraryID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE type = ? AND status IN ('queued','running') AND payload LIKE ?`,
		jobType, fmt.Sprintf(`%%"libraryId":%d%%`, libraryID)).Scan(&n)
	if err != nil {
		return false, wrap("active job", err)
	}
	return n > 0, nil
}
