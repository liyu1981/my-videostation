package store

import (
	"context"
	"time"
)

// Job is a durable background task.
type Job struct {
	ID         int64      `json:"id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	Progress   float64    `json:"progress"`
	Message    string     `json:"message,omitempty"`
	Error      string     `json:"error,omitempty"`
	Payload    string     `json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

const jobColumns = `id, type, status, progress, message, error, payload, created_at, started_at, finished_at`

func scanJob(r rowScanner, j *Job) error {
	var created string
	var started, finished, payload *string
	if err := r.Scan(&j.ID, &j.Type, &j.Status, &j.Progress, &j.Message, &j.Error,
		&payload, &created, &started, &finished); err != nil {
		return err
	}
	j.CreatedAt = parseTime(created)
	if started != nil {
		t := parseTime(*started)
		j.StartedAt = &t
	}
	if finished != nil {
		t := parseTime(*finished)
		j.FinishedAt = &t
	}
	if payload != nil {
		j.Payload = *payload
	}
	return nil
}

// CreateJob inserts a queued job.
func (s *Store) CreateJob(ctx context.Context, typ, payload string) (*Job, error) {
	if payload == "" {
		payload = "{}"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (type, status, payload, created_at) VALUES (?, 'queued', ?, ?)`,
		typ, payload, now())
	if err != nil {
		return nil, wrap("create job", err)
	}
	id, _ := res.LastInsertId()
	return s.GetJob(ctx, id)
}

// GetJob loads one job.
func (s *Store) GetJob(ctx context.Context, id int64) (*Job, error) {
	var j Job
	if err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id), &j); err != nil {
		return nil, wrap("get job", err)
	}
	return &j, nil
}

// ListJobs returns the most recent jobs.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+jobColumns+` FROM jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, wrap("list jobs", err)
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		if err := scanJob(rows, &j); err != nil {
			return nil, wrap("scan job", err)
		}
		out = append(out, j)
	}
	return out, wrap("iterate jobs", rows.Err())
}

// StartJob marks a job as running.
func (s *Store) StartJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'running', started_at = ?, progress = 0 WHERE id = ?`, now(), id)
	return wrap("start job", err)
}

// UpdateJob records progress and a human-readable message.
func (s *Store) UpdateJob(ctx context.Context, id int64, progress float64, message string) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET progress = ?, message = ? WHERE id = ?`, progress, message, id)
	return wrap("update job", err)
}

// FinishJob marks a job terminal.
func (s *Store) FinishJob(ctx context.Context, id int64, status, errMsg, message string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, error = ?, message = COALESCE(NULLIF(?,''), message),
		 finished_at = ?, progress = CASE WHEN ? = 'succeeded' THEN 100 ELSE progress END
		 WHERE id = ?`,
		status, errMsg, message, now(), status, id)
	return wrap("finish job", err)
}

// QueuedJobs returns jobs that were queued but not started (used on boot).
func (s *Store) QueuedJobs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs WHERE status = 'queued' ORDER BY id`)
	if err != nil {
		return nil, wrap("queued jobs", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, wrap("iterate queued jobs", rows.Err())
}

// FailStaleJobs marks jobs left running by a previous process as failed.
func (s *Store) FailStaleJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'failed', error = 'interrupted by restart', finished_at = ?
		WHERE status = 'running'`, now())
	return wrap("fail stale jobs", err)
}
