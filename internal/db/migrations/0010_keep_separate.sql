-- 0010: let the user keep two same-title items separate (skip automatic part merging)

ALTER TABLE media_items ADD COLUMN keep_separate INTEGER NOT NULL DEFAULT 0;
