-- 0014: real accounts, password hashes, sessions, per-user collections.

ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
UPDATE users SET updated_at = created_at WHERE updated_at = '';
CREATE UNIQUE INDEX idx_users_username_nocase ON users(username COLLATE NOCASE);

CREATE TABLE sessions (
    token_hash TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent TEXT    NOT NULL DEFAULT '',
    ip         TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL,
    expires_at TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Collections become per-user. Rebuild both tables to add user_id and replace
-- the global UNIQUE constraints on name/system_key with per-user ones.
DROP INDEX IF EXISTS idx_item_collections_item;
ALTER TABLE item_collections RENAME TO item_collections_old;
ALTER TABLE collections RENAME TO collections_old;

CREATE TABLE collections (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    system_key TEXT,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    UNIQUE(user_id, name),
    UNIQUE(user_id, system_key)
);

INSERT INTO collections (id, user_id, name, system_key, created_at, updated_at)
SELECT id, 1, name, system_key, created_at, updated_at FROM collections_old;

CREATE TABLE item_collections (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    item_id       INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    added_at      TEXT    NOT NULL,
    PRIMARY KEY (collection_id, item_id)
);
CREATE INDEX idx_item_collections_item ON item_collections(item_id);

INSERT INTO item_collections (collection_id, item_id, added_at)
SELECT collection_id, item_id, added_at FROM item_collections_old;

DROP TABLE item_collections_old;
DROP TABLE collections_old;
