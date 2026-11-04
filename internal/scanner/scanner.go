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
	"sync"
	"sync/atomic"
	"time"

	"github.com/liyu1981/myvideostation/internal/media"
	"github.com/liyu1981/myvideostation/internal/naming"
	"github.com/liyu1981/myvideostation/internal/scanrules"
	"github.com/liyu1981/myvideostation/internal/sidecar"
	"github.com/liyu1981/myvideostation/internal/store"
)

// Scanner indexes libraries.
type Scanner struct {
	store    *store.Store
	probe    *media.Prober
	log      *slog.Logger
	probeers int
	cacheDir string
	logMu    sync.Mutex
	jobLog   func(level, msg string)
}

// New creates a scanner. probe may be nil (technical metadata is then skipped).
func New(st *store.Store, probe *media.Prober, log *slog.Logger) *Scanner {
	return &Scanner{store: st, probe: probe, log: log, probeers: 4}
}

// SetCacheDir enables caching of embedded .vsmeta artwork under <dir>/images.
func (sc *Scanner) SetCacheDir(dir string) { sc.cacheDir = dir }

// SetJobLogger routes scanner warnings into the current job's persisted log.
func (sc *Scanner) SetJobLogger(fn func(level, msg string)) {
	sc.logMu.Lock()
	sc.jobLog = fn
	sc.logMu.Unlock()
}

func (sc *Scanner) logf(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	switch level {
	case "warn":
		sc.log.Warn(msg)
	case "error":
		sc.log.Error(msg)
	default:
		sc.log.Info(msg)
	}
	sc.logMu.Lock()
	fn := sc.jobLog
	sc.logMu.Unlock()
	if fn != nil {
		fn(level, msg)
	}
}

// Stats summarises one library scan.
type Stats struct {
	Discovered   int `json:"discovered"`
	Indexed      int `json:"indexed"`
	Created      int `json:"created"`
	Probed       int `json:"probed"`
	Unchanged    int `json:"unchanged"`
	Extras       int `json:"extras"`
	Samples      int `json:"samples"`
	Unidentified int `json:"unidentified"`
	LocalMeta    int `json:"localMeta"`
	Errors       int `json:"errors"`
	Skipped      int `json:"skipped"`
}

func (s Stats) String() string {
	out := fmt.Sprintf("%d files: %d new, %d probed, %d unchanged, %d extras, %d samples, %d unidentified, %d local metadata, %d errors",
		s.Discovered, s.Created, s.Probed, s.Unchanged, s.Extras, s.Samples, s.Unidentified, s.LocalMeta, s.Errors)
	if s.Skipped > 0 {
		out += fmt.Sprintf(", %d skipped", s.Skipped)
	}
	return out
}

type foundFile struct {
	path  string
	rel   string
	root  string
	size  int64
	mtime int64
	inode int64
}

var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, "#snapshot": true, ".trash": true,
	".@__thumb": true, "lost+found": true, "node_modules": true,
	"$recycle.bin": true, "system volume information": true, ".git": true,
}

// planned pairs a discovered file with the item it will become.
type planned struct {
	file foundFile
	item store.ScannedFile
}

// ScanLibrary indexes one library incrementally, reporting progress 0-100.
func (sc *Scanner) ScanLibrary(ctx context.Context, libraryID int64, report func(float64, string)) (Stats, error) {
	return sc.ScanLibraryForce(ctx, libraryID, false, report)
}

