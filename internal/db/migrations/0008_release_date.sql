-- 0008: release/premiere date from sidecars (NFO <premiered>/<aired>, .vsmeta field 6)

ALTER TABLE media_items ADD COLUMN release_date TEXT;
