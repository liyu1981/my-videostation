package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMakesPathsAbsolute(t *testing.T) {
	cfg, err := Load([]string{"--data", "relative-data", "--listen", "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.DataDir) {
		t.Errorf("DataDir = %q, want absolute", cfg.DataDir)
	}
	if !filepath.IsAbs(cfg.DBPath) {
		t.Errorf("DBPath = %q, want absolute", cfg.DBPath)
	}
	if filepath.Dir(cfg.DBPath) != cfg.DataDir {
		t.Errorf("DBPath %q not inside DataDir %q", cfg.DBPath, cfg.DataDir)
	}
}
