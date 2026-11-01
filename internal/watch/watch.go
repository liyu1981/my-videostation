// Package watch watches local library folders with fsnotify and queues debounced
// scans. It is best-effort: network filesystems and exhausted inotify limits fall
// back to the scheduled scanner.
package watch

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/liyu1981/myvideostation/internal/jobs"
	"github.com/liyu1981/myvideostation/internal/store"
)

const (
	syncInterval = 20 * time.Second
	debounce     = 10 * time.Second
)

// Watcher keeps one fsnotify watcher per library and enqueues a scan after a
// quiet period. Libraries are re-synced every syncInterval so create/update/
// delete and the watch toggle are picked up without restarting the server.
type Watcher struct {
	store   *store.Store
	jobs    *jobs.Manager
	log     *slog.Logger
	mu      sync.Mutex
	active  map[int64]*libraryWatch
	skipped map[int64]bool
}

type libraryWatch struct {
	id       int64
	paths    []string
	fw       *fsnotify.Watcher
	stopCh   chan struct{}
	stopOnce sync.Once
	timerMu  sync.Mutex
	timer    *time.Timer
}

// New creates a watcher. Run starts the sync loop.
func New(st *store.Store, jm *jobs.Manager, log *slog.Logger) *Watcher {
	return &Watcher{store: st, jobs: jm, log: log, active: map[int64]*libraryWatch{}, skipped: map[int64]bool{}}
}

// Run watches libraries until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.sync(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.stopAll()
			return
		case <-ticker.C:
			w.sync(ctx)
		}
	}
}

func (w *Watcher) sync(ctx context.Context) {
	libs, err := w.store.ListLibraries(ctx)
	if err != nil {
		w.log.Warn("watcher library sync failed", "err", err)
		return
	}
	wanted := map[int64]store.Library{}
	for _, l := range libs {
		if l.WatchEnabled {
			wanted[l.ID] = l
		}
	}
	w.mu.Lock()
	for id, lw := range w.active {
		if _, ok := wanted[id]; !ok {
			lw.stop()
			delete(w.active, id)
		}
	}
	for id := range w.skipped {
		if _, ok := wanted[id]; !ok {
			delete(w.skipped, id)
		}
	}
	w.mu.Unlock()
	for _, lib := range wanted {
		if w.skipped[lib.ID] {
			continue
		}
		paths := localPaths(lib)
		if len(paths) == 0 {
			if !w.skipped[lib.ID] {
				w.log.Info("watcher skipped (network filesystem)", "library", lib.ID)
				w.skipped[lib.ID] = true
			}
			continue
		}
		delete(w.skipped, lib.ID)
		w.mu.Lock()
		lw, ok := w.active[lib.ID]
		w.mu.Unlock()
		if ok {
			if !samePaths(lw.paths, paths) {
				lw.stop()
				w.mu.Lock()
				delete(w.active, lib.ID)
				w.mu.Unlock()
				w.start(ctx, lib, paths)
			}
			continue
		}
		w.start(ctx, lib, paths)
	}
}

func (w *Watcher) start(ctx context.Context, lib store.Library, paths []string) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		w.log.Warn("watcher create failed", "library", lib.ID, "err", err)
		return
	}
	for _, p := range paths {
		if err := addRecursive(fw, p); err != nil {
			w.log.Warn("watcher add failed; scheduled scans continue", "library", lib.ID, "path", p, "err", err)
			_ = fw.Close()
			w.skipped[lib.ID] = true
			return
		}
	}
	lw := &libraryWatch{id: lib.ID, paths: paths, fw: fw, stopCh: make(chan struct{})}
	w.mu.Lock()
	w.active[lib.ID] = lw
	w.mu.Unlock()
	go w.loop(ctx, lw)
	w.log.Info("watching library", "library", lib.ID, "paths", len(paths))
}

