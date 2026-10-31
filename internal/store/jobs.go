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

// JobLog is one persisted log line for a job.
type JobLog struct {
	ID        int64     `json:"id"`
	JobID     int64     `json:"jobId"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// AppendJobLog stores one log line. Logs are kept until the job is deleted.
func (s *Store) AppendJobLog(ctx context.Context, jobID int64, level, message string) error {
	if jobID <= 0 || message == "" {
		return nil
	}
	switch level {
	case "info", "warn", "error":
	default:
		level = "info"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO job_logs (job_id, level, message, created_at) VALUES (?,?,?,?)`,
		jobID, level, message, now())
	return wrap("append job log", err)
}

// JobLogs returns the newest lines for a job (oldest first).
func (s *Store) JobLogs(ctx context.Context, jobID int64, limit int) ([]JobLog, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, level, message, created_at FROM (
			SELECT id, job_id, level, message, created_at FROM job_logs
			WHERE job_id = ? ORDER BY id DESC LIMIT ?
		) ORDER BY id ASC`, jobID, limit)
	if err != nil {
		return nil, wrap("job logs", err)
	}
	defer rows.Close()
	out := []JobLog{}
	for rows.Next() {
		var l JobLog
		var created string
		if err := rows.Scan(&l.ID, &l.JobID, &l.Level, &l.Message, &created); err != nil {
			return nil, wrap("scan job log", err)
		}
		l.CreatedAt = parseTime(created)
		out = append(out, l)
	}
	return out, wrap("iterate job logs", rows.Err())
}

// PruneJobLogs keeps only the newest keep lines for a job.
func (s *Store) PruneJobLogs(ctx context.Context, jobID int64, keep int) error {
	if keep <= 0 {
		keep = 50000
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM job_logs WHERE job_id = ? AND id NOT IN (
			SELECT id FROM job_logs WHERE job_id = ? ORDER BY id DESC LIMIT ?
		)`, jobID, jobID, keep)
	return wrap("prune job logs", err)
}

// DeleteJob removes a job and its logs. Media and library data are untouched.
func (s *Store) DeleteJob(ctx context.Context, jobID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_logs WHERE job_id = ?`, jobID); err != nil {
		return wrap("delete job logs", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, jobID); err != nil {
		return wrap("delete job", err)
	}
	return wrap("commit", tx.Commit())
}

// ClearFinishedJobs deletes terminal jobs (and their logs); active jobs are kept.
func (s *Store) ClearFinishedJobs(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, wrap("begin", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM job_logs WHERE job_id IN (SELECT id FROM jobs WHERE status NOT IN ('queued','running'))`); err != nil {
		return 0, wrap("delete finished logs", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE status NOT IN ('queued','running')`)
	if err != nil {
		return 0, wrap("delete finished jobs", err)
	}
	n, _ := res.RowsAffected()
	return n, wrap("commit", tx.Commit())
}

// FailStaleJobs marks jobs left running by a previous process as failed.
func (s *Store) FailStaleJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'failed', error = 'interrupted by restart', finished_at = ?
		WHERE status = 'running'`, now())
	return wrap("fail stale jobs", err)
}
