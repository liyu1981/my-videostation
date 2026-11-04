package store

import (
	"context"
	"strings"
)

// PersonRef is one cast/crew member with a visible-item count.
type PersonRef struct {
	ID    int64
	Name  string
	Count int
}

// PersonRefs lists people with visible items, optionally filtered by name.
func (s *Store) PersonRefs(ctx context.Context, query string, limit int) ([]PersonRef, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	scope, scopeArgs := accessScope(ctx, "m")
	where := []string{"m.hidden = 0"}
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		where = append(where, "p.name LIKE ?")
		args = append(args, "%"+q+"%")
	}
	args = append(args, scopeArgs...)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, COUNT(DISTINCT ip.item_id)
		FROM people p
		JOIN item_people ip ON ip.person_id = p.id
		JOIN media_items m ON m.id = ip.item_id
		WHERE `+strings.Join(where, " AND ")+scope+`
		GROUP BY p.id, p.name
		ORDER BY p.name COLLATE NOCASE
		LIMIT ?`, args...)
	if err != nil {
		return nil, wrap("person refs", err)
	}
	defer rows.Close()
	out := []PersonRef{}
	for rows.Next() {
		var p PersonRef
		if err := rows.Scan(&p.ID, &p.Name, &p.Count); err != nil {
			return nil, wrap("scan person ref", err)
		}
		out = append(out, p)
	}
	return out, wrap("iterate person refs", rows.Err())
}

// RatingRefs lists distinct content ratings in a library (0 = all libraries).
func (s *Store) RatingRefs(ctx context.Context, libraryID int64, kind string) ([]string, error) {
	where := []string{"COALESCE(content_rating,'') <> ''", "hidden = 0"}
	args := []any{}
	if libraryID > 0 {
		where = append(where, "library_id = ?")
		args = append(args, libraryID)
	}
	if kind != "" && kind != "all" {
		where = append(where, "kind = ?")
		args = append(args, kind)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT content_rating FROM media_items
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY content_rating COLLATE NOCASE`, args...)
	if err != nil {
		return nil, wrap("rating refs", err)
	}
	return scanStrings(rows)
}

// FavoriteItemIDs reports which of the given items are in the current user's
// favorites system collection.
func (s *Store) FavoriteItemIDs(ctx context.Context, itemIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	args := []any{UserID(ctx)}
	for _, id := range itemIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ic.item_id FROM item_collections ic
		JOIN collections c ON c.id = ic.collection_id
		WHERE c.user_id = ? AND c.system_key = 'favorites'
		  AND ic.item_id IN (`+placeholders(len(itemIDs))+`)`, args...)
	if err != nil {
		return nil, wrap("favorite ids", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("scan favorite id", err)
		}
		out[id] = true
	}
	return out, wrap("iterate favorite ids", rows.Err())
}

// SeriesRef carries the series/season context of an episode.
type SeriesRef struct {
	SeriesID     int64
	SeasonID     int64
	SeriesName   string
	SeasonName   string
	PosterPath   string
	BackdropPath string
}

// SeriesRefsForEpisodes batches the parent chain for a set of episodes.
func (s *Store) SeriesRefsForEpisodes(ctx context.Context, episodeIDs []int64) (map[int64]SeriesRef, error) {
	out := map[int64]SeriesRef{}
	if len(episodeIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(episodeIDs))
	for _, id := range episodeIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, sh.id, s.id, COALESCE(sh.title,''), COALESCE(s.title,''),
		       COALESCE(sh.poster_path,''), COALESCE(sh.backdrop_path,'')
		FROM media_items e
		JOIN media_items s ON s.id = e.parent_id AND s.kind = 'season'
		JOIN media_items sh ON sh.id = s.parent_id AND sh.kind = 'show'
		WHERE e.id IN (`+placeholders(len(episodeIDs))+`)`, args...)
	if err != nil {
		return nil, wrap("series refs", err)
	}
	defer rows.Close()
	for rows.Next() {
		var epID int64
		var r SeriesRef
		if err := rows.Scan(&epID, &r.SeriesID, &r.SeasonID, &r.SeriesName, &r.SeasonName, &r.PosterPath, &r.BackdropPath); err != nil {
			return nil, wrap("scan series ref", err)
		}
		out[epID] = r
	}
	return out, wrap("iterate series refs", rows.Err())
}

// ChildCounts returns the number of visible children per item id.
func (s *Store) ChildCounts(ctx context.Context, itemIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(itemIDs))
	for _, id := range itemIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT parent_id, COUNT(*) FROM media_items
		WHERE hidden = 0 AND parent_id IN (`+placeholders(len(itemIDs))+`)
		GROUP BY parent_id`, args...)
	if err != nil {
		return nil, wrap("child counts", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, wrap("scan child count", err)
		}
		out[id] = n
	}
	return out, wrap("iterate child counts", rows.Err())
}
