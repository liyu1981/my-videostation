-- 0003: metadata matching state

ALTER TABLE media_items ADD COLUMN metadata_status TEXT NOT NULL DEFAULT 'unmatched';
ALTER TABLE media_items ADD COLUMN match_confidence REAL NOT NULL DEFAULT 0;
ALTER TABLE media_items ADD COLUMN last_refresh_at TEXT;

CREATE INDEX idx_items_meta_status ON media_items(library_id, metadata_status);
