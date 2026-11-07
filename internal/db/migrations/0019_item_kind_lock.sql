-- 0019: manual type assignment. When an admin reclassifies an item (movie,
-- episode, home video), rescans must not move it back to the kind inferred from
-- the filename.
ALTER TABLE media_items ADD COLUMN kind_locked INTEGER NOT NULL DEFAULT 0;
