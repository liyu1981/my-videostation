package httpapi

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/store"
)

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ItemFilter{
		LibraryID:      int64Query(r, "libraryId"),
		Kind:           q.Get("kind"),
		ParentID:       int64Query(r, "parentId"),
		Query:          q.Get("q"),
		Sort:           q.Get("sort"),
		Category:       q.Get("category"),
		Genre:          q.Get("genre"),
		Year:           intQuery(r, "year", 0),
		Folder:         q.Get("folder"),
		Page:           intQuery(r, "page", 1),
		PerPage:        intQuery(r, "perPage", 60),
		MetadataStatus: q.Get("metadataStatus"),
	}
	if v := q.Get("watched"); v == "true" || v == "false" {
		watched := v == "true"
		f.Watched = &watched
	}
	items, total, err := s.store.ListItems(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	perPage := f.PerPage
	if perPage <= 0 || perPage > 200 {
		perPage = 60
	}
	pages := (total + perPage - 1) / perPage
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      s.itemDTOs(items),
		"page":       max(f.Page, 1),
		"perPage":    perPage,
		"total":      total,
		"totalPages": pages,
	})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	home, err := s.store.GetHome(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	type shelfDTO struct {
		Title string    `json:"title"`
		Kind  string    `json:"kind"`
		Items []itemDTO `json:"items"`
	}
	out := map[string]any{
		"continue": s.itemDTOs(home.Continue),
		"recent":   s.itemDTOs(home.Recent),
	}
	shelves := make([]shelfDTO, 0, len(home.Shelves))
	for _, sh := range home.Shelves {
		shelves = append(shelves, shelfDTO{Title: sh.Title, Kind: sh.Kind, Items: s.itemDTOs(sh.Items)})
	}
	out["shelves"] = shelves
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []itemDTO{}, "query": ""})
		return
	}
	items, err := s.store.Search(r.Context(), q, intQuery(r, "limit", 40))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.itemDTOs(items), "query": q})
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleItemChildren(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		switch it.Kind {
		case store.KindShow:
			kind = store.KindSeason
		case store.KindSeason:
			kind = store.KindEpisode
		default:
			writeJSON(w, http.StatusOK, map[string]any{"items": []itemDTO{}})
			return
		}
	}
	items, _, err := s.store.ListItems(r.Context(), store.ItemFilter{
		ParentID: id, Kind: kind, Page: 1, PerPage: 200, Sort: "title",
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.itemDTOs(items)})
}

func (s *Server) handleItemNeighbors(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	prev, next, err := s.store.Neighbors(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	body := map[string]any{}
	if prev != nil {
		body["prev"] = s.itemDTO(*prev)
	}
	if next != nil {
		body["next"] = s.itemDTO(*next)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePatchItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var patch store.ItemPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	it, err := s.store.UpdateItem(r.Context(), id, patch)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "item.update", "item:"+strconv.FormatInt(id, 10), it.Title)
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("item updated", "id", id)
	writeJSON(w, http.StatusOK, detail)
}

type progressInput struct {
	PositionMS int64 `json:"positionMs"`
	DurationMS int64 `json:"durationMs"`
	Played     bool  `json:"played"`
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in progressInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if err := s.store.SetProgress(r.Context(), id, in.PositionMS, in.DurationMS, in.Played); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleArt(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
			return
		}
		s.serveItemArt(w, r, id, kind)
	}
}

