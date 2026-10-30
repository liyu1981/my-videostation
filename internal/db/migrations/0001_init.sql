-- 0001_init.sql — MyVideoStation initial schema (M1)
--
-- Conventions:
--   * timestamps are RFC3339 UTC strings (created in Go, not by SQLite defaults)
--   * parent_id = 0 means "no parent" so unique indexes work without NULL tricks
--   * media_items is a single self-referencing table (movie|show|season|episode|home_video|extra)

CREATE TABLE libraries (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    type        TEXT    NOT NULL CHECK (type IN ('movie','tv','home_video','tv_recording')),
    language    TEXT    NOT NULL DEFAULT 'en-US',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE library_paths (
    id           INTEGER PRIMARY KEY,
    library_id   INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path         TEXT    NOT NULL,
    recursive    INTEGER NOT NULL DEFAULT 1,
    last_scan_at TEXT,
    UNIQUE (library_id, path)
);

CREATE TABLE media_items (
    id             INTEGER PRIMARY KEY,
    library_id     INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    parent_id      INTEGER NOT NULL DEFAULT 0,
    kind           TEXT    NOT NULL CHECK (kind IN ('movie','show','season','episode','home_video','extra')),
    title          TEXT    NOT NULL,
    sort_title     TEXT    NOT NULL DEFAULT '',
    original_title TEXT,
    year           INTEGER,
    overview       TEXT,
    tagline        TEXT,
    runtime_ms     INTEGER,
    rating         REAL,
    content_rating TEXT,
    poster_path    TEXT,
    backdrop_path  TEXT,
    provider       TEXT,
    provider_id    TEXT,
    season_number  INTEGER,
    episode_number INTEGER,
    air_date       TEXT,
    added_at       TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL,
    locked         INTEGER NOT NULL DEFAULT 0,
    hidden         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_items_library_kind ON media_items(library_id, kind, added_at DESC);
CREATE INDEX idx_items_parent       ON media_items(parent_id);
CREATE UNIQUE INDEX idx_items_movie_key
    ON media_items(library_id, kind, title, year) WHERE kind IN ('movie','home_video');
CREATE UNIQUE INDEX idx_items_show_key
    ON media_items(library_id, title) WHERE kind = 'show';
CREATE UNIQUE INDEX idx_items_season_key
    ON media_items(parent_id, season_number) WHERE kind = 'season';
CREATE UNIQUE INDEX idx_items_episode_key
    ON media_items(parent_id, season_number, episode_number) WHERE kind = 'episode';

CREATE TABLE media_files (
    id               INTEGER PRIMARY KEY,
    library_id       INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    item_id          INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    path             TEXT    NOT NULL UNIQUE,
    rel_path         TEXT    NOT NULL DEFAULT '',
    size             INTEGER NOT NULL DEFAULT 0,
    mtime            INTEGER NOT NULL DEFAULT 0,
    inode            INTEGER NOT NULL DEFAULT 0,
    container        TEXT,
    duration_ms      INTEGER,
    bitrate          INTEGER,
    width            INTEGER,
    height           INTEGER,
    video_codec      TEXT,
    hdr              TEXT,
    audio_streams    TEXT    NOT NULL DEFAULT '[]',
    subtitle_streams TEXT    NOT NULL DEFAULT '[]',
    probed_at        TEXT,
    missing          INTEGER NOT NULL DEFAULT 0,
    added_at         TEXT    NOT NULL
);

CREATE INDEX idx_files_item    ON media_files(item_id);
CREATE INDEX idx_files_library ON media_files(library_id);

CREATE TABLE people (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    provider     TEXT,
    provider_id  TEXT,
    profile_path TEXT
);

CREATE TABLE item_people (
    item_id   INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role      TEXT    NOT NULL CHECK (role IN ('cast','director','writer','producer')),
    character TEXT,
    ordering  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (item_id, person_id, role)
);

CREATE TABLE genres (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE item_genres (
    item_id  INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    genre_id INTEGER NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, genre_id)
);

-- per-field metadata locks (item-level lock lives on media_items.locked)
CREATE TABLE item_locks (
    item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    field   TEXT    NOT NULL,
    PRIMARY KEY (item_id, field)
);

CREATE TABLE users (
    id             INTEGER PRIMARY KEY,
    username       TEXT    NOT NULL UNIQUE,
    display_name   TEXT    NOT NULL DEFAULT '',
    role           TEXT    NOT NULL DEFAULT 'admin',
    rating_ceiling TEXT,
    created_at     TEXT    NOT NULL
);

INSERT INTO users (id, username, display_name, role, created_at)
VALUES (1, 'admin', 'Admin', 'admin', strftime('%Y-%m-%dT%H:%M:%SZ','now'));

CREATE TABLE playback_progress (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id     INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    position_ms INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    played      INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT    NOT NULL,
    PRIMARY KEY (user_id, item_id)
);

CREATE TABLE jobs (
    id          INTEGER PRIMARY KEY,
    type        TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed','canceled')),
    progress    REAL    NOT NULL DEFAULT 0,
    message     TEXT    NOT NULL DEFAULT '',
    error       TEXT    NOT NULL DEFAULT '',
    payload     TEXT    NOT NULL DEFAULT '{}',
    created_at  TEXT    NOT NULL,
    started_at  TEXT,
    finished_at TEXT
);

CREATE INDEX idx_jobs_status ON jobs(status, id DESC);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- full-text search; rowid mirrors media_items.id (maintained by the store)
CREATE VIRTUAL TABLE media_fts USING fts5(
    title,
    original_title,
    overview,
    people,
    tokenize = 'unicode61 remove_diacritics 2'
);
