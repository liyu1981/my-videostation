-- 0020: background transcode cache (ADR 0006).
-- One row per (file, variant profile). The output lives under
-- <data>/cache/transcodes/<fileID>/<variantKey>.mp4; media folders stay read-only.
CREATE TABLE transcode_variants (
    id             INTEGER PRIMARY KEY,
    file_id        INTEGER NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    item_id        INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    mtime          INTEGER NOT NULL,
    size           INTEGER NOT NULL,
    variant_key    TEXT NOT NULL,
    path           TEXT NOT NULL,
    bytes          INTEGER NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    state          TEXT NOT NULL DEFAULT 'queued', -- queued|running|ready|failed
    job_id         INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    last_used_at   TEXT,
    UNIQUE(file_id, variant_key)
);
CREATE INDEX idx_transcode_variants_state ON transcode_variants(state);
CREATE INDEX idx_transcode_variants_lru ON transcode_variants(state, last_used_at);
