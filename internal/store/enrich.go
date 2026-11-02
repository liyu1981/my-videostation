package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Enrichment is a metadata update produced by a provider.
type Enrichment struct {
	Provider      string
	ProviderID    string
	Status        string // matched | manual | unmatched | failed
	Confidence    float64
	Title         *string
	OriginalTitle *string
	Year          *int
	Overview      *string
	Tagline       *string
	Rating        *float64
	ContentRating *string
	ReleaseDate   *string
	RuntimeMS     *int64
	Genres        []string
	Cast          []Person
	Crew          []Person
	PosterPath    *string
	BackdropPath  *string
}

// ApplyEnrichment writes provider metadata, honouring per-field locks.
func (s *Store) ApplyEnrichment(ctx context.Context, itemID int64, e Enrichment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()

	sets := []string{"updated_at = ?"}
	args := []any{now()}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if e.Title != nil {
		add("title", *e.Title)
		add("sort_title", sortTitle(*e.Title))
	}
	if e.OriginalTitle != nil {
		add("original_title", *e.OriginalTitle)
	}
	if e.Year != nil {
		add("year", nullInt(*e.Year))
	}
	if e.Overview != nil {
		add("overview", *e.Overview)
	}
	if e.Tagline != nil {
		add("tagline", *e.Tagline)
	}
	if e.Rating != nil {
		add("rating", *e.Rating)
	}
	if e.ContentRating != nil {
		add("content_rating", *e.ContentRating)
	}
	if e.ReleaseDate != nil {
		add("release_date", nullStr(*e.ReleaseDate))
	}
	if e.RuntimeMS != nil {
		add("runtime_ms", *e.RuntimeMS)
	}
	if e.PosterPath != nil {
		add("poster_path", nullStr(*e.PosterPath))
	}
	if e.BackdropPath != nil {
		add("backdrop_path", nullStr(*e.BackdropPath))
	}
	if e.Provider != "" {
		add("provider", e.Provider)
	}
	if e.ProviderID != "" {
		add("provider_id", e.ProviderID)
	}
	if e.Provider != "" && e.ProviderID != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO item_provider_ids (item_id, provider, provider_id, created_at)
			VALUES (?,?,?,?)
			ON CONFLICT(item_id, provider) DO UPDATE SET provider_id = excluded.provider_id`,
			itemID, e.Provider, e.ProviderID, now()); err != nil {
			return wrap("provider id upsert", err)
		}
	}
	if e.Status != "" {
		add("metadata_status", e.Status)
	}
	add("match_confidence", e.Confidence)
	add("last_refresh_at", now())

	args = append(args, itemID)
	if _, err := tx.ExecContext(ctx,
		`UPDATE media_items SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return wrap("apply enrichment", err)
	}

	if e.Genres != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_genres WHERE item_id = ?`, itemID); err != nil {
			return wrap("clear genres", err)
		}
		for _, name := range e.Genres {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
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

	if e.Cast != nil {
		if err := replaceCredits(ctx, tx, itemID, "cast", e.Cast); err != nil {
			return err
		}
	}
	if e.Crew != nil {
		if err := replaceCredits(ctx, tx, itemID, "crew", e.Crew); err != nil {
			return err
		}
	}

	if err := refreshFTS(ctx, tx, itemID); err != nil {
		return err
	}
	return wrap("commit", tx.Commit())
}

// replaceCredits swaps cast or crew credits for an item.
func replaceCredits(ctx context.Context, tx *sql.Tx, itemID int64, group string, people []Person) error {
	roles := []string{"director", "writer"}
	if group == "cast" {
		roles = []string{"cast"}
	}
	for _, role := range roles {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_people WHERE item_id = ? AND role = ?`, itemID, role); err != nil {
			return wrap("clear credits", err)
		}
	}
	for i, p := range people {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		role := p.Role
		if group == "crew" && role != "writer" {
			role = "director"
		}
		if group == "cast" {
			role = "cast"
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO people (name) VALUES (?)`, name); err != nil {
			return wrap("insert person", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO item_people (item_id, person_id, role, character, ordering)
			SELECT ?, id, ?, ?, ? FROM people WHERE name = ?`,
			itemID, role, nullStr(p.Character), i, name); err != nil {
			return wrap("link person", err)
		}
	}
	return nil
}

// SetArt points an item's poster or backdrop at a cached/local image.
func (s *Store) SetArt(ctx context.Context, itemID int64, kind, path string) error {
	col := "poster_path"
	if kind == "backdrop" {
		col = "backdrop_path"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE media_items SET `+col+` = ?, updated_at = ? WHERE id = ?`, nullStr(path), now(), itemID)
	return wrap("set art", err)
}

// CountUnmatched counts items that still need metadata in a library (0 = all libraries).
func (s *Store) CountUnmatched(ctx context.Context, libraryID int64) (int, error) {
	q := `SELECT COUNT(*) FROM media_items WHERE kind IN ('movie','show') AND metadata_status = 'unmatched'`
	args := []any{}
	if libraryID > 0 {
		q += ` AND library_id = ?`
		args = append(args, libraryID)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, wrap("count unmatched", err)
	}
	return n, nil
}

// UnmatchedItems lists items that still need metadata.
func (s *Store) UnmatchedItems(ctx context.Context, libraryID int64, limit int) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT ` + itemColumns + `, ` + itemStateColumns + `
		FROM media_items m ` + fmt.Sprintf(itemJoin, UserID(ctx)) + `
		WHERE m.kind IN ('movie','show') AND m.metadata_status = 'unmatched' AND m.hidden = 0`
	args := []any{}
	if libraryID > 0 {
		q += ` AND m.library_id = ?`
		args = append(args, libraryID)
	}
	q += ` ORDER BY m.added_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, wrap("unmatched items", err)
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

// ItemProviderID returns the stored provider id for one provider ("" if absent).
func (s *Store) ItemProviderID(ctx context.Context, itemID int64, provider string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT provider_id FROM item_provider_ids WHERE item_id = ? AND provider = ?`, itemID, provider).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", wrap("provider id", err)
	}
	return id, nil
}
