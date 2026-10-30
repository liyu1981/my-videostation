// Package scanner walks library paths, classifies files and persists media items.
package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/liyu1981/myvideostation/internal/media"
	"github.com/liyu1981/myvideostation/internal/naming"
	"github.com/liyu1981/myvideostation/internal/store"
)

// Scanner indexes libraries.
type Scanner struct {
	store *store.Store
	probe *media.Prober
	log   *slog.Logger
}

// New creates a scanner. probe may be nil (technical metadata is then skipped).
func New(st *store.Store, probe *media.Prober, log *slog.Logger) *Scanner {
	return &Scanner{store: st, probe: probe, log: log}
}

// Stats summarises one library scan.
type Stats struct {
	Discovered   int `json:"discovered"`
	Indexed      int `json:"indexed"`
	Created      int `json:"created"`
	Probed       int `json:"probed"`
	Unchanged    int `json:"unchanged"`
	Extras       int `json:"extras"`
	Unidentified int `json:"unidentified"`
	Errors       int `json:"errors"`
}

func (s Stats) String() string {
	return fmt.Sprintf("%d files: %d new, %d probed, %d unchanged, %d extras, %d unidentified, %d errors",
		s.Discovered, s.Created, s.Probed, s.Unchanged, s.Extras, s.Unidentified, s.Errors)
}

type foundFile struct {
	path  string
	rel   string
	size  int64
	mtime int64
	inode int64
}

var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, "#snapshot": true, ".trash": true,
	".@__thumb": true, "lost+found": true, "node_modules": true,
	"$recycle.bin": true, "system volume information": true, ".git": true,
}

// ScanLibrary indexes one library, reporting progress 0-100.
func (sc *Scanner) ScanLibrary(ctx context.Context, libraryID int64, report func(float64, string)) (Stats, error) {
	var stats Stats
	lib, err := sc.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return stats, fmt.Errorf("load library: %w", err)
	}
	if report == nil {
		report = func(float64, string) {}
	}

	if sc.probe == nil {
		sc.log.Warn("ffprobe not found: indexing without technical metadata (run `pnpm bootstrap`)")
	}

	known, err := sc.store.FileStats(ctx, libraryID)
	if err != nil {
		return stats, err
	}
	if err := sc.store.ResetMissing(ctx, libraryID); err != nil {
		return stats, err
	}

	// Discover everything first so progress is accurate.
	var files []foundFile
	for _, p := range lib.Paths {
		report(0, "walking "+p.Path)
		got, err := sc.discover(ctx, lib.Type, p.Path)
		if err != nil {
			sc.log.Warn("walk failed", "path", p.Path, "err", err)
			stats.Errors++
			continue
		}
		files = append(files, got...)
	}
	stats.Discovered = len(files)
	if len(files) == 0 {
		if err := sc.store.DeleteOrphanItems(ctx, libraryID); err != nil {
			return stats, err
		}
		report(100, "no video files found")
		return stats, nil
	}

	batch := make([]store.ScannedFile, 0, 128)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		created, err := sc.store.ApplyScanBatch(ctx, batch)
		if err != nil {
			return err
		}
		stats.Created += created
		batch = batch[:0]
		return nil
	}

	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		item, ok := sc.classify(lib.Type, f, &stats)
		item.LibraryID = libraryID
		if ok {
			prev, seen := known[f.path]
			changed := !seen || prev.Size != f.size || prev.Mtime != f.mtime
			if changed && sc.probe != nil {
				tech, err := sc.probe.Probe(ctx, f.path)
				if err != nil {
					sc.log.Warn("probe failed", "file", f.path, "err", err)
					stats.Errors++
				} else {
					item.Probed = true
					item.Container = tech.Container
					item.DurationMS = tech.DurationMS
					item.Bitrate = tech.Bitrate
					item.Width, item.Height = tech.Width, tech.Height
					item.VideoCodec = tech.VideoCodec
					item.HDR = tech.HDR
					for _, a := range tech.Audio {
						item.Audio = append(item.Audio, store.AudioStream(a))
					}
					for _, sub := range tech.Subtitles {
						item.Subtitle = append(item.Subtitle, store.SubtitleStream(sub))
					}
					stats.Probed++
				}
			} else if !changed {
				stats.Unchanged++
			}
			if changed && item.PosterPath == "" {
				item.PosterPath, item.BackdropPath = media.FindLocalArt(f.path)
			}
			batch = append(batch, item)
			stats.Indexed++
		}
		if len(batch) >= 100 {
			if err := flush(); err != nil {
				return stats, err
			}
			pct := float64(i+1) / float64(len(files)) * 95
			report(pct, fmt.Sprintf("indexing %d/%d", i+1, len(files)))
		}
	}
	if err := flush(); err != nil {
		return stats, err
	}
	if err := sc.store.DeleteOrphanItems(ctx, libraryID); err != nil {
		return stats, err
	}
	for _, p := range lib.Paths {
		if err := sc.store.MarkScanned(ctx, p.ID); err != nil {
			return stats, err
		}
	}
	report(100, stats.String())
	return stats, nil
}

func (sc *Scanner) classify(libraryType string, f foundFile, stats *Stats) (store.ScannedFile, bool) {
	res := naming.Parse(libraryType, f.path)
	item := store.ScannedFile{
		Path:    f.path,
		RelPath: f.rel,
		Size:    f.size,
		Mtime:   f.mtime,
		Inode:   f.inode,
	}

	switch res.Kind {
	case naming.KindExtra:
		stats.Extras++
		return store.ScannedFile{}, false
	case naming.KindUnknown, "":
		stats.Unidentified++
		sc.log.Debug("unidentified file", "file", f.path)
		return store.ScannedFile{}, false
	case naming.KindEpisode:
		item.Kind = store.KindEpisode
		item.ShowTitle = res.Title
		item.Year = res.Year
		item.SeasonNumber = res.Season
		item.EpisodeNumber = res.Episode
		item.EpisodeTitle = res.EpisodeTitle
		if item.SeasonNumber == 0 {
			item.SeasonNumber = 1
		}
		if item.EpisodeNumber == 0 {
			stats.Unidentified++
			return store.ScannedFile{}, false
		}
	case naming.KindHomeVideo:
		item.Kind = store.KindHomeVideo
		item.Title = res.Title
		item.Year = res.Year
	default: // movie
		item.Kind = store.KindMovie
		item.Title = res.Title
		item.Year = res.Year
	}
	if item.Title == "" && item.ShowTitle == "" {
		stats.Unidentified++
		return store.ScannedFile{}, false
	}
	return item, true
}

func (sc *Scanner) discover(ctx context.Context, libraryType, root string) ([]foundFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	var out []foundFile
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			sc.log.Debug("walk error", "path", path, "err", err)
			return nil // keep going
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (skipDirs[strings.ToLower(name)] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !naming.IsVideoFile(name) {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = name
		}
		f := foundFile{
			path:  path,
			rel:   rel,
			size:  st.Size(),
			mtime: st.ModTime().Unix(),
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			f.inode = int64(sys.Ino)
		}
		out = append(out, f)
		return nil
	})
	return out, err
}
