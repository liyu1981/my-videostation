-- 0018: per-library indexing rules (GAP-005).
-- Globs are JSON string arrays; they are matched relative to each library path.

ALTER TABLE libraries ADD COLUMN exclude_globs TEXT NOT NULL DEFAULT '[]';
ALTER TABLE libraries ADD COLUMN include_globs TEXT NOT NULL DEFAULT '[]';
ALTER TABLE libraries ADD COLUMN min_file_size_mb INTEGER NOT NULL DEFAULT 0;
ALTER TABLE libraries ADD COLUMN skip_extras INTEGER NOT NULL DEFAULT 0;
