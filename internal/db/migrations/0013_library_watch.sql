-- 0013: per-library filesystem watcher toggle (default on for local mounts).

ALTER TABLE libraries ADD COLUMN watch_enabled INTEGER NOT NULL DEFAULT 1;
