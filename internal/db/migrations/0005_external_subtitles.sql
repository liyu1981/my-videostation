-- 0005: sidecar subtitles discovered next to media files

CREATE TABLE external_subtitles (
    id       INTEGER PRIMARY KEY,
    file_id  INTEGER NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    path     TEXT    NOT NULL,
    language TEXT    NOT NULL DEFAULT '',
    label    TEXT    NOT NULL DEFAULT '',
    format   TEXT    NOT NULL,
    UNIQUE (file_id, path)
);

CREATE INDEX idx_external_subtitles_file ON external_subtitles(file_id);
