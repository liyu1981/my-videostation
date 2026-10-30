// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps the SQL connection pool.
type DB struct {
	*sql.DB
	Path string
}

// Open opens (and creates) the SQLite database with production pragmas.
func Open(ctx context.Context, path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=mmap_size(134217728)"

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL allows many readers but a single writer; keep the pool small and let
	// busy_timeout absorb contention.
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &DB{DB: sqlDB, Path: path}, nil
}

// Migrate applies embedded migrations in filename order inside transactions.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %q: bad version prefix: %w", name, err)
		}
		if applied[version] {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			version, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
