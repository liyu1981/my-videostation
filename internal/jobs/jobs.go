// Package jobs runs durable background jobs from the jobs table.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/liyu1981/myvideostation/internal/redact"
	"github.com/liyu1981/myvideostation/internal/store"
)

// Progress reports completion (0-100) and a short status message.
type Progress func(percent float64, message string)

// Handler executes one job type.
type Handler func(ctx context.Context, job *store.Job, report Progress) error

// Manager owns the worker goroutine and the handler registry.
type Manager struct {
	store    *store.Store
	log      *slog.Logger
	handlers map[string]Handler
	queue    chan int64
}

// New creates a manager.
func New(st *store.Store, log *slog.Logger) *Manager {
	return &Manager{
		store:    st,
		log:      log,
		handlers: map[string]Handler{},
		queue:    make(chan int64, 64),
	}
}

// Handle registers a handler for a job type.
func (m *Manager) Handle(jobType string, h Handler) {
	m.handlers[jobType] = h
}

// Enqueue persists a queued job and schedules it.
func (m *Manager) Enqueue(ctx context.Context, jobType string, payload any) (*store.Job, error) {
	if _, ok := m.handlers[jobType]; !ok {
		return nil, fmt.Errorf("no handler for job type %q", jobType)
	}
	raw := "{}"
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	var libraryID int64
	var probe struct {
		LibraryID int64 `json:"libraryId"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err == nil {
		libraryID = probe.LibraryID
	}
	job, err := m.store.CreateJob(ctx, jobType, raw, libraryID)
	if err != nil {
		return nil, err
	}
	select {
	case m.queue <- job.ID:
	default:
		// Queue is full; the job stays queued in the DB and will be picked up on
		// the next Start/recovery.
		m.log.Warn("job queue full, leaving job queued", "job", job.ID)
	}
	return job, nil
}

// Start recovers stale jobs, re-queues pending ones and launches the worker.
func (m *Manager) Start(ctx context.Context) {
	if err := m.store.FailStaleJobs(ctx); err != nil {
		m.log.Error("fail stale jobs", "err", err)
	}
	if ids, err := m.store.QueuedJobs(ctx); err == nil {
		for _, id := range ids {
			select {
			case m.queue <- id:
			default:
			}
		}
	}
	go m.loop(ctx)
}

func (m *Manager) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-m.queue:
			m.run(ctx, id)
		}
	}
}

func (m *Manager) run(ctx context.Context, id int64) {
	job, err := m.store.GetJob(ctx, id)
	if err != nil {
		m.log.Error("load job", "job", id, "err", err)
		return
	}
	h, ok := m.handlers[job.Type]
	if !ok {
		_ = m.store.FinishJob(ctx, id, "failed", "no handler registered", "")
		return
	}
	if job.Status != "queued" {
		return
	}
	if err := m.store.StartJob(ctx, id); err != nil {
		m.log.Error("start job", "job", id, "err", err)
		return
	}
	m.log.Info("job started", "job", id, "type", job.Type)
	_ = m.store.AppendJobLog(ctx, id, "info", "job started ("+job.Type+")")

	// Throttle DB writes for progress updates.
	last := time.Time{}
	report := func(pct float64, msg string) {
		if time.Since(last) < 300*time.Millisecond && pct < 100 {
			return
		}
		last = time.Now()
		if err := m.store.UpdateJob(ctx, id, pct, msg); err != nil {
			m.log.Warn("update job", "job", id, "err", err)
		}
	}

	err = h(ctx, job, report)
	if err != nil {
		safeErr := redact.String(err.Error())
		m.log.Error("job failed", "job", id, "type", job.Type, "err", safeErr)
		_ = m.store.AppendJobLog(ctx, id, "error", safeErr)
		_ = m.store.FinishJob(ctx, id, "failed", safeErr, "")
		_ = m.store.PruneJobLogs(ctx, id, 50000)
		return
	}
	_ = m.store.AppendJobLog(ctx, id, "info", "job finished")
	_ = m.store.FinishJob(ctx, id, "succeeded", "", "")
	_ = m.store.PruneJobLogs(ctx, id, 50000)
	m.log.Info("job finished", "job", id, "type", job.Type)
}