// serveItemArt serves an item's poster/backdrop (thumbnail variant applied).
// Shared by the SPA API and the Jellyfin adapter.
func (s *Server) serveItemArt(w http.ResponseWriter, r *http.Request, itemID int64, kind string) {
	it, err := s.store.GetItem(r.Context(), itemID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artPath := it.PosterPath
	if kind == "backdrop" {
		artPath = it.BackdropPath
	}
	if artPath == "" {
		http.NotFound(w, r)
		return
	}
	ok, err := s.withinLibrary(r, artPath)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	s.serveArt(w, r, kind, artPath)
}

func (s *Server) withinLibrary(r *http.Request, p string) (bool, error) {
	if !filepath.IsAbs(p) {
		// Older rows stored paths relative to the process cwd (e.g. "data/cache/...").
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	clean := filepath.Clean(p)
	// Cached artwork (provider downloads, .vsmeta embeds) lives under <data>/cache.
	if s.cacheDir != "" {
		if cache, err := filepath.Abs(s.cacheDir); err == nil {
			if rel, err := filepath.Rel(cache, clean); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true, nil
			}
		}
	}
	libs, err := s.store.ListLibraries(r.Context())
	if err != nil {
		return false, err
	}
	for _, lib := range libs {
		for _, lp := range lib.Paths {
			root := filepath.Clean(lp.Path)
			rel, err := filepath.Rel(root, clean)
			if err != nil {
				continue
			}
			if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Server) handleItemDuplicate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	dup, err := s.store.FindDuplicateLeaf(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if dup == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": dup})
}

type mergeInput struct {
	FromID int64 `json:"fromId"`
}

func (s *Server) handleItemMerge(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in mergeInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.FromID == 0 || in.FromID == id {
		writeError(w, http.StatusBadRequest, "bad_request", "fromId is required")
		return
	}
	if err := s.store.MergeItems(r.Context(), id, in.FromID); err != nil {
		s.fail(w, err)
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type keepSeparateInput struct {
	OtherID int64 `json:"otherId"`
}

func (s *Server) handleKeepSeparate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in keepSeparateInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.OtherID == 0 || in.OtherID == id {
		writeError(w, http.StatusBadRequest, "bad_request", "otherId is required")
		return
	}
	if err := s.store.KeepSeparate(r.Context(), id, in.OtherID); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type applyLocalInput struct {
	SourceID int64 `json:"sourceId"`
}

func (s *Server) handleApplyLocal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in applyLocalInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.SourceID == 0 || in.SourceID == id {
		writeError(w, http.StatusBadRequest, "bad_request", "sourceId is required")
		return
	}
	if err := s.store.CopyMetadata(r.Context(), id, in.SourceID); err != nil {
		s.fail(w, err)
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// handleRescanItem queues a folder-scoped rescan for one item: the directories
// holding its files and its descendants' files (a show rescans its whole tree, a
// season its episode folder). Only files under that folder are re-walked — the
// rest of the library is untouched, so it is much cheaper than a library scan.
func (s *Server) handleRescanItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	var in struct {
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
	}
	dirs, err := s.store.ItemScopeDirs(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(dirs) == 0 {
		writeError(w, http.StatusConflict, "no_files", "item has no files to rescan")
		return
	}
	scope := commonDir(dirs)
	// A show's files live in season folders; rescan the show folder so a brand-new
	// season is picked up too, not just changes inside the existing one.
	if it.Kind == store.KindShow {
		if lib, err := s.store.GetLibrary(r.Context(), it.LibraryID); err == nil {
			scope = showScopeDir(lib, scope)
		}
	}
	job, err := s.jobs.Enqueue(r.Context(), "scan", map[string]any{
		"libraryId": it.LibraryID,
		"force":     in.Force,
		"scope":     scope,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "item.rescan", "item:"+strconv.FormatInt(id, 10), jobAction(in.Force))
	writeJSON(w, http.StatusAccepted, job)
}

// commonDir returns the deepest directory containing every path in dirs.
func commonDir(dirs []string) string {
	base := filepath.Clean(dirs[0])
	for _, d := range dirs[1:] {
		d = filepath.Clean(d)
		for {
			rel, err := filepath.Rel(base, d)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				break // d sits under base, so base already contains both
			}
			parent := filepath.Dir(base)
			if parent == base {
				break // filesystem root
			}
			base = parent
		}
	}
	return base
}

// seasonDirPattern matches the folder names scanners see between a show and its
// episodes. Used to climb from an episode folder to the show folder.
var seasonDirPattern = regexp.MustCompile(`(?i)^(season|series|specials?|s\d{1,3}|staffel|temporada|saison|seizoen)\b`)

// showScopeDir turns the folder that holds a show's episodes into the show folder
// itself, stopping at the library root. Flat shows (files directly in the show
// folder) are returned unchanged.
func showScopeDir(lib *store.Library, dir string) string {
	dir = filepath.Clean(dir)
	roots := make([]string, 0, len(lib.Paths))
	for _, p := range lib.Paths {
		roots = append(roots, filepath.Clean(p.Path))
	}
	for {
		if !seasonDirPattern.MatchString(filepath.Base(dir)) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		for _, root := range roots {
			if parent == root || dir == root {
				return dir // never climb out of the library root
			}
		}
		dir = parent
	}
}

// handleReclassifyItem changes an item's type (movie / TV episode / home video).
// Episode assignment creates or reuses the show and season in the same library;
// the choice survives rescans.
func (s *Server) handleReclassifyItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in store.ReclassifyInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	it, err := s.store.ReclassifyItem(r.Context(), id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "item.reclassify", "item:"+strconv.FormatInt(id, 10), in.Kind)
	detail, err := s.itemDetail(r.Context(), it)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
