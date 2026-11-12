package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// TranscodeVariant is one cached output of a background transcode job (ADR 0006).
type TranscodeVariant struct {
	ID         int64      `json:"id"`
	FileID     int64      `json:"fileId"`
	ItemID     int64      `json:"itemId"`
	Mtime      int64      `json:"mtime"`
	Size       int64      `json:"size"`
	Key        string     `json:"variantKey"`
	Path       string     `json:"-"`
	Bytes      int64      `json:"bytes"`
	DurationMS int64      `json:"durationMs"`
	State      string     `json:"state"` // queued|running|ready|failed
	JobID      int64      `json:"jobId,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

const variantColumns = `id, file_id, item_id, mtime, size, variant_key, path, bytes,
	duration_ms, state, job_id, error, created_at, updated_at, last_used_at`

func scanVariant(row rowScanner) (*TranscodeVariant, error) {
	var v TranscodeVariant
	var created, updated string
	var lastUsed sql.NullString
	if err := row.Scan(&v.ID, &v.FileID, &v.ItemID, &v.Mtime, &v.Size, &v.Key, &v.Path, &v.Bytes,
		&v.DurationMS, &v.State, &v.JobID, &v.Error, &created, &updated, &lastUsed); err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	if lastUsed.Valid {
		if t, err := time.Parse(time.RFC3339, lastUsed.String); err == nil {
			v.LastUsedAt = &t
		}
	}
	return &v, nil
}

// VariantByKey returns the cached row for (file, profile), if any.
func (s *Store) VariantByKey(ctx context.Context, fileID int64, key string) (*TranscodeVariant, error) {
	v, err := scanVariant(s.db.QueryRowContext(ctx,
		`SELECT `+variantColumns+` FROM transcode_variants WHERE file_id = ? AND variant_key = ?`,
		fileID, key))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrap("variant by key", err)
	}
	return v, nil
}

// VariantByID returns one cached row.
func (s *Store) VariantByID(ctx context.Context, id int64) (*TranscodeVariant, error) {
	v, err := scanVariant(s.db.QueryRowContext(ctx,
		`SELECT `+variantColumns+` FROM transcode_variants WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrap("variant by id", err)
	}
	return v, nil
}

// EnsureVariant inserts a queued row if the (file, profile) pair is new and
// returns the row either way. It does not change an existing ready row.
func (s *Store) EnsureVariant(ctx context.Context, v TranscodeVariant) (*TranscodeVariant, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transcode_variants
			(file_id, item_id, mtime, size, variant_key, path, state, created_at, updated_at)
		VALUES (?,?,?,?,?,?,'queued',?,?)
		ON CONFLICT(file_id, variant_key) DO NOTHING`,
		v.FileID, v.ItemID, v.Mtime, v.Size, v.Key, v.Path, now(), now())
	if err != nil {
		return nil, wrap("ensure variant", err)
	}
	return s.VariantByKey(ctx, v.FileID, v.Key)
}

// SetVariantState updates a variant after a job starts/finishes.
func (s *Store) SetVariantState(ctx context.Context, id int64, state, path string, bytes, durationMS, jobID int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE transcode_variants SET state = ?, path = ?, bytes = ?, duration_ms = ?,
			job_id = ?, error = ?, updated_at = ?
		WHERE id = ?`,
		state, path, bytes, durationMS, jobID, errMsg, now(), id)
	return wrap("set variant state", err)
}

// TouchVariant records a successful read for LRU eviction.
func (s *Store) TouchVariant(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transcode_variants SET last_used_at = ? WHERE id = ?`, now(), id)
	return wrap("touch variant", err)
}

// DeleteVariant removes one row (the caller deletes the file).
func (s *Store) DeleteVariant(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM transcode_variants WHERE id = ?`, id)
	return wrap("delete variant", err)
}

// DeleteVariantsForFile removes every cached variant of a file (source changed).
func (s *Store) DeleteVariantsForFile(ctx context.Context, fileID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM transcode_variants WHERE file_id = ?`, fileID)
	return wrap("delete variants for file", err)
}

// VariantCacheBytes returns the total size of ready variants.
func (s *Store) VariantCacheBytes(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT SUM(bytes) FROM transcode_variants WHERE state = 'ready'`).Scan(&n); err != nil {
		return 0, wrap("variant cache bytes", err)
	}
	return n.Int64, nil
}

// EvictVariants deletes least-recently-used ready rows until the cache fits the
// budget. It returns the removed file paths so the caller can unlink them.
func (s *Store) EvictVariants(ctx context.Context, budgetBytes int64) ([]string, error) {
	if budgetBytes <= 0 {
		return nil, nil
	}
	total, err := s.VariantCacheBytes(ctx)
	if err != nil || total <= budgetBytes {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, bytes FROM transcode_variants
		WHERE state = 'ready'
		ORDER BY COALESCE(last_used_at, created_at) ASC, id ASC`)
	if err != nil {
		return nil, wrap("list evictable variants", err)
	}
	type candidate struct {
		id    int64
		path  string
		bytes int64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.path, &c.bytes); err != nil {
			rows.Close()
			return nil, wrap("scan evictable variant", err)
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, wrap("iterate evictable variants", err)
	}
	var removed []string
	for _, c := range candidates {
		if total <= budgetBytes {
			break
		}
		if err := s.DeleteVariant(ctx, c.id); err != nil {
			return removed, err
		}
		removed = append(removed, c.path)
		total -= c.bytes
	}
	return removed, nil
}
