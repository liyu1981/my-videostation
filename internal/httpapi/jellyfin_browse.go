package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
	"github.com/liyu1981/myvideostation/internal/store"
)

// registerJellyfinBrowseRoutes mounts the J1 discovery surface: home rows,
// facet endpoints and the item-relation stubs clients probe while rendering.
func (s *Server) registerJellyfinBrowseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /Items/Suggestions", s.anyAuth(s.handleJFSuggestions))
	mux.HandleFunc("GET /Items/Resume", s.anyAuth(s.handleJFResume))
	mux.HandleFunc("GET /Users/{userId}/Items/Resume", s.anyAuth(s.handleJFResume))
	mux.HandleFunc("GET /Items/Latest", s.anyAuth(s.handleJFLatest))
	mux.HandleFunc("GET /Users/{userId}/Items/Latest", s.anyAuth(s.handleJFLatest))
	mux.HandleFunc("GET /Shows/{itemId}/NextUp", s.anyAuth(s.handleJFNextUp))

	mux.HandleFunc("GET /Genres", s.anyAuth(s.handleJFGenres))
	mux.HandleFunc("GET /Users/{userId}/Genres", s.anyAuth(s.handleJFGenres))
	mux.HandleFunc("GET /Persons", s.anyAuth(s.handleJFPersons))
	mux.HandleFunc("GET /Studios", s.anyAuth(s.handleJFStudios))
	mux.HandleFunc("GET /Items/Filters", s.anyAuth(s.handleJFLegacyFilters))
	mux.HandleFunc("GET /Items/Filters2", s.anyAuth(s.handleJFFilters))

	mux.HandleFunc("GET /UserItems/Resume", s.anyAuth(s.handleJFResume))
	mux.HandleFunc("GET /Shows/Upcoming", s.anyAuth(s.handleJFUpcoming))
	mux.HandleFunc("GET /Shows/NextUp", s.anyAuth(s.handleJFNextUpAll))
	mux.HandleFunc("GET /Shows/{itemId}/Seasons", s.anyAuth(s.handleJFSeasons))
	mux.HandleFunc("GET /Items/{itemId}/Intros", s.anyAuth(s.handleJFEmptyQuery))
	mux.HandleFunc("GET /Videos/{itemId}/AdditionalParts", s.anyAuth(s.handleJFEmptyQuery))
	mux.HandleFunc("POST /DisplayPreferences/{name}", s.anyAuth(s.handleJFSaveDisplayPreferences))
	mux.HandleFunc("GET /socket", s.anyAuth(s.handleJFSocket))
	mux.HandleFunc("GET /Items/{itemId}/Similar", s.anyAuth(s.handleJFSimilar))
	mux.HandleFunc("GET /Items/{itemId}/ThemeMedia", s.anyAuth(s.handleJFThemeMedia))
	mux.HandleFunc("GET /Items/{itemId}/LocalTrailers", s.anyAuth(s.handleJFLocalTrailers))
	mux.HandleFunc("GET /Items/{itemId}/SpecialFeatures", s.anyAuth(s.handleJFSpecialFeatures))
	mux.HandleFunc("GET /Items/{itemId}/Images", s.anyAuth(s.handleJFImageList))
}

// jfBrowseContext resolves the token user and a user-scoped context.
func (s *Server) jfBrowseContext(w http.ResponseWriter, r *http.Request) (context.Context, bool) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}
	return s.userContext(r.Context(), me, ""), true
}

// ---- home rows -------------------------------------------------------------

// handleJFSuggestions answers /Items/Suggestions. We do not have a recommendation
// engine, so this returns recent unplayed items of the requested kinds (falling
// back to recent items when everything is watched).
func (s *Server) handleJFSuggestions(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := jfInt(q, "limit", 6)
	if limit <= 0 || limit > 100 {
		limit = 6
	}
	kinds := []string{}
	for _, t := range jfCSV(q, "type") {
		if k := jfKindForType(t); k != "" {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		kinds = []string{store.KindMovie, store.KindShow}
	}
	unplayed := false
	items, _, err := s.store.ListItems(ctx, store.ItemFilter{
		Kinds: kinds, Watched: &unplayed, Category: "recent_added", Page: 1, PerPage: limit,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(items) == 0 {
		items, _, err = s.store.ListItems(ctx, store.ItemFilter{
			Kinds: kinds, Category: "recent_added", Page: 1, PerPage: limit,
		})
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), items, false), TotalRecordCount: len(items),
	})
}

