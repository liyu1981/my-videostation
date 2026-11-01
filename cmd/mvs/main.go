// Command mvs runs the MyVideoStation server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/liyu1981/myvideostation/internal/buildinfo"
	"github.com/liyu1981/myvideostation/internal/config"
	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/enrich"
	"github.com/liyu1981/myvideostation/internal/httpapi"
	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/media"
	"github.com/liyu1981/myvideostation/internal/metadata"
	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
	"github.com/liyu1981/myvideostation/internal/watch"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mvs:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	log := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	st := store.New(database)

	prober, err := media.Detect(cfg.FFprobe)
	switch {
	case err == nil:
		log.Info("ffprobe found", "path", prober.Path)
	case errors.Is(err, media.ErrNoProber):
		log.Warn("ffprobe not found: technical metadata will be skipped", "hint", "run `pnpm bootstrap`")
	default:
		return err
	}

	// Metadata provider is built from settings at call time so a key can be added
	// without restarting the server.
	providerFactory := func(ctx context.Context) (metadata.Provider, error) {
		key, _, err := st.Setting(ctx, metadata.SettingAPIKey)
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, metadata.ErrNotConfigured
		}
		language, _, _ := st.Setting(ctx, metadata.SettingLanguage)
		region, _, _ := st.Setting(ctx, metadata.SettingRegion)
		return metadata.NewTMDB(key, language, region), nil
	}
	providers := metadata.NewRegistry()
	providers.Register("tmdb", providerFactory)
	cacheDir := filepath.Join(cfg.DataDir, "cache")
	if n, err := st.RebaseCachePaths(ctx, cacheDir); err != nil {
		log.Warn("cached artwork path repair failed", "err", err)
	} else if n > 0 {
		log.Info("re-pointed cached artwork paths", "count", n)
	}
	enricher := enrich.NewWithRegistry(st, providers, cacheDir, log)
	ffmpegPath := media.DetectFFmpeg("")
	if ffmpegPath == "" {
		log.Warn("ffmpeg not found: embedded subtitle extraction disabled", "hint", "run `pnpm bootstrap`")
	} else {
		log.Info("ffmpeg found", "path", ffmpegPath)
	}

	sc := scanner.New(st, prober, log)
	sc.SetCacheDir(cacheDir)
	manager := jobs.New(st, log)
	manager.Handle("scan", func(ctx context.Context, job *store.Job, report jobs.Progress) error {
		var payload struct {
			LibraryID int64 `json:"libraryId"`
			Force     bool  `json:"force"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("bad job payload: %w", err)
		}
		if payload.LibraryID == 0 {
			return errors.New("missing libraryId")
		}
		// Any scan (manual, watcher, create) resets the auto-scan clock so the
		// scheduler does not immediately scan the same library again.
		_ = st.MarkAutoScan(ctx, payload.LibraryID)
		name := "library"
		if lib, err := st.GetLibrary(ctx, payload.LibraryID); err == nil {
			name = lib.Name
		}
		lastMsg := ""
		var logMu sync.Mutex
		reportMsg := func(p float64, msg string) {
			report(p, msg)
			logMu.Lock()
			changed := msg != lastMsg
			if changed {
				lastMsg = msg
			}
			logMu.Unlock()
			if changed {
				_ = st.AppendJobLog(ctx, job.ID, "info", msg)
			}
		}
		logLine := func(level, msg string) {
			_ = st.AppendJobLog(ctx, job.ID, level, msg)
		}
		sc.SetJobLogger(logLine)
		defer sc.SetJobLogger(nil)

		if payload.Force {
			reportMsg(0, "rescanning "+name+" (force re-probe)")
		} else {
			reportMsg(0, "scanning "+name)
		}
		stats, err := sc.ScanLibraryForce(ctx, payload.LibraryID, payload.Force, reportMsg)
		if err != nil {
			return err
		}
		reportMsg(100, name+": "+stats.String())
		// Chain the first metadata pass: a scan leaves unmatched items, so queue
		// enrichment automatically (deduplicated against an active enrich job).
		if n, err := st.CountUnmatched(ctx, payload.LibraryID); err == nil && n > 0 {
			if active, _ := st.HasActiveJob(ctx, "enrich", payload.LibraryID); !active {
				if _, err := manager.Enqueue(ctx, "enrich", map[string]int64{"libraryId": payload.LibraryID}); err != nil {
					log.Warn("auto-enrich enqueue failed", "library", payload.LibraryID, "err", err)
				} else {
					_ = st.AppendJobLog(ctx, job.ID, "info", fmt.Sprintf("queued metadata match for %d unmatched item(s)", n))
				}
			}
		}
		return nil
	})
	manager.Handle("enrich", func(ctx context.Context, job *store.Job, report jobs.Progress) error {
		var payload struct {
			LibraryID int64 `json:"libraryId"`
			Force     bool  `json:"force"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("bad job payload: %w", err)
		}
		lastMsg := ""
		reportMsg := func(p float64, msg string) {
			report(p, msg)
			if msg != lastMsg {
				lastMsg = msg
				_ = st.AppendJobLog(ctx, job.ID, "info", msg)
			}
		}
		stats, err := enricher.EnrichLibrary(ctx, payload.LibraryID, reportMsg)
		if err != nil {
			return err
		}
		reportMsg(100, stats.String())
		return nil
	})
	manager.Start(ctx)

	// Filesystem watcher: local/bind-mounted libraries get debounced scans;
	// network filesystems and exhausted inotify limits fall back to the scheduler.
	watcher := watch.New(st, manager, log)
	go watcher.Run(ctx)

	// Auto-scan scheduler: checks every minute for libraries whose interval elapsed.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ids, err := st.LibrariesDueForScan(ctx)
				if err != nil {
					log.Error("auto-scan lookup", "err", err)
					continue
				}
				for _, id := range ids {
					active, err := st.HasActiveJob(ctx, "scan", id)
					if err != nil || active {
						continue
					}
					if _, err := manager.Enqueue(ctx, "scan", map[string]int64{"libraryId": id}); err != nil {
						log.Error("auto-scan enqueue", "library", id, "err", err)
						continue
					}
					if err := st.MarkAutoScan(ctx, id); err != nil {
						log.Warn("mark auto-scan", "library", id, "err", err)
					}
					log.Info("auto-scan started", "library", id)
				}
			}
		}
	}()

	cors := splitList(os.Getenv("MVS_CORS_ORIGIN"))
	if len(cors) == 0 {
		cors = []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}
	api := httpapi.New(httpapi.Options{
		Store:      st,
		Jobs:       manager,
		Logger:     log,
		CORSOrigin: cors,
		Version:    buildinfo.Version,
		MediaDir:   cfg.DataDir,
		Provider:   providerFactory,
		Enricher:   enricher,
		FFmpeg:     ffmpegPath,
		CacheDir:   cacheDir,
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("MyVideoStation listening",
			"addr", "http://"+cfg.Listen,
			"version", buildinfo.Version,
			"data", absPath(cfg.DataDir),
			"db", absPath(cfg.DBPath))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
