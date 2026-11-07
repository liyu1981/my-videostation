package enrich

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/liyu1981/myvideostation/internal/db"
	"github.com/liyu1981/myvideostation/internal/metadata"
	"github.com/liyu1981/myvideostation/internal/store"
)

// fakeProvider answers movie/TV searches and details with fixed data.
type fakeProvider struct {
	searchQueries []string
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) SearchMovies(_ context.Context, query string, year int) ([]metadata.Candidate, error) {
	f.searchQueries = append(f.searchQueries, query)
	return []metadata.Candidate{{ProviderID: "42", Title: query, Year: year}}, nil
}

func (f *fakeProvider) SearchTV(ctx context.Context, query string, year int) ([]metadata.Candidate, error) {
	return f.SearchMovies(ctx, query, year)
}

func (f *fakeProvider) Movie(_ context.Context, id string) (*metadata.Details, error) {
	return &metadata.Details{Candidate: metadata.Candidate{ProviderID: id, Title: "Matched Title", Year: 2020}}, nil
}

func (f *fakeProvider) TV(ctx context.Context, id string) (*metadata.Details, error) {
	return f.Movie(ctx, id)
}

// TestHomeVideoCanBeSearchedAndMatched: a home video (the placeholder kind for an
// unparseable episode) can be searched and matched manually as a movie, so it can
// be identified without renaming the file.
func TestHomeVideoCanBeSearchedAndMatched(t *testing.T) {
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
	lib, err := st.CreateLibrary(ctx, "Home", "home_video", "", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "some episode.mkv")
	if _, err := st.ApplyScanBatch(ctx, []store.ScannedFile{{
		LibraryID: lib.ID, Path: path, RelPath: "some episode.mkv",
		Kind: store.KindHomeVideo, Title: "some episode",
	}}); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListItems(ctx, store.ItemFilter{LibraryID: lib.ID, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	item := items[0]

	provider := &fakeProvider{}
	reg := metadata.NewRegistry()
	reg.Register("fake", func(context.Context) (metadata.Provider, error) { return provider, nil })
	svc := NewWithRegistry(st, reg, filepath.Join(dir, "cache"), slog.New(slog.NewTextHandler(io.Discard, nil)))

	candidates, err := svc.Search(ctx, item.ID, "", 0, "", "fake")
	if err != nil {
		t.Fatalf("search home video: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderID != "42" {
		t.Fatalf("candidates = %+v", candidates)
	}
	if len(provider.searchQueries) != 1 || provider.searchQueries[0] != "some episode" {
		t.Fatalf("search queries = %v", provider.searchQueries)
	}

	// Searching as an episode (the edit dialog's type override) uses TV search and
	// falls back to the item title when there is no parent show yet.
	tvCandidates, err := svc.SearchAs(ctx, item.ID, "episode", "", 0, "", "fake")
	if err != nil {
		t.Fatalf("search as episode: %v", err)
	}
	if len(tvCandidates) != 1 || tvCandidates[0].MediaType != "tv" {
		t.Fatalf("tv candidates = %+v", tvCandidates)
	}
	if got := provider.searchQueries[len(provider.searchQueries)-1]; got != "some episode" {
		t.Fatalf("episode search query = %q", got)
	}

	if err := svc.Match(ctx, item.ID, "42", "", 0, 0, "fake"); err != nil {
		t.Fatalf("match home video: %v", err)
	}
	got, err := st.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Matched Title" {
		t.Fatalf("title after match = %q", got.Title)
	}
}
