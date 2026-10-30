-- 0002: multi-episode files + extras indexing

ALTER TABLE media_items ADD COLUMN episode_end INTEGER;

CREATE INDEX idx_items_extra ON media_items(parent_id, kind) WHERE kind = 'extra';