// ScanLibraryForce indexes one library. With force=true every file is re-probed
// (and sidecar metadata re-applied), regardless of size/mtime.
func (sc *Scanner) ScanLibraryForce(ctx context.Context, libraryID int64, force bool, report func(float64, string)) (Stats, error) {
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

	rules := scanrules.New(scanrules.Options{
		ExcludeGlobs:  lib.ExcludeGlobs,
		IncludeGlobs:  lib.IncludeGlobs,
		MinFileSizeMB: lib.MinFileSizeMB,
		SkipExtras:    lib.SkipExtras,
	})

	report(0, "walking folders")
	var files []foundFile
	for _, p := range lib.Paths {
		sc.logf("info", "walking %s", p.Path)
		got, err := sc.discover(ctx, p.Path, rules, &stats)
		if err != nil {
			sc.logf("warn", "walk failed: %s: %v", p.Path, err)
			stats.Errors++
			continue
		}
		files = append(files, got...)
	}
	stats.Discovered = len(files)
	report(3, fmt.Sprintf("%d video files found", len(files)))

	plans := sc.classify(lib.Type, files, rules.SkipExtras(), &stats)
	sc.logf("info", "classified %d items (extras %d, samples %d, unidentified %d)",
		len(plans), stats.Extras, stats.Samples, stats.Unidentified)
	for i := range plans {
		plans[i].item.LibraryID = libraryID
		sc.attachSidecars(&plans[i], &stats)
	}
	report(8, fmt.Sprintf("%d items to index", len(plans)))

	// Probe changed files in parallel (I/O bound), then write sequentially.
	results := make([]*media.Technical, len(plans))
	probeFailed := make([]bool, len(plans))
	needs := make([]int, 0, len(plans))
	for i := range plans {
		prev, seen := known[plans[i].file.path]
		if force || !seen || prev.Size != plans[i].file.size || prev.Mtime != plans[i].file.mtime {
			needs = append(needs, i)
		} else {
			stats.Unchanged++
		}
	}
	if sc.probe != nil && len(needs) > 0 {
		sc.probeAll(ctx, plans, needs, results, probeFailed, &stats, func(done, idx int) {
			pct := 8 + float64(done)/float64(len(needs))*67
			report(pct, fmt.Sprintf("probing %d/%d: %s", done, len(needs), plans[idx].file.path))
		})
	}
	for i := range plans {
		if tech := results[i]; tech != nil {
			applyTech(&plans[i].item, tech)
			stats.Probed++
		} else if probeFailed[i] {
			stats.Errors++
		}
		if plans[i].item.PosterPath == "" && results[i] != nil {
			plans[i].item.PosterPath, plans[i].item.BackdropPath = media.FindLocalArt(plans[i].file.path)
		}
		applySubtitles(&plans[i].item, plans[i].file.path)
	}

	// Small, time-bounded batches so the library populates while scanning, plus a
	// per-file fallback so one bad row cannot abort the whole scan.
	batch := make([]store.ScannedFile, 0, 32)
	lastFlush := time.Now()
	flush := func(force bool) error {
		if len(batch) == 0 {
			return nil
		}
		if !force && len(batch) < 8 && time.Since(lastFlush) < 400*time.Millisecond {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		created, err := sc.store.ApplyScanBatch(ctx, batch)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			sc.logf("warn", "batch failed (%d files), retrying per file: %v", len(batch), err)
			created = 0
			for _, f := range batch {
				c, ferr := sc.store.ApplyScanBatch(ctx, []store.ScannedFile{f})
				if ferr != nil {
					sc.logf("warn", "skipped %s: %v", f.Path, ferr)
					stats.Errors++
					stats.Skipped++
					continue
				}
				created += c
			}
		}
		stats.Created += created
		lastFlush = time.Now()
		batch = batch[:0]
		return nil
	}
	for i := range plans {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		batch = append(batch, plans[i].item)
		stats.Indexed++
		before := len(batch)
		if err := flush(false); err != nil {
			return stats, err
		}
		if before > 0 && len(batch) == 0 {
			report(75+float64(i+1)/float64(len(plans))*23,
				fmt.Sprintf("indexing %d/%d (%d added): %s", i+1, len(plans), stats.Created, plans[i].file.path))
		}
	}
	if err := flush(true); err != nil {
		return stats, err
	}
	if err := sc.store.DeleteOrphanItems(ctx, libraryID); err != nil {
		return stats, err
	}
	if merged, err := sc.store.MergePartDuplicates(ctx, libraryID); err != nil {
		sc.logf("warn", "duplicate part merge failed: %v", err)
	} else if merged > 0 {
		sc.logf("info", "merged %d duplicate part item(s)", merged)
	}
	sc.reconcileRuleVisibility(ctx, lib, rules)
	for _, p := range lib.Paths {
		if err := sc.store.MarkScanned(ctx, p.ID); err != nil {
			return stats, err
		}
	}
	report(100, stats.String())
	return stats, nil
}