func (w *Watcher) loop(ctx context.Context, lw *libraryWatch) {
	defer lw.stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-lw.stopCh:
			return
		case ev, ok := <-lw.fw.Events:
			if !ok {
				return
			}
			if ignoreEvent(ev.Name) {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if err := addRecursive(lw.fw, ev.Name); err != nil {
						w.log.Warn("watcher add directory failed", "library", lw.id, "path", ev.Name, "err", err)
					}
				}
			}
			lw.schedule(w)
		case err, ok := <-lw.fw.Errors:
			if !ok {
				return
			}
			w.log.Warn("library watcher error", "library", lw.id, "err", err)
			if isWatchLimit(err) {
				w.log.Warn("watcher disabled; scheduled scans continue", "library", lw.id)
				return
			}
		}
	}
}

func (w *Watcher) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, lw := range w.active {
		lw.stop()
		delete(w.active, id)
	}
}

func (lw *libraryWatch) stop() {
	lw.stopOnce.Do(func() {
		close(lw.stopCh)
		_ = lw.fw.Close()
		lw.timerMu.Lock()
		if lw.timer != nil {
			lw.timer.Stop()
		}
		lw.timerMu.Unlock()
	})
}

func (lw *libraryWatch) schedule(w *Watcher) {
	lw.timerMu.Lock()
	defer lw.timerMu.Unlock()
	if lw.timer != nil {
		lw.timer.Stop()
	}
	lw.timer = time.AfterFunc(debounce, func() { w.enqueue(lw.id) })
}

func (w *Watcher) enqueue(libraryID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	active, err := w.store.HasActiveJob(ctx, "scan", libraryID)
	if err != nil || active {
		return
	}
	if _, err := w.jobs.Enqueue(ctx, "scan", map[string]any{"libraryId": libraryID}); err != nil {
		w.log.Warn("watcher scan enqueue failed", "library", libraryID, "err", err)
		return
	}
	w.log.Info("watcher queued scan", "library", libraryID)
}

func localPaths(lib store.Library) []string {
	var out []string
	for _, p := range lib.Paths {
		if !isNetworkFS(p.Path) {
			out = append(out, p.Path)
		}
	}
	return out
}

func samePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func addRecursive(fw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && skipWatchDir(d.Name()) {
			return filepath.SkipDir
		}
		if err := fw.Add(path); err != nil {
			if isWatchLimit(err) {
				return err
			}
			return nil
		}
		return nil
	})
}

var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, ".@__thumb": true, "lost+found": true,
	"node_modules": true, "$recycle.bin": true, "system volume information": true, ".git": true,
}

func skipWatchDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return skipDirs[strings.ToLower(name)]
}

func ignoreEvent(name string) bool {
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") {
		return true
	}
	slash := filepath.ToSlash(name)
	for _, part := range strings.Split(slash, "/") {
		if skipDirs[strings.ToLower(part)] {
			return true
		}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tmp", ".part", ".crdownload", ".swp", ".swx", ".lock", ".partial":
		return true
	}
	return false
}

func isWatchLimit(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no space left on device") ||
		strings.Contains(msg, "too many open files") ||
		strings.Contains(msg, "too many watches")
}

// isNetworkFS reports whether path lives on a filesystem where inotify does not
// deliver remote changes (CIFS/SMB/NFS, Docker Desktop shares). Local bind mounts
// report their underlying ext4/btrfs/xfs type, so they stay watchable.
func isNetworkFS(path string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	best, fsType := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		sep := -1
		for i, f := range fields {
			if f == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+1 >= len(fields) || len(fields) < 5 {
			continue
		}
		mp := unescapeMount(fields[4])
		if abs == mp || strings.HasPrefix(abs, mp+"/") {
			if len(mp) >= len(best) {
				best, fsType = mp, fields[sep+1]
			}
		}
	}
	switch fsType {
	case "cifs", "smb3", "smbfs", "nfs", "nfs4", "9p", "virtiofs", "fuse", "fuseblk":
		return true
	}
	return strings.HasPrefix(fsType, "fuse.")
}

func unescapeMount(s string) string {
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(s)
}
