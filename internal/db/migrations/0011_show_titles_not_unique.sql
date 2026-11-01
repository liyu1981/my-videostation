-- 0011: show titles are no longer unique within a library.
-- Matching/editing a show may temporarily create a same-title item; the UI then
-- offers to merge the two shows (seasons/episodes) or keep them separate.

DROP INDEX IF EXISTS idx_items_show_key;
