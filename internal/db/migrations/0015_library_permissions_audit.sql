-- 0015: per-library permissions and an admin activity log.

CREATE TABLE library_permissions (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    can_view   INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, library_id)
);
CREATE INDEX idx_library_permissions_library ON library_permissions(library_id);

CREATE TABLE audit_events (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username   TEXT    NOT NULL DEFAULT '',
    action     TEXT    NOT NULL,
    target     TEXT    NOT NULL DEFAULT '',
    detail     TEXT    NOT NULL DEFAULT '',
    ip         TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_audit_events_created ON audit_events(created_at DESC, id DESC);
CREATE INDEX idx_audit_events_user ON audit_events(user_id, created_at DESC);