// reconcileRuleVisibility hides items whose every file is now excluded by the
// library's rules, and unhides them when a file is indexed again. Deleted files
// (not excluded by a rule) keep the existing "missing" behaviour.
func (sc *Scanner) reconcileRuleVisibility(ctx context.Context, lib *store.Library, rules scanrules.Rules) {
	refs, err := sc.store.LibraryFileRefs(ctx, lib.ID)
	if err != nil {
		sc.logf("warn", "rule visibility check failed: %v", err)
		return
	}
	roots := make([]string, 0, len(lib.Paths))
	for _, p := range lib.Paths {
		roots = append(roots, filepath.Clean(p.Path))
	}
	type itemState struct {
		kind        string
		visible     bool
		allExcluded bool
		hidden      bool
		seen        bool
	}
	items := map[int64]*itemState{}
	for _, r := range refs {
		st := items[r.ItemID]
		if st == nil {
			st = &itemState{kind: r.Kind, allExcluded: true, hidden: r.Hidden}
			items[r.ItemID] = st
		}
		st.seen = true
		if !r.Missing {
			st.visible = true
			continue
		}
		rel := ""
		for _, root := range roots {
			if r.Path == root {
				rel = ""
				break
			}
			if strings.HasPrefix(r.Path, root+string(filepath.Separator)) {
				rel = strings.TrimPrefix(r.Path, root+string(filepath.Separator))
				break
			}
		}
		skip, _ := rules.SkipFile(rel, r.Size)
		if !skip {
			st.allExcluded = false
		}
	}
	var hiddenN, shownN int
	for id, st := range items {
		if st.kind == store.KindExtra || !st.seen {
			continue // extras stay hidden by design
		}
		want := !st.visible && st.allExcluded
		if want == st.hidden {
			continue
		}
		if err := sc.store.SetItemHidden(ctx, id, want); err != nil {
			sc.logf("warn", "could not update visibility for item %d: %v", id, err)
			continue
		}
		if want {
			hiddenN++
		} else {
			shownN++
		}
	}
	if hiddenN > 0 || shownN > 0 {
		sc.logf("info", "scan rules hid %d item(s) and restored %d", hiddenN, shownN)
	}
}

// attachSidecars loads .nfo/.vsmeta metadata for one planned file.
func (sc *Scanner) attachSidecars(p *planned, stats *Stats) {
	res := sidecar.Load(p.file.path, sc.cacheDir)
	if res == nil {
		return
	}
	if res.File != nil {
		p.item.LocalMeta = toLocalMeta(res.File)
		stats.LocalMeta++
	}
	if res.Show != nil {
		p.item.ShowMeta = toLocalMeta(res.Show)
	}
}

func toLocalMeta(m *sidecar.Meta) *store.LocalMetadata {
	if m == nil {
		return nil
	}
	return &store.LocalMetadata{
		Source:        m.Source,
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		SortTitle:     m.SortTitle,
		Tagline:       m.Tagline,
		Overview:      m.Overview,
		Certification: m.Certification,
		Year:          m.Year,
		ReleaseDate:   m.ReleaseDate,
		Rating:        m.Rating,
		RuntimeMS:     m.RuntimeMS,
		Genres:        m.Genres,
		Cast:          m.Cast,
		Directors:     m.Directors,
		Writers:       m.Writers,
		PosterPath:    m.PosterPath,
		BackdropPath:  m.BackdropPath,
	}
}

func (sc *Scanner) probeAll(ctx context.Context, plans []planned, needs []int,
	results []*media.Technical, failed []bool, stats *Stats, progress func(done, idx int)) {
	sem := make(chan struct{}, sc.probeers)
	var wg sync.WaitGroup
	var done atomic.Int64
	for _, idx := range needs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			tech, err := sc.probe.Probe(ctx, plans[i].file.path)
			if err != nil {
				sc.logf("warn", "probe failed: %s: %v", plans[i].file.path, err)
				failed[i] = true
			} else {
				results[i] = tech
			}
			progress(int(done.Add(1)), i)
		}(idx)
	}
	wg.Wait()
}

func applySubtitles(item *store.ScannedFile, path string) {
	found := media.FindSubtitles(path)
	item.Subtitles = make([]store.SubtitleFile, 0, len(found))
	for _, sub := range found {
		item.Subtitles = append(item.Subtitles, store.SubtitleFile{
			Path: sub.Path, Language: sub.Language, Label: sub.Label, Format: sub.Format,
		})
	}
}

func applyTech(item *store.ScannedFile, tech *media.Technical) {
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
}

