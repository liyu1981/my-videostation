package store

import (
	"context"
	"database/sql"
	"time"
)

// AuditEvent is one persisted admin/user activity entry.
type AuditEvent struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId,omitempty"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	IP        string    `json:"ip,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// RecordAudit appends an activity entry. userID may be zero for failed logins.
func (s *Store) RecordAudit(ctx context.Context, userID int64, username, action, target, detail, ip string) error {
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (user_id, username, action, target, detail, ip, created_at)
		VALUES (?,?,?,?,?,?,?)`, uid, username, action, target, detail, ip, now())
	return wrap("record audit", err)
}

// ListAuditEvents returns the newest activity entries.
func (s *Store) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(user_id,0), username, action, target, detail, ip, created_at
		FROM audit_events ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, wrap("list audit", err)
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var uid sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &uid, &e.Username, &e.Action, &e.Target, &e.Detail, &e.IP, &created); err != nil {
			return nil, wrap("scan audit", err)
		}
		if uid.Valid {
			e.UserID = uid.Int64
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	return out, wrap("iterate audit", rows.Err())
}
