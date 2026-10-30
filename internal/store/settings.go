package store

import (
	"context"
	"database/sql"
	"errors"
)

// Settings returns all key/value settings.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, wrap("settings", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, wrap("scan setting", err)
		}
		out[k] = v
	}
	return out, wrap("iterate settings", rows.Err())
}

// Setting reads one value; the bool reports presence.
func (s *Store) Setting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, wrap("get setting", err)
	}
	return v, true, nil
}

// PutSettings upserts a batch of settings.
func (s *Store) PutSettings(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()
	for k, v := range values {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return wrap("put setting", err)
		}
	}
	return wrap("commit", tx.Commit())
}