// classify turns discovered files into items, resolving extra parents in a second pass.
func (sc *Scanner) classify(libraryType string, files []foundFile, skipExtras bool, stats *Stats) []planned {
	plans := make([]planned, 0, len(files))
	dirHasMedia := map[string]int{}
	type extraCandidate struct {
		file foundFile
		res  naming.Result
	}
	var extras []extraCandidate

	for _, f := range files {
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
			if res.Extra == "sample" {
				stats.Samples++
				continue
			}
			if skipExtras {
				// Library rule: trailers/featurettes are not indexed at all.
				stats.Skipped++
				continue
			}
			extras = append(extras, extraCandidate{file: f, res: res})
			continue
		case naming.KindUnknown, "":
			// Best effort: keep the file in the DB as an editable placeholder even
			// when the filename cannot be parsed (it shows up as unmatched).
			stats.Unidentified++
			title := strings.TrimSpace(res.Title)
			if title == "" {
				title = placeholderTitle(f.path)
			}
			sc.logf("warn", "unidentified file (added as placeholder): %s", f.path)
			item.Kind = store.KindHomeVideo
			item.Title = title
			item.Year = res.Year
		case naming.KindEpisode:
			item.Kind = store.KindEpisode
			item.ShowTitle = res.Title
			item.Year = res.Year
			item.SeasonNumber = res.Season
			item.EpisodeNumber = res.Episode
			item.EpisodeEnd = res.EpisodeEnd
			item.EpisodeTitle = res.EpisodeTitle
			if item.SeasonNumber == 0 {
				item.SeasonNumber = 1
			}
			if item.EpisodeNumber == 0 {
				stats.Unidentified++
				continue
			}
		case naming.KindHomeVideo:
			item.Kind = store.KindHomeVideo
			item.Title = res.Title
			item.Year = res.Year
		default:
			item.Kind = store.KindMovie
			item.Title = res.Title
			item.Year = res.Year
		}
		if item.Title == "" && item.ShowTitle == "" {
			stats.Unidentified++
			continue
		}
		dirHasMedia[filepath.Dir(f.path)]++
		plans = append(plans, planned{file: f, item: item})
	}

	for _, ex := range extras {
		parent := nearestDirWithMedia(filepath.Dir(ex.file.path), ex.file.root, dirHasMedia)
		plans = append(plans, planned{file: ex.file, item: store.ScannedFile{
			Path:      ex.file.path,
			RelPath:   ex.file.rel,
			Size:      ex.file.size,
			Mtime:     ex.file.mtime,
			Inode:     ex.file.inode,
			Kind:      store.KindExtra,
			Title:     extraLabel(ex.res.Extra),
			ParentDir: parent,
		}})
		stats.Extras++
	}
	return plans
}

// placeholderTitle derives a readable title from a filename when parsing fails.
func placeholderTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.TrimSpace(strings.NewReplacer("_", " ", ".", " ").Replace(base))
	if base == "" {
		return "Unidentified video"
	}
	return base
}

func extraLabel(kind string) string {
	switch kind {
	case "trailer":
		return "Trailer"
	case "featurette":
		return "Featurette"
	case "deleted_scenes":
		return "Deleted Scenes"
	case "behind_the_scenes":
		return "Behind the Scenes"
	case "interview":
		return "Interview"
	case "bloopers":
		return "Bloopers"
	default:
		return "Extra"
	}
}

// nearestDirWithMedia walks up from dir until a directory containing real media is found.
func nearestDirWithMedia(dir, root string, dirHasMedia map[string]int) string {
	root = filepath.Clean(root)
	cur := filepath.Clean(dir)
	for i := 0; i < 4; i++ {
		if dirHasMedia[cur] > 0 {
			return cur
		}
		if cur == root || len(cur) <= len(root) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

// relOf returns the slash-separated path of child relative to root.
func relOf(root, child string) string {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return filepath.Base(child)
	}
	return rel
}

func (sc *Scanner) discover(ctx context.Context, root string, rules scanrules.Rules, stats *Stats) ([]foundFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	root = filepath.Clean(root)

	var out []foundFile
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			sc.log.Debug("walk error", "path", path, "err", err)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (skipDirs[strings.ToLower(name)] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			if path != root && rules.Excluded(relOf(root, path)) {
				stats.Skipped++
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
		rel := relOf(root, path)
		if skip, why := rules.SkipFile(rel, st.Size()); skip {
			stats.Skipped++
			sc.log.Debug("skip file", "path", path, "reason", why)
			return nil
		}
		f := foundFile{path: path, rel: rel, root: root, size: st.Size(), mtime: st.ModTime().Unix()}
		f.inode = inodeOf(st)
		out = append(out, f)
		return nil
	})
	return out, err
}
