// Package store contains the typed data access layer over SQLite.
//
// Queries are hand-written for M1 (schema is still moving); sqlc can be introduced
// once the model stabilises — see docs/PLAN.md.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liyu1981/myvideostation/internal/db"
)

// DefaultUserID is the built-in admin profile (auth arrives in M6).
const DefaultUserID int64 = 1

// Store provides data access.
type Store struct {
	db *db.DB
}

// New wraps a database handle.
func New(d *db.DB) *Store { return &Store{db: d} }

// DB exposes the underlying handle (used by the migrator and health checks).
func (s *Store) DB() *db.DB { return s.db }

// Kind values mirror media_items.kind.
const (
	KindMovie     = "movie"
	KindShow      = "show"
	KindSeason    = "season"
	KindEpisode   = "episode"
	KindHomeVideo = "home_video"
	KindExtra     = "extra"
)

var ErrNotFound = errors.New("not found")

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// scanStrings scans a single-column string result set.
func scanStrings(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// mustJSON marshals a value for storage; it never fails for our types.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// sqliteErrIsUnique reports whether err is a UNIQUE constraint violation.
func sqliteErrIsUnique(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
