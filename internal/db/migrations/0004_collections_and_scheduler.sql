-- 0004: collections, auto-scan scheduling

CREATE TABLE collections (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL UNIQUE,
    system_key TEXT    UNIQUE, -- favorites | watchlist
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

CREATE TABLE item_collections (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    item_id       INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    added_at      TEXT    NOT NULL,
    PRIMARY KEY (collection_id, item_id)
);

CREATE INDEX idx_item_collections_item ON item_collections(item_id);

INSERT INTO collections (name, system_key, created_at, updated_at) VALUES
    ('Favorites', 'favorites', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ('Watchlist', 'watchlist', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'));

ALTER TABLE libraries ADD COLUMN auto_scan_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE libraries ADD COLUMN last_auto_scan_at TEXT;

CREATE INDEX idx_items_year ON media_items(library_id, kind, year);
