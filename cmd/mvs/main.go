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
	"syscall"
	"time"

	"github.com/liyu1981/myvideostation/internal/buildinfo"
	"github.com/liyu1981/myvideostation/internal/config"
	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/httpapi"
	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/media"
	"github.com/liyu1981/myvideostation/internal/scanner"
	"github.com/liyu1981/myvideostation/internal/store"
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

	sc := scanner.New(st, prober, log)
	manager := jobs.New(st, log)
	manager.Handle("scan", func(ctx context.Context, job *store.Job, report jobs.Progress) error {
		var payload struct {
			LibraryID int64 `json:"libraryId"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("bad job payload: %w", err)
		}
		if payload.LibraryID == 0 {
			return errors.New("missing libraryId")
		}
		name := "library"
		if lib, err := st.GetLibrary(ctx, payload.LibraryID); err == nil {
			name = lib.Name
		}
		report(0, "scanning "+name)
		stats, err := sc.ScanLibrary(ctx, payload.LibraryID, func(p float64, msg string) {
			report(p, msg)
		})
		if err != nil {
			return err
		}
		report(100, name+": "+stats.String())
		return nil
	})
	manager.Start(ctx)

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
