-- 0007: persisted job logs (scan progress + warnings), viewable in the Tasks UI

CREATE TABLE job_logs (
    id         INTEGER PRIMARY KEY,
    job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    level      TEXT    NOT NULL DEFAULT 'info', -- info | warn | error
    message    TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);

CREATE INDEX idx_job_logs_job ON job_logs(job_id, id);
