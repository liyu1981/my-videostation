// Package playback turns media files into on-demand HLS sessions: remux when
// the codecs are already HLS-friendly, transcode when they are not.
package playback

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNoFFmpeg is returned when transcoding is requested without ffmpeg.
var ErrNoFFmpeg = errors.New("ffmpeg not available")

// ErrTooManySessions is returned when the session cap is reached and no idle
// session can be evicted.
var ErrTooManySessions = errors.New("too many concurrent transcode sessions")

const (
	defaultMaxSessions = 2
	defaultIdleTimeout = 3 * time.Minute
	startupTimeout     = 45 * time.Second
)

// Options configures the session manager.
type Options struct {
	CacheDir    string // /data/cache; sessions live in <CacheDir>/transcodes/<id>
	FFmpeg      string // ffmpeg binary; empty disables transcoding
	MaxSessions int
	IdleTimeout time.Duration
	Encoder     string // libx264 | h264_nvenc | h264_qsv
	HWAccel     string // optional -hwaccel value, e.g. cuda, vaapi, qsv
	Paced       bool   // throttle to ~1x (default true)
	// ReapInterval overrides the idle-session sweep period (tests).
	ReapInterval time.Duration
	Logger       *slog.Logger
}

// Manager owns the live HLS sessions.
type Manager struct {
	cacheDir    string
	ffmpeg      string
	maxSessions int
	idle        time.Duration
	log         *slog.Logger

	encoder    string
	hwaccel    string
	paced      bool
	reapPeriod time.Duration

	readrateOnce sync.Once
	readrate     bool

	mu       sync.Mutex
	sessions map[string]*Session
	byKey    map[string]*Session
	closed   bool
	reaper   *time.Ticker
	stop     chan struct{}
}

// Session is one running ffmpeg HLS job.
type Session struct {
	ID       string
	Key      string
	FileID   int64
	Path     string
	Dir      string
	Playlist string
	Plan     *Plan

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}

	lastSeen  atomic.Int64 // unix nanoseconds
	mu        sync.Mutex
	encodedMS int64
	speed     float64
	startedAt time.Time
	ended     bool
	errMsg    string
}

// Status is the JSON-facing snapshot of a session.
type Status struct {
	ID           string   `json:"id"`
	FileID       int64    `json:"fileId"`
	Mode         Mode     `json:"mode"`
	PlaylistURL  string   `json:"playlistUrl"`
	StartMS      int64    `json:"startMs"`
	DurationMS   int64    `json:"durationMs"`
	EncodedMS    int64    `json:"encodedMs"`
	Speed        float64  `json:"speed"`
	Height       int      `json:"height"`
	VideoCodec   string   `json:"videoCodec"`
	AudioCodec   string   `json:"audioCodec"`
	AudioIndex   int      `json:"audioIndex"`
	BurnSubtitle int      `json:"burnSubtitle"`
	Reasons      []string `json:"reasons,omitempty"`
	Running      bool     `json:"running"`
	Error        string   `json:"error,omitempty"`
	StartedAt    string   `json:"startedAt"`
}

// New creates a manager and clears stale session directories from a previous
// process (they are pure cache; a restart invalidates every playlist).
func New(o Options) *Manager {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.MaxSessions <= 0 {
		o.MaxSessions = defaultMaxSessions
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = defaultIdleTimeout
	}
	if o.Encoder == "" {
		o.Encoder = "libx264"
	}
	m := &Manager{
		cacheDir:    o.CacheDir,
		ffmpeg:      o.FFmpeg,
		maxSessions: o.MaxSessions,
		idle:        o.IdleTimeout,
		log:         o.Logger,
		encoder:     o.Encoder,
		hwaccel:     o.HWAccel,
		paced:       o.Paced,
		reapPeriod:  o.ReapInterval,
		sessions:    map[string]*Session{},
		byKey:       map[string]*Session{},
		stop:        make(chan struct{}),
	}
	if m.reapPeriod <= 0 {
		m.reapPeriod = 15 * time.Second
	}
	m.purgeCache()
	m.reaper = time.NewTicker(m.reapPeriod)
	go m.reap(m.reaper)
	return m
}

// Available reports whether transcoding is possible.
func (m *Manager) Available() bool { return m.ffmpeg != "" }

// Encoder is the configured encoder name.
func (m *Manager) Encoder() string { return m.encoder }

