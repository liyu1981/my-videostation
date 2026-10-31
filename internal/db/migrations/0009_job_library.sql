-- 0009: associate jobs with a library so the Libraries page can show per-library tasks

ALTER TABLE jobs ADD COLUMN library_id INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_jobs_library ON jobs(library_id, status, id DESC);