func (s *Server) handleJFResume(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	limit := jfInt(r.URL.Query(), "Limit", 12)
	if v := jfInt(r.URL.Query(), "limit", 0); v > 0 {
		limit = v
	}
	items, total, err := s.store.ListItems(ctx, store.ItemFilter{
		Kinds:     []string{store.KindMovie, store.KindEpisode, store.KindHomeVideo},
		Resumable: true,
		Category:  "recent_watched",
		Page:      1,
		PerPage:   limit,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), items, false), TotalRecordCount: total,
	})
}

func (s *Server) handleJFLatest(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := jfInt(q, "Limit", 20)
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	var libID int64
	libType := ""
	if raw := firstValue(q, "ParentId"); raw != "" {
		k, id, err := jellyfin.Decode(raw)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch k {
		case jellyfin.KindLibrary:
			libID = id
		case jellyfin.KindItem:
			it, err := s.store.GetItem(ctx, id)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			libID = it.LibraryID
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		lib, err := s.store.GetLibrary(ctx, libID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		libType = lib.Type
	}
	kinds := []string{store.KindMovie, store.KindHomeVideo}
	if libType == "tv" {
		kinds = []string{store.KindEpisode}
	}
	items, _, err := s.store.ListItems(ctx, store.ItemFilter{
		LibraryID: libID,
		Kinds:     kinds,
		Category:  "recent_added",
		Page:      1,
		PerPage:   limit,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), items, false))
}

func (s *Server) handleJFNextUp(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	seriesID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	show, err := s.store.GetItem(ctx, seriesID)
	if err != nil || show.Kind != store.KindShow {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	episodes, _, err := s.jfSeriesEpisodes(ctx, seriesID)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Find the furthest watched/in-progress episode, then the next one.
	last := -1
	for i := range episodes {
		if episodes[i].Played || episodes[i].PositionMS > 0 {
			last = i
		}
	}
	next := []store.Item{}
	switch {
	case last >= 0 && episodes[last].PositionMS > 0 && !episodes[last].Played:
		next = append(next, episodes[last]) // in-progress episode wins
	case last >= 0:
		if n, err := s.store.NextEpisode(ctx, episodes[last].ID); err == nil && n != nil {
			next = append(next, *n)
		}
	case len(episodes) > 0:
		next = append(next, episodes[0])
	}
	limit := jfInt(r.URL.Query(), "Limit", 1)
	if limit <= 0 {
		limit = 1
	}
	if len(next) > limit {
		next = next[:limit]
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), next, false), TotalRecordCount: len(next),
	})
}

// ---- facets ----------------------------------------------------------------

