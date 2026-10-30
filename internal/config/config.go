// Package config resolves runtime configuration from flags, environment and defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the process configuration. Precedence: flags > MVS_* env > defaults.
type Config struct {
	Listen    string // host:port to bind
	DataDir   string // writable directory for db, cache, logs
	DBPath    string // sqlite file (defaults to <DataDir>/mvs.db)
	FFprobe   string // explicit ffprobe path (optional; auto-detected)
	LogLevel  string // debug|info|warn|error
	LogFormat string // text|json
}

// Defaults returns the baseline configuration.
func Defaults() Config {
	dataDir := envOr("MVS_DATA_DIR", "data")
	return Config{
		Listen:    envOr("MVS_LISTEN", "127.0.0.1:8080"),
		DataDir:   dataDir,
		DBPath:    envOr("MVS_DB", filepath.Join(dataDir, "mvs.db")),
		FFprobe:   envOr("MVS_FFPROBE", ""),
		LogLevel:  envOr("MVS_LOG_LEVEL", "info"),
		LogFormat: envOr("MVS_LOG_FORMAT", "text"),
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
	// The database lives in the data dir unless -db/MVS_DB says otherwise.
	if !dbFlagSet {
		cfg.DBPath = filepath.Join(cfg.DataDir, "mvs.db")
	}
	switch cfg.LogFormat {
	case "text", "json":
	default:
		return Config{}, fmt.Errorf("invalid log format %q (want text|json)", cfg.LogFormat)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