func (m *Manager) purgeCache() {
	if m.cacheDir == "" {
		return
	}
	dir := filepath.Join(m.cacheDir, "transcodes")
	if err := os.RemoveAll(dir); err != nil {
		m.log.Warn("could not clear old transcode cache", "dir", dir, "err", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.log.Warn("could not create transcode cache", "dir", dir, "err", err)
	}
}

// Start resolves a session for req, reusing an identical live session when one
// exists. The returned session is ready to serve its playlist.
func (m *Manager) Start(ctx context.Context, req Request) (*Session, error) {
	if m.ffmpeg == "" {
		return nil, ErrNoFFmpeg
	}
	if m.cacheDir == "" {
		return nil, errors.New("transcode cache directory is not configured")
	}
	if req.Path == "" {
		return nil, errors.New("no media path")
	}
	key := sessionKey(req)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("server is shutting down")
	}
	if s, ok := m.byKey[key]; ok && s.reusable() {
		s.touch()
		m.mu.Unlock()
		return s, nil
	}
	m.purgeLocked()
	for len(m.sessions) >= m.maxSessions {
		victim := m.oldestLocked()
		if victim == nil {
			m.mu.Unlock()
			return nil, ErrTooManySessions
		}
		m.log.Info("evicting idle transcode session", "session", victim.ID, "file", victim.FileID)
		m.mu.Unlock()
		m.Stop(victim.ID)
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, errors.New("server is shutting down")
		}
	}
	var stale *Session
	if old, ok := m.byKey[key]; ok {
		// A failed job for the same inputs is useless; drop it when replacing.
		stale = old
		delete(m.sessions, old.ID)
		delete(m.byKey, key)
	}
	id := newID()
	dir := filepath.Join(m.cacheDir, "transcodes", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	req.OutDir = dir
	plan, err := BuildPlan(req, m.planOptions())
	if err != nil {
		m.mu.Unlock()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	s := &Session{
		ID:       id,
		Key:      key,
		FileID:   req.FileID,
		Path:     req.Path,
		Dir:      dir,
		Playlist: filepath.Join(dir, "index.m3u8"),
		Plan:     plan,
		done:     make(chan struct{}),
	}
	s.touch()
	m.sessions[id] = s
	m.byKey[key] = s
	m.mu.Unlock()
	if stale != nil {
		m.Stop(stale.ID)
	}

	if err := m.spawn(s); err != nil {
		m.drop(s)
		return nil, err
	}
	if err := s.waitPlaylist(startupTimeout); err != nil {
		m.drop(s)
		return nil, err
	}
	m.log.Info("transcode session started",
		"session", s.ID, "file", s.FileID, "mode", plan.Mode,
		"height", plan.Height, "audio", plan.AudioIndex, "reasons", plan.Reasons)
	return s, nil
}

func (m *Manager) planOptions() options {
	return options{
		Encoder:  m.encoder,
		HWAccel:  m.hwaccel,
		Paced:    m.paced,
		Readrate: m.supportsReadrate(),
	}
}

func (m *Manager) spawn(s *Session) error {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, m.ffmpeg, s.Plan.Args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr := &limitedBuffer{limit: 8 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	s.cmd = cmd
	s.cancel = cancel
	s.startedAt = time.Now()
	go s.readProgress(stdout)
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.ended = true
		if err != nil && !errors.Is(err, context.Canceled) {
			s.errMsg = strings.TrimSpace(stderr.String())
			if s.errMsg == "" {
				s.errMsg = err.Error()
			}
		}
		s.mu.Unlock()
		close(s.done)
	}()
	return nil
}

// supportsReadrate detects -readrate once (ffmpeg 5.0+). Older builds fall
// back to -re, which is supported everywhere.
func (m *Manager) supportsReadrate() bool {
	m.readrateOnce.Do(func() {
		if m.ffmpeg == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-h", "full").Output()
		m.readrate = strings.Contains(string(out), "-readrate")
	})
	return m.readrate
}

// Get returns a live session by id, touching its activity time.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.touch()
	}
	return s, ok
}

// Statuses snapshots every session (admin/debug endpoint).
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	out := make([]Status, 0, len(list))
	for _, s := range list {
		out = append(out, s.Status())
	}
	return out
}

// Stop terminates a session and removes its segments.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
		if m.byKey[s.Key] == s {
			delete(m.byKey, s.Key)
		}
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.cmd != nil {
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			if s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
			<-s.done
		}
	}
	_ = os.RemoveAll(s.Dir)
}

// Shutdown stops every session. Safe to call twice.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	if m.reaper != nil {
		m.reaper.Stop()
	}
	close(m.stop)
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
	_ = os.RemoveAll(filepath.Join(m.cacheDir, "transcodes"))
}

func (m *Manager) drop(s *Session) {
	m.mu.Lock()
	delete(m.sessions, s.ID)
	if m.byKey[s.Key] == s {
		delete(m.byKey, s.Key)
	}
	m.mu.Unlock()
	m.Stop(s.ID)
	_ = os.RemoveAll(s.Dir)
}

