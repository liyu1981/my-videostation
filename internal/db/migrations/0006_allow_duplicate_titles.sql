-- 0006: allow multiple items with the same title/year (editions, qualities,
-- separate folders). Identity is the file path, not the title; merging versions
-- is a future feature. A plain index keeps title lookups fast.
DROP INDEX IF EXISTS idx_items_movie_key;

CREATE INDEX IF NOT EXISTS idx_items_title_year
    ON media_items(library_id, kind, title, year);