// jfFacetScope resolves the library and item kinds for facet endpoints.
func (s *Server) jfFacetScope(ctx context.Context, r *http.Request) (int64, []string, error) {
	libID := int64(0)
	libType := ""
	parentKind := ""
	if raw := firstValue(r.URL.Query(), "ParentId"); raw != "" {
		k, id, err := jellyfin.Decode(raw)
		if err != nil {
			return 0, nil, store.NewInvalid("invalid parent id")
		}
		switch k {
		case jellyfin.KindLibrary:
			lib, err := s.store.GetLibrary(ctx, id)
			if err != nil {
				return 0, nil, err
			}
			libID, libType = lib.ID, lib.Type
		case jellyfin.KindItem:
			it, err := s.store.GetItem(ctx, id)
			if err != nil {
				return 0, nil, err
			}
			libID, parentKind = it.LibraryID, it.Kind
			if lib, err := s.store.GetLibrary(ctx, libID); err == nil {
				libType = lib.Type
			}
		default:
			return 0, nil, store.NewInvalid("invalid parent id")
		}
	}
	kinds := []string{}
	for _, t := range jfCSV(r.URL.Query(), "IncludeItemTypes") {
		if k := jfKindForType(t); k != "" {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		switch {
		case parentKind == store.KindShow:
			kinds = []string{store.KindShow}
		case parentKind == store.KindSeason:
			kinds = []string{store.KindEpisode}
		case libType == "tv":
			kinds = []string{store.KindShow}
		case libType == "home_video", libType == "tv_recording":
			kinds = []string{store.KindHomeVideo}
		case libType == "movie":
			kinds = []string{store.KindMovie}
		}
	}
	return libID, kinds, nil
}

func (s *Server) handleJFGenres(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	libID, kinds, err := s.jfFacetScope(ctx, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	facets, err := s.store.FacetsForKinds(ctx, libID, kinds)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]jellyfin.NameGuidPair, 0, len(facets.Genres))
	for _, g := range facets.Genres {
		out = append(out, jellyfin.NameGuidPair{Name: g.Value, ID: jellyfin.GenreID(g.ID)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleJFPersons(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	refs, err := s.store.PersonRefs(ctx, firstValue(q, "searchTerm"), jfInt(q, "Limit", 100))
	if err != nil {
		s.fail(w, err)
		return
	}
	serverID := s.jellyfinServerID(ctx)
	out := make([]jellyfin.BaseItemDto, 0, len(refs))
	for _, p := range refs {
		out = append(out, jellyfin.BaseItemDto{
			Name: p.Name, ServerID: serverID, ID: jellyfin.PersonID(p.ID), Type: "Person",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleJFStudios(w http.ResponseWriter, r *http.Request) {
	// Studios are not part of our metadata model.
	writeJSON(w, http.StatusOK, []jellyfin.NameGuidPair{})
}

func (s *Server) jfQueryFilters(ctx context.Context, r *http.Request) (jellyfin.QueryFilters, error) {
	libID, kinds, err := s.jfFacetScope(ctx, r)
	if err != nil {
		return jellyfin.QueryFilters{}, err
	}
	facets, err := s.store.FacetsForKinds(ctx, libID, kinds)
	if err != nil {
		return jellyfin.QueryFilters{}, err
	}
	ratings, err := s.store.RatingRefs(ctx, libID, "")
	if err != nil {
		return jellyfin.QueryFilters{}, err
	}
	out := jellyfin.QueryFilters{
		Genres:          make([]jellyfin.NameGuidPair, 0, len(facets.Genres)),
		OfficialRatings: ratings,
		Years:           make([]int, 0, len(facets.Years)),
	}
	for _, g := range facets.Genres {
		out.Genres = append(out.Genres, jellyfin.NameGuidPair{Name: g.Value, ID: jellyfin.GenreID(g.ID)})
	}
	for _, y := range facets.Years {
		var n int
		if _, err := fmt.Sscanf(y.Value, "%d", &n); err == nil {
			out.Years = append(out.Years, n)
		}
	}
	return out, nil
}

func (s *Server) handleJFFilters(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	filters, err := s.jfQueryFilters(ctx, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, filters)
}

func (s *Server) handleJFLegacyFilters(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	filters, err := s.jfQueryFilters(ctx, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	genres := make([]string, 0, len(filters.Genres))
	for _, g := range filters.Genres {
		genres = append(genres, g.Name)
	}
	writeJSON(w, http.StatusOK, jellyfin.LegacyQueryFilters{
		Genres: genres, OfficialRatings: filters.OfficialRatings, Years: filters.Years,
	})
}

// ---- item relations / stubs ------------------------------------------------

// handleJFEmptyQuery answers relation endpoints that have no data in our model.
func (s *Server) handleJFEmptyQuery(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{Items: []jellyfin.BaseItemDto{}})
}

// handleJFUpcoming answers /Shows/Upcoming (we do not track air dates).
func (s *Server) handleJFUpcoming(w http.ResponseWriter, r *http.Request) {
	s.handleJFEmptyQuery(w, r)
}

// handleJFSaveDisplayPreferences accepts (and ignores) client display prefs.
func (s *Server) handleJFSaveDisplayPreferences(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleJFSocket rejects the WebSocket upgrade quickly (no realtime socket yet).
func (s *Server) handleJFSocket(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
}

// handleJFSeasons serves /Shows/{id}/Seasons.
func (s *Server) handleJFSeasons(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	seriesID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := s.store.GetItem(ctx, seriesID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	seasons, total, err := s.store.ListItems(ctx, store.ItemFilter{
		ParentID: seriesID, Kinds: []string{store.KindSeason}, Page: 1, PerPage: 200,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	sort.SliceStable(seasons, func(i, j int) bool {
		return seasons[i].SeasonNumber < seasons[j].SeasonNumber
	})
	start := jfInt(r.URL.Query(), "StartIndex", 0)
	limit := jfInt(r.URL.Query(), "Limit", 200)
	if start < 0 {
		start = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if start >= len(seasons) {
		seasons = nil
	} else {
		seasons = seasons[start:]
	}
	if len(seasons) > limit {
		seasons = seasons[:limit]
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), seasons, false), TotalRecordCount: total, StartIndex: start,
	})
}

// handleJFNextUpAll serves the home-screen /Shows/NextUp (optionally per series).
func (s *Server) handleJFNextUpAll(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := jfInt(q, "Limit", 10)
	if limit <= 0 || limit > 200 {
		limit = 10
	}

	var shows []store.Item
	if raw := firstValue(q, "SeriesId"); raw != "" {
		seriesID, err := jellyfin.DecodeKind(raw, jellyfin.KindItem)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		show, err := s.store.GetItem(ctx, seriesID)
		if err != nil || show.Kind != store.KindShow {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		shows = append(shows, *show)
	} else {
		var err error
		shows, _, err = s.store.ListItems(ctx, store.ItemFilter{
			Kinds: []string{store.KindShow}, Page: 1, PerPage: 200, Category: "recent_added",
		})
		if err != nil {
			s.fail(w, err)
			return
		}
	}

	next := []store.Item{}
	for i := range shows {
		episodes, _, err := s.jfSeriesEpisodes(ctx, shows[i].ID)
		if err != nil || len(episodes) == 0 {
			continue
		}
		last := -1
		for j := range episodes {
			if episodes[j].Played || episodes[j].PositionMS > 0 {
				last = j
			}
		}
		switch {
		case last >= 0 && episodes[last].PositionMS > 0 && !episodes[last].Played:
			// Partially watched episode wins (the TV app sends
			// enableResumable=false on the home row but still expects this).
			next = append(next, episodes[last])
		case last >= 0:
			if n, err := s.store.NextEpisode(ctx, episodes[last].ID); err == nil && n != nil {
				next = append(next, *n)
			}
		case firstValue(q, "SeriesId") != "":
			// Series detail page wants a playable suggestion even before starting.
			next = append(next, episodes[0])
		}
	}
	if len(next) > limit {
		next = next[:limit]
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{
		Items: s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), next, false), TotalRecordCount: len(next),
	})
}

func (s *Server) handleJFSimilar(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, jellyfin.QueryResult{Items: []jellyfin.BaseItemDto{}})
}

func (s *Server) handleJFThemeMedia(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.jfBrowseContext(w, r); !ok {
		return
	}
	empty := func() map[string]any {
		return map[string]any{"Items": []any{}, "TotalRecordCount": 0, "OwnerId": r.PathValue("itemId")}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ThemeVideosResult":     empty(),
		"ThemeSongsResult":      empty(),
		"SoundtrackSongsResult": empty(),
	})
}

func (s *Server) jfExtraItems(w http.ResponseWriter, r *http.Request, trailersOnly bool) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := s.store.GetItem(ctx, itemID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	extras, err := s.store.ListExtras(ctx, itemID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if trailersOnly {
		filtered := extras[:0]
		for _, e := range extras {
			if strings.EqualFold(e.Title, "trailer") {
				filtered = append(filtered, e)
			}
		}
		extras = filtered
	}
	writeJSON(w, http.StatusOK, s.jfItemDTOs(ctx, s.jellyfinServerID(ctx), extras, false))
}

func (s *Server) handleJFLocalTrailers(w http.ResponseWriter, r *http.Request) {
	s.jfExtraItems(w, r, true)
}

func (s *Server) handleJFSpecialFeatures(w http.ResponseWriter, r *http.Request) {
	s.jfExtraItems(w, r, false)
}

func (s *Server) handleJFImageList(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.jfBrowseContext(w, r)
	if !ok {
		return
	}
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	out := []jellyfin.ImageInfo{}
	if it.PosterPath != "" {
		out = append(out, jellyfin.ImageInfo{ImageType: "Primary", ImageIndex: 0, Width: 1000, Height: 1500})
	}
	if it.BackdropPath != "" {
		out = append(out, jellyfin.ImageInfo{ImageType: "Backdrop", ImageIndex: 0, Width: 1920, Height: 1080})
	}
	writeJSON(w, http.StatusOK, out)
}
