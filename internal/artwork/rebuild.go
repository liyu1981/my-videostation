package artwork

import (
	"context"
	"fmt"

	"github.com/liyu1981/myvideostation/internal/store"
)

// RebuildJob pre-generates every thumbnail variant. It is the body of the
// `thumbnails` job; report receives 0-100 progress plus a short message.
func RebuildJob(ctx context.Context, st *store.Store, cacheDir string, report func(float64, string)) error {
	if report == nil {
		report = func(float64, string) {}
	}
	values, err := st.Settings(ctx)
	if err != nil {
		return err
	}
	opts := FromSettings(values)
	if !opts.Thumbnails {
		report(100, "thumbnail generation is disabled")
		return nil
	}
	refs, err := st.ItemsWithArt(ctx)
	if err != nil {
		return err
	}
	generated, failed := 0, 0
	for i, ref := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, art := range []struct{ kind, path string }{
			{"poster", ref.PosterPath},
			{"backdrop", ref.BackdropPath},
		} {
			if art.path == "" || !Supported(art.path) {
				continue
			}
			if _, err := Variant(cacheDir, art.path, WidthFor(opts, art.kind), opts.Quality); err != nil {
				failed++
				continue
			}
			generated++
		}
		report(float64(i+1)/float64(len(refs))*100,
			fmt.Sprintf("thumbnail %d/%d (%d generated)", i+1, len(refs), generated))
	}
	report(100, fmt.Sprintf("%d thumbnail(s) generated, %d failed", generated, failed))
	return nil
}
