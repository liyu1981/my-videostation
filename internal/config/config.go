// Package config resolves runtime configuration from flags, environment and defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the process configuration. Precedence: flags > MVS_* env > defaults.
type Config struct {
	Listen    string // host:port to bind
	DataDir   string // writable directory for db, cache, logs
	DBPath    string // sqlite file (defaults to <DataDir>/mvs.db)
	FFprobe   string // explicit ffprobe path (optional; auto-detected)
	LogLevel  string // debug|info|warn|error
	LogFormat string // text|json

	// Transcoding (M5).
	TranscodeSessions int           // concurrent HLS sessions (default 2)
	TranscodeIdle     time.Duration // idle session timeout (default 3m)
	TranscodeEncoder  string        // libx264 | h264_nvenc | h264_qsv
	TranscodeHWAccel  string        // optional ffmpeg -hwaccel value
	TranscodePaced    bool          // throttle ffmpeg to ~1x (default true)
}

// Defaults returns the baseline configuration.
func Defaults() Config {
	dataDir := envOr("MVS_DATA_DIR", "data")
	return Config{
		Listen:    envOr("MVS_LISTEN", "127.0.0.1:6500"),
		DataDir:   dataDir,
		DBPath:    envOr("MVS_DB", filepath.Join(dataDir, "mvs.db")),
		FFprobe:   envOr("MVS_FFPROBE", ""),
		LogLevel:  envOr("MVS_LOG_LEVEL", "info"),
		LogFormat: envOr("MVS_LOG_FORMAT", "text"),

		TranscodeSessions: envInt("MVS_TRANSCODE_SESSIONS", 2),
		TranscodeIdle:     time.Duration(envInt("MVS_TRANSCODE_IDLE_SECONDS", 180)) * time.Second,
		TranscodeEncoder:  envOr("MVS_TRANSCODE_ENCODER", "libx264"),
		TranscodeHWAccel:  envOr("MVS_TRANSCODE_HWACCEL", ""),
		TranscodePaced:    envBool("MVS_TRANSCODE_PACED", true),
	}
}

// Load parses flags on top of environment-derived defaults.
func Load(args []string) (Config, error) {
	cfg := Defaults()
	fs := flag.NewFlagSet("mvs", flag.ContinueOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "listen address (host:port)")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "data directory (db, cache)")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "sqlite database path")
	fs.StringVar(&cfg.FFprobe, "ffprobe", cfg.FFprobe, "path to ffprobe binary")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "debug|info|warn|error")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "text|json")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	dbFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbFlagSet = true
		}
	})
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if cfg.DataDir == "" {
		return Config{}, fmt.Errorf("data directory must not be empty")
	}
	// Absolute paths only: cached artwork paths are persisted in SQLite and must
	// not depend on the process working directory.
	if abs, err := filepath.Abs(cfg.DataDir); err == nil {
		cfg.DataDir = abs
	}
	// The database lives in the data dir unless -db/MVS_DB says otherwise.
	if !dbFlagSet {
		cfg.DBPath = filepath.Join(cfg.DataDir, "mvs.db")
	}
	if abs, err := filepath.Abs(cfg.DBPath); err == nil {
		cfg.DBPath = abs
	}
	if cfg.FFprobe != "" {
		if abs, err := filepath.Abs(cfg.FFprobe); err == nil {
			cfg.FFprobe = abs
		}
	}
	switch cfg.LogFormat {
	case "text", "json":
	default:
		return Config{}, fmt.Errorf("invalid log format %q (want text|json)", cfg.LogFormat)
	}
	return cfg, nil
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
