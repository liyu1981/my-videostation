package enrich

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/artwork"
	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	d, err := db.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	cacheDir := filepath.Join(dir, "cache")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(st, nil, cacheDir, log), st, cacheDir
}

func TestIsLocalArt(t *testing.T) {
	s, _, cacheDir := newTestService(t)
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(cacheDir, "images", "abc.jpg"), false}, // downloaded
		{"/media/movies/Heat (1995)/poster.jpg", true},        // beside the media
		{"", false},
	}
	for _, tc := range cases {
		if got := s.isLocalArt(tc.path); got != tc.want {
			t.Errorf("isLocalArt(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCoverPreferenceAndSettings(t *testing.T) {
	s, st, _ := newTestService(t)
	ctx := context.Background()
	if got := s.coverPreference(ctx); got != artwork.CoverProvider {
		t.Fatalf("default cover preference = %q", got)
	}
	if err := st.PutSettings(ctx, map[string]string{artwork.SettingCoverPreference: artwork.CoverLocal}); err != nil {
		t.Fatal(err)
	}
	if got := s.coverPreference(ctx); got != artwork.CoverLocal {
		t.Fatalf("stored cover preference = %q", got)
	}
}
