package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type fsListing struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent,omitempty"`
	Roots   []fsEntry `json:"roots"`
	Entries []fsEntry `json:"entries"`
}

// handleFSList lists the directories at one server-side path so the UI can offer
// a folder picker instead of asking users to type absolute paths. It is read-only
// and returns directories only.
func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		p = "/"
	}
	if !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "bad_request", "path must be absolute")
		return
	}
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			writeError(w, http.StatusNotFound, "not_found", "folder does not exist")
		case errors.Is(err, os.ErrPermission):
			writeError(w, http.StatusForbidden, "forbidden", "permission denied")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "cannot read folder")
		}
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not_a_directory", "path is not a directory")
		return
	}
	dirEntries, err := os.ReadDir(p)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			writeError(w, http.StatusForbidden, "forbidden", "permission denied")
		} else {
			writeError(w, http.StatusInternalServerError, "internal", "cannot read folder")
		}
		return
	}
	out := fsListing{Path: p, Roots: fsRoots(), Entries: make([]fsEntry, 0, len(dirEntries))}
	if p != "/" {
		out.Parent = filepath.Dir(p)
	}
	for _, e := range dirEntries {
		if skipFSEntry(e.Name()) {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&os.ModeSymlink != 0 {
			// Follow symlinked directories, but never fail the whole listing.
			if target, err := os.Stat(filepath.Join(p, e.Name())); err == nil && target.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			continue
		}
		out.Entries = append(out.Entries, fsEntry{Name: e.Name(), Path: filepath.Join(p, e.Name())})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

// skipFSEntry hides Synology/Windows metadata folders that are never libraries.
func skipFSEntry(name string) bool {
	switch name {
	case "@eaDir", "#recycle", "#snapshot", "@Recycle", "$RECYCLE.BIN", "System Volume Information":
		return true
	}
	return false
}

// fsRoots returns quick-jump locations. /volume* covers Synology volumes.
func fsRoots() []fsEntry {
	seen := map[string]bool{}
	out := make([]fsEntry, 0, 6)
	add := func(p string) {
		p = filepath.Clean(p)
		if seen[p] {
			return
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return
		}
		seen[p] = true
		name := filepath.Base(p)
		if p == "/" {
			name = "/"
		}
		out = append(out, fsEntry{Name: name, Path: p})
	}
	add("/")
	if volumes, err := filepath.Glob("/volume*"); err == nil {
		sort.Strings(volumes)
		for _, v := range volumes {
			add(v)
		}
	}
	for _, p := range []string{"/media", "/mnt", "/data", "/home"} {
		add(p)
	}
	return out
}
