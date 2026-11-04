package playback

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireFFmpeg skips integration tests when ffmpeg is unavailable.
func requireFFmpeg(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	return path
}

// makeSample renders a short clip; vcodec/acodec pick the source properties.
func makeSample(t *testing.T, ffmpeg, dir, name, vcodec, acodec string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "12", "-c:v", vcodec, "-g", "25", "-c:a", acodec, "-shortest", out,
	}
	if vcodec == "libx264" {
		args = append(args[:len(args)-4], append([]string{"-preset", "ultrafast"}, args[len(args)-4:]...)...)
	}
	cmd := exec.Command(ffmpeg, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create sample %s: %v\n%s", name, err, b)
	}
	return out
}

func newTestManager(t *testing.T, ffmpeg string) *Manager {
	t.Helper()
	m := New(Options{
		CacheDir:     t.TempDir(),
		FFmpeg:       ffmpeg,
		MaxSessions:  1,
		IdleTimeout:  time.Minute,
		ReapInterval: 50 * time.Millisecond,
		Encoder:      "libx264",
		Paced:        false, // encode as fast as possible in tests
	})
	t.Cleanup(m.Shutdown)
	return m
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func readPlaylist(t *testing.T, s *Session) string {
	t.Helper()
	b, err := os.ReadFile(s.Playlist)
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	return string(b)
}

func TestManagerRemux(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	file := makeSample(t, ffmpeg, dir, "sample.mkv", "libx264", "aac")
	m := newTestManager(t, ffmpeg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := m.Start(ctx, Request{
		FileID: 1, Path: file, Container: "mkv", VideoCodec: "h264",
		Width: 320, Height: 240, DurationMS: 12000,
		Audio:      []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.Plan.Mode != ModeRemux {
		t.Fatalf("mode = %s, want remux (reasons %v)", s.Plan.Mode, s.Plan.Reasons)
	}
	playlist := readPlaylist(t, s)
	if !strings.Contains(playlist, "#EXTM3U") || !strings.Contains(playlist, "init.mp4") {
		t.Fatalf("unexpected playlist:\n%s", playlist)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "init.mp4")); err != nil {
		t.Fatalf("missing init segment: %v", err)
	}
	waitFor(t, 30*time.Second, func() bool {
		entries, _ := filepath.Glob(filepath.Join(s.Dir, "seg*.m4s"))
		return len(entries) >= 2
	})
	// Re-requesting the same parameters must reuse the session.
	again, err := m.Start(ctx, Request{
		FileID: 1, Path: file, Container: "mkv", VideoCodec: "h264",
		Width: 320, Height: 240, DurationMS: 12000,
		Audio:      []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
	})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if again.ID != s.ID {
		t.Fatalf("expected session reuse, got %s and %s", s.ID, again.ID)
	}

	sessionDir := s.Dir
	m.Stop(s.ID)
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session dir not removed: %v", err)
	}
	if _, ok := m.Get(s.ID); ok {
		t.Fatal("stopped session still registered")
	}
}

func TestManagerTranscodeWithSeek(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	file := makeSample(t, ffmpeg, dir, "legacy.avi", "mpeg4", "libmp3lame")
	m := newTestManager(t, ffmpeg)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s, err := m.Start(ctx, Request{
		FileID: 2, Path: file, Container: "avi", VideoCodec: "mpeg4",
		Width: 320, Height: 240, DurationMS: 12000,
		Audio:      []AudioStream{{Index: 1, Codec: "mp3", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
		StartMS: 4000, MaxHeight: 120,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.Plan.Mode != ModeTranscode {
		t.Fatalf("mode = %s, want transcode", s.Plan.Mode)
	}
	if s.Plan.Height != 120 {
		t.Fatalf("height = %d, want 120", s.Plan.Height)
	}
	if s.Plan.DurationMS != 8000 {
		t.Fatalf("duration = %d, want 8000", s.Plan.DurationMS)
	}
	waitFor(t, 60*time.Second, func() bool {
		st := s.Status()
		if st.Error != "" {
			t.Fatalf("ffmpeg failed: %s", st.Error)
		}
		return st.EncodedMS > 0 || !st.Running
	})
	waitFor(t, 60*time.Second, func() bool {
		entries, _ := filepath.Glob(filepath.Join(s.Dir, "seg*.m4s"))
		return len(entries) >= 1
	})
}

func TestManagerSessionCapEvictsIdle(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	a := makeSample(t, ffmpeg, dir, "a.mkv", "libx264", "aac")
	b := makeSample(t, ffmpeg, dir, "b.mkv", "libx264", "aac")
	m := newTestManager(t, ffmpeg) // MaxSessions = 1

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	first, err := m.Start(ctx, Request{
		FileID: 1, Path: a, Container: "mkv", VideoCodec: "h264", Height: 240,
		DurationMS: 12000, Audio: []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
	})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	firstDir := first.Dir
	second, err := m.Start(ctx, Request{
		FileID: 2, Path: b, Container: "mkv", VideoCodec: "h264", Height: 240,
		DurationMS: 12000, Audio: []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
	})
	if err != nil {
		t.Fatalf("second start should evict the idle session: %v", err)
	}
	if _, ok := m.Get(first.ID); ok {
		t.Fatal("first session should have been evicted")
	}
	if _, err := os.Stat(firstDir); !os.IsNotExist(err) {
		t.Fatalf("evicted session dir still present: %v", err)
	}
	if _, ok := m.Get(second.ID); !ok {
		t.Fatal("second session missing")
	}
}

func TestManagerIdleReaper(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	file := makeSample(t, ffmpeg, dir, "sample.mkv", "libx264", "aac")
	m := New(Options{
		CacheDir:     t.TempDir(),
		FFmpeg:       ffmpeg,
		MaxSessions:  2,
		IdleTimeout:  150 * time.Millisecond,
		ReapInterval: 50 * time.Millisecond,
		Paced:        false,
	})
	t.Cleanup(m.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := m.Start(ctx, Request{
		FileID: 1, Path: file, Container: "mkv", VideoCodec: "h264", Height: 240,
		DurationMS: 12000, Audio: []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		_, ok := m.Get(s.ID)
		return !ok
	})
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Fatalf("reaped session dir still present: %v", err)
	}
}

func TestManagerNoFFmpeg(t *testing.T) {
	m := New(Options{CacheDir: t.TempDir()})
	t.Cleanup(m.Shutdown)
	if m.Available() {
		t.Fatal("manager without ffmpeg should not be available")
	}
	if _, err := m.Start(context.Background(), Request{Path: "x"}); err != ErrNoFFmpeg {
		t.Fatalf("err = %v, want ErrNoFFmpeg", err)
	}
}