// purgeLocked forgets finished sessions (caller holds m.mu).
func (m *Manager) purgeLocked() {
	for id, s := range m.sessions {
		if !s.failed() {
			continue
		}
		delete(m.sessions, id)
		if m.byKey[s.Key] == s {
			delete(m.byKey, s.Key)
		}
		go func(dir string) { _ = os.RemoveAll(dir) }(s.Dir)
	}
}

func (m *Manager) oldestLocked() *Session {
	var oldest *Session
	for _, s := range m.sessions {
		if oldest == nil {
			oldest = s
			continue
		}
		// Finished jobs are cheaper to lose than a still-running encode.
		if s.finished() != oldest.finished() {
			if s.finished() {
				oldest = s
			}
			continue
		}
		if s.lastSeen.Load() < oldest.lastSeen.Load() {
			oldest = s
		}
	}
	return oldest
}

func (m *Manager) reap(ticker *time.Ticker) {
	if m.reapPeriod <= 0 {
		m.reapPeriod = 15 * time.Second
	}
	defer m.reaper.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.mu.Lock()
			var stale []string
			for id, s := range m.sessions {
				idleFor := time.Since(time.Unix(0, s.lastSeen.Load()))
				if s.finished() {
					stale = append(stale, id)
					continue
				}
				if idleFor > m.idle {
					m.log.Info("stopping idle transcode session", "session", id, "idle", idleFor.Round(time.Second))
					stale = append(stale, id)
				}
			}
			m.mu.Unlock()
			for _, id := range stale {
				m.Stop(id)
			}
		}
	}
}

// ---- session helpers ------------------------------------------------------

func (s *Session) touch() { s.lastSeen.Store(time.Now().UnixNano()) }

func (s *Session) finished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// failed reports whether the encoder died with an error (unusable session).
func (s *Session) failed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended && s.errMsg != ""
}

// reusable reports whether the session still has a playlist to serve.
func (s *Session) reusable() bool {
	s.mu.Lock()
	ended, msg := s.ended, s.errMsg
	s.mu.Unlock()
	if ended && msg != "" {
		return false
	}
	return playlistReady(s.Playlist)
}

func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{
		ID:           s.ID,
		FileID:       s.FileID,
		Mode:         s.Plan.Mode,
		PlaylistURL:  "/hls/" + s.ID + "/index.m3u8",
		StartMS:      s.Plan.StartMS,
		DurationMS:   s.Plan.DurationMS,
		EncodedMS:    s.encodedMS,
		Speed:        s.speed,
		Height:       s.Plan.Height,
		VideoCodec:   s.Plan.VideoCodec,
		AudioCodec:   s.Plan.AudioCodec,
		AudioIndex:   s.Plan.AudioIndex,
		BurnSubtitle: s.Plan.BurnSubtitle,
		Reasons:      s.Plan.Reasons,
		Running:      !s.ended,
		Error:        s.errMsg,
		StartedAt:    s.startedAt.UTC().Format(time.RFC3339),
	}
	return st
}

func (s *Session) Err() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errMsg
}

// waitPlaylist blocks until ffmpeg has written the initial playlist or has
// failed. It deliberately ignores client cancellation: the job stays useful for
// a retry or a second viewer.
func (s *Session) waitPlaylist(timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if playlistReady(s.Playlist) {
			return nil
		}
		select {
		case <-s.done:
			// Short clips can finish before the first poll; check once more.
			if playlistReady(s.Playlist) {
				return nil
			}
			if msg := s.Err(); msg != "" {
				return fmt.Errorf("ffmpeg failed: %s", msg)
			}
			return errors.New("ffmpeg exited before producing a playlist")
		case <-deadline.C:
			return errors.New("timed out waiting for the transcode to start")
		case <-tick.C:
		}
	}
}

func playlistReady(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

// readProgress parses `-progress pipe:1` output.
func (s *Session) readProgress(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				s.mu.Lock()
				s.encodedMS = v / 1000
				s.mu.Unlock()
			}
		case "speed":
			if v, err := strconv.ParseFloat(strings.TrimSuffix(value, "x"), 64); err == nil {
				s.mu.Lock()
				s.speed = v
				s.mu.Unlock()
			}
		}
	}
}

// limitedBuffer keeps the tail of ffmpeg's stderr for error reporting.
type limitedBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func sessionKey(req Request) string {
	start := req.StartMS
	if start < 3000 {
		start = 0
	}
	// Bucket seeks to 2s so a handful of seeks near a keyframe reuse one job.
	start = start / 2000 * 2000
	return fmt.Sprintf("%d|%d|%d|%d|%d|%s|%d|%d|%s",
		req.FileID, req.Mtime, start, req.AudioIndex, req.BurnSubtitle,
		req.BurnSubtitlePath, req.MaxHeight, req.DurationMS, strings.ToLower(req.Force))
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
