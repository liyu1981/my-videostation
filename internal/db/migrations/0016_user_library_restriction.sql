-- 0016: distinguish an unrestricted user from an explicitly restricted allow-list.

ALTER TABLE users ADD COLUMN library_restricted INTEGER NOT NULL DEFAULT 0;
UPDATE users SET library_restricted = 1
WHERE id IN (SELECT DISTINCT user_id FROM library_permissions);
