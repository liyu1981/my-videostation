package store

import "context"

// ArtRef points at one item's artwork files (empty when absent).
type ArtRef struct {
	ItemID       int64
	PosterPath   string
	BackdropPath string
}

// ItemsWithArt lists every item that has a cached/local poster or backdrop.
// Used by the thumbnail rebuild job; it is unscoped on purpose (admin job).
func (s *Store) ItemsWithArt(ctx context.Context) ([]ArtRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(poster_path,''), COALESCE(backdrop_path,'')
		FROM media_items
		WHERE COALESCE(poster_path,'') <> '' OR COALESCE(backdrop_path,'') <> ''
		ORDER BY id`)
	if err != nil {
		return nil, wrap("items with art", err)
	}
	defer rows.Close()
	out := []ArtRef{}
	for rows.Next() {
		var a ArtRef
		if err := rows.Scan(&a.ItemID, &a.PosterPath, &a.BackdropPath); err != nil {
			return nil, wrap("scan art ref", err)
		}
		out = append(out, a)
	}
	return out, wrap("iterate art refs", rows.Err())
}
