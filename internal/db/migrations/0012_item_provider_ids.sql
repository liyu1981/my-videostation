-- 0012: keep one provider id per metadata provider/plugin for each item.

CREATE TABLE item_provider_ids (
    item_id     INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    provider    TEXT    NOT NULL,
    provider_id TEXT    NOT NULL,
    created_at  TEXT    NOT NULL,
    PRIMARY KEY (item_id, provider)
);

INSERT OR IGNORE INTO item_provider_ids (item_id, provider, provider_id, created_at)
SELECT id, provider, provider_id, updated_at
FROM media_items
WHERE COALESCE(provider,'') <> '' AND COALESCE(provider_id,'') <> '';
