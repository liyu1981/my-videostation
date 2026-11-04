package httpapi

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/jellyfin"
	"github.com/liyu1981/myvideostation/internal/playback"
	"github.com/liyu1981/myvideostation/internal/store"
)

// ---- parameter helpers -----------------------------------------------------

func jfCSV(q map[string][]string, key string) []string {
	raw := q[key]
	if len(raw) == 0 {
		return nil
	}
	out := []string{}
	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func jfBool(q map[string][]string, key string) bool {
	v := strings.TrimSpace(firstValue(q, key))
	return v == "true" || v == "1"
}

func jfInt(q map[string][]string, key string, def int) int {
	v := strings.TrimSpace(firstValue(q, key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func firstValue(q map[string][]string, key string) string {
	if v, ok := q[key]; ok && len(v) > 0 {
		return v[0]
	}
	for k, v := range q {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func jfTicks(ms int64) int64 { return ms * 10000 }

// ---- browse ----------------------------------------------------------------

func (s *Server) handleJFItems(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	res, err := s.jfQueryItems(ctx, r.URL.Query())
	if err != nil {
		if store.IsInvalid(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err == store.ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) jfQueryItems(ctx context.Context, q map[string][]string) (jellyfin.QueryResult, error) {
	serverID := s.jellyfinServerID(ctx)
	start := jfInt(q, "StartIndex", 0)
	if start < 0 {
		start = 0
	}
	limit := jfInt(q, "Limit", 60)
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	types := jfCSV(q, "IncludeItemTypes")
	excludes := jfCSV(q, "ExcludeItemTypes")
	recursive := jfBool(q, "Recursive")
	sortBy := jfCSV(q, "SortBy")
	descending := strings.EqualFold(firstValue(q, "SortOrder"), "Descending")

	parentRaw := firstValue(q, "ParentId")
	var libID, parentID int64
	var parentKind string
	if parentRaw != "" && !strings.EqualFold(parentRaw, "root") {
		k, id, err := jellyfin.Decode(parentRaw)
		if err != nil {
			return jellyfin.QueryResult{}, store.NewInvalid("invalid parent id")
		}
		switch k {
		case jellyfin.KindLibrary:
			libID = id
		case jellyfin.KindItem:
			parentID = id
		default:
			return jellyfin.QueryResult{}, store.NewInvalid("invalid parent id")
		}
	}
	libType := "movie"
	if libID > 0 {
		lib, err := s.store.GetLibrary(ctx, libID)
		if err != nil {
			return jellyfin.QueryResult{}, err
		}
		libType = lib.Type
	}
	if parentID > 0 {
		parent, err := s.store.GetItem(ctx, parentID)
		if err != nil {
			return jellyfin.QueryResult{}, err
		}
		libType = "" // parent kind drives the result
		parentKind = parent.Kind
	}

	kinds, empty := jfTargetKinds(types, excludes, libType, parentKind, recursive)
	if empty {
		return jellyfin.QueryResult{Items: []jellyfin.BaseItemDto{}, TotalRecordCount: 0, StartIndex: start}, nil
	}

	filter := store.ItemFilter{
		LibraryID:    libID,
		Kinds:        kinds,
		ExcludeKinds: jfExcludeKinds(excludes, kinds),
		Query:        firstValue(q, "SearchTerm"),
		Sort:         jfSortKey(sortBy),
		PerPage:      limit,
		Page:         start/limit + 1,
		Genres:       jfCSV(q, "Genres"),
		Ratings:      jfCSV(q, "OfficialRatings"),
	}
	if parentID > 0 && parentKind != store.KindShow {
		filter.ParentID = parentID
	}
	if y := jfCSV(q, "Years"); len(y) > 0 {
		for _, raw := range y {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				filter.Years = append(filter.Years, n)
			}
		}
	}
	for _, raw := range jfCSV(q, "GenreIds") {
		if id, err := jellyfin.DecodeKind(raw, jellyfin.KindGenre); err == nil {
			filter.GenreIDs = append(filter.GenreIDs, id)
		}
	}
	for _, raw := range jfCSV(q, "PersonIds") {
		if id, err := jellyfin.DecodeKind(raw, jellyfin.KindPerson); err == nil {
			filter.PersonIDs = append(filter.PersonIDs, id)
		}
	}
	for _, f := range jfCSV(q, "Filters") {
		switch strings.ToLower(f) {
		case "isplayed":
			watched := true
			filter.Watched = &watched
		case "isunplayed":
			watched := false
			filter.Watched = &watched
		case "isfavorite":
			filter.Favorites = true
		case "isresumable":
			filter.Resumable = true
		}
	}
	if raw := firstValue(q, "IsFolder"); raw != "" {
		v := strings.EqualFold(raw, "true") || raw == "1"
		filter.Folders = &v
	}
	if !descending && filter.Sort == "" {
		// Jellyfin defaults to ascending name order; our store defaults to
		// recently added, which is what clients expect for "Latest".
	}

	var items []store.Item
	var total int
	var err error
	if parentID > 0 && parentKind == store.KindShow && recursive && jfWantsEpisodes(types) {
		items, total, err = s.jfSeriesEpisodes(ctx, parentID)
	} else {
		items, total, err = s.store.ListItems(ctx, filter)
	}
	if err != nil {
		return jellyfin.QueryResult{}, err
	}

	// StartIndex may not be a multiple of Limit; adjust the page slice.
	if offset := start % limit; offset > 0 {
		if offset < len(items) {
			items = items[offset:]
		} else {
			items = nil
		}
	}
	if filter.Kind == store.KindEpisode || filter.Kind == store.KindSeason || containsKind(kinds, store.KindEpisode) || containsKind(kinds, store.KindSeason) {
		jfSortEpisodes(items, descending)
	} else if descending {
		if filter.Sort != "" {
			reverseItems(items)
		}
	}
	return jellyfin.QueryResult{Items: s.jfItemDTOs(ctx, serverID, items, false), TotalRecordCount: total, StartIndex: start}, nil
}

// jfItemDTOs maps a page of items, batching episode ancestors, child counts and
// favorite flags so list rows are complete without N+1 queries.
func (s *Server) jfItemDTOs(ctx context.Context, serverID string, items []store.Item, detail bool) []jellyfin.BaseItemDto {
	episodeIDs := []int64{}
	childIDs := []int64{}
	allIDs := make([]int64, 0, len(items))
	for i := range items {
		allIDs = append(allIDs, items[i].ID)
		switch items[i].Kind {
		case store.KindEpisode:
			episodeIDs = append(episodeIDs, items[i].ID)
		case store.KindShow, store.KindSeason:
			childIDs = append(childIDs, items[i].ID)
		}
	}
	seriesRefs, err := s.store.SeriesRefsForEpisodes(ctx, episodeIDs)
	if err != nil {
		s.log.Debug("series refs failed", "err", err)
	}
	counts, err := s.store.ChildCounts(ctx, childIDs)
	if err != nil {
		s.log.Debug("child counts failed", "err", err)
	}
	favorites, err := s.store.FavoriteItemIDs(ctx, allIDs)
	if err != nil {
		s.log.Debug("favorite ids failed", "err", err)
	}
	out := make([]jellyfin.BaseItemDto, 0, len(items))
	for i := range items {
		opts := jfItemOpts{
			detail:     detail,
			childCount: counts[items[i].ID],
			favorite:   favorites[items[i].ID],
		}
		if ref, ok := seriesRefs[items[i].ID]; ok {
			opts.series = &ref
		}
		out = append(out, s.jfBaseItemCtx(ctx, &items[i], serverID, opts))
	}
	return out
}

// jfExcludeKinds maps Jellyfin ExcludeItemTypes to store kinds, but only when
// the include set is a mixed "all" query (explicit Kinds are filtered directly).
func jfExcludeKinds(excludes, kinds []string) []string {
	if len(kinds) > 0 || len(excludes) == 0 {
		return nil
	}
	out := []string{}
	for _, t := range excludes {
		if k := jfKindForType(t); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func jfKindForType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "movie":
		return store.KindMovie
	case "series":
		return store.KindShow
	case "season":
		return store.KindSeason
	case "episode":
		return store.KindEpisode
	case "video", "musicvideo":
		return store.KindHomeVideo
	}
	return ""
}

// jfTargetKinds maps Jellyfin Include/ExcludeItemTypes and parent/library
// context to our item kinds. empty=true means the parent cannot have children.
func jfTargetKinds(types, excludes []string, libType, parentKind string, recursive bool) (kinds []string, empty bool) {
	excluded := map[string]bool{}
	for _, t := range excludes {
		if k := jfKindForType(t); k != "" {
			excluded[k] = true
		}
	}
	keep := func(k string) (string, bool) {
		if excluded[k] {
			return "", false
		}
		return k, true
	}

	switch parentKind {
	case store.KindShow:
		if recursive && jfWantsEpisodes(types) {
			k, ok := keep(store.KindEpisode)
			if !ok {
				return nil, true
			}
			return []string{k}, false
		}
		k, ok := keep(store.KindSeason)
		if !ok {
			return nil, true
		}
		return []string{k}, false
	case store.KindSeason:
		k, ok := keep(store.KindEpisode)
		if !ok {
			return nil, true
		}
		return []string{k}, false
	case "":
		// library or global query
	default:
		return nil, true
	}

	if len(types) == 0 {
		var defaults []string
		switch libType {
		case "tv":
			defaults = []string{store.KindShow}
		case "home_video", "tv_recording":
			defaults = []string{store.KindHomeVideo}
		case "movie":
			defaults = []string{store.KindMovie}
		default:
			return nil, false // mixed top-level default
		}
		out := defaults[:0]
		for _, k := range defaults {
			if !excluded[k] {
				out = append(out, k)
			}
		}
		if len(out) == 0 {
			return nil, true
		}
		return out, false
	}

	seen := map[string]bool{}
	out := []string{}
	for _, t := range types {
		k := jfKindForType(t)
		if k == "" || excluded[k] || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, true
	}
	// An episode request with Recursive means "all episodes of this library".
	if containsKind(out, store.KindEpisode) && !containsKind(out, store.KindMovie) && !containsKind(out, store.KindShow) {
		return []string{store.KindEpisode}, false
	}
	return out, false
}

func containsKind(kinds []string, kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func jfWantsEpisodes(types []string) bool {
	for _, t := range types {
		if strings.EqualFold(t, "Episode") {
			return true
		}
	}
	return len(types) == 0
}

func (s *Server) jfSeriesEpisodes(ctx context.Context, seriesID int64) ([]store.Item, int, error) {
	seasons, _, err := s.store.ListItems(ctx, store.ItemFilter{
		ParentID: seriesID, Kind: store.KindSeason, Page: 1, PerPage: 200, Sort: "title",
	})
	if err != nil {
		return nil, 0, err
	}
	all := []store.Item{}
	for _, season := range seasons {
		eps, _, err := s.store.ListItems(ctx, store.ItemFilter{
			ParentID: season.ID, Kind: store.KindEpisode, Page: 1, PerPage: 200,
		})
		if err != nil {
			return nil, 0, err
		}
		all = append(all, eps...)
	}
	jfSortEpisodes(all, false)
	return all, len(all), nil
}

func jfSortEpisodes(items []store.Item, descending bool) {
	sort.SliceStable(items, func(i, j int) bool {
		si, sj := items[i].SeasonNumber, items[j].SeasonNumber
		if si != sj {
			if descending {
				return si > sj
			}
			return si < sj
		}
		if descending {
			return items[i].EpisodeNumber > items[j].EpisodeNumber
		}
		return items[i].EpisodeNumber < items[j].EpisodeNumber
	})
}

func reverseItems(items []store.Item) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func jfSortKey(sortBy []string) string {
	for _, s := range sortBy {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "sortname":
			return "title"
		case "productionyear":
			return "year"
		case "communityrating", "criticrating":
			return "rating"
		case "runtime":
			return "runtime"
		case "datecreated", "datelastcontentadded", "premieredate":
			return "added"
		}
	}
	return ""
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// ---- item detail -----------------------------------------------------------

func (s *Server) handleJFItem(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	serverID := s.jellyfinServerID(ctx)
	k, id, err := jellyfin.Decode(r.PathValue("itemId"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if k == jellyfin.KindLibrary {
		lib, err := s.store.GetLibrary(ctx, id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, jellyfin.BaseItemDto{
			Name:           lib.Name,
			ServerID:       serverID,
			ID:             jellyfin.LibraryID(lib.ID),
			Type:           "CollectionFolder",
			CollectionType: jfCollectionType(lib.Type),
			IsFolder:       true,
			ChildCount:     int(lib.ItemCount),
			PlayAccess:     "Full",
		})
		return
	}
	if k != jellyfin.KindItem {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	it, err := s.store.GetItem(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	dto := s.jfBaseItem(ctx, it, serverID, true)
	writeJSON(w, http.StatusOK, dto)
}

// jfItemOpts carries batched context for list rows.
type jfItemOpts struct {
	detail     bool
	series     *store.SeriesRef
	childCount int
	favorite   bool
}

// jfBaseItem maps one of our items to the Jellyfin item shape.
func (s *Server) jfBaseItem(ctx context.Context, it *store.Item, serverID string, detail bool) jellyfin.BaseItemDto {
	return s.jfBaseItemCtx(ctx, it, serverID, jfItemOpts{detail: detail})
}

func (s *Server) jfBaseItemCtx(ctx context.Context, it *store.Item, serverID string, opts jfItemOpts) jellyfin.BaseItemDto {
	d := jellyfin.BaseItemDto{
		Name:              it.Title,
		ServerID:          serverID,
		ID:                jellyfin.ItemID(it.ID),
		SortName:          it.SortTitle,
		IsFolder:          it.Kind == store.KindShow || it.Kind == store.KindSeason,
		ProductionYear:    it.Year,
		Overview:          it.Overview,
		OfficialRating:    it.ContentRating,
		CommunityRating:   it.Rating,
		RunTimeTicks:      jfTicks(it.RuntimeMS),
		DateCreated:       it.AddedAt.UTC().Format("2006-01-02T15:04:05.0000000Z"),
		CanDownload:       false,
		CanDelete:         false,
		PlayAccess:        "Full",
		LocationType:      "FileSystem",
		BackdropImageTags: []string{},
	}
	if it.ReleaseDate != "" {
		d.PremiereDate = it.ReleaseDate
	}
	switch it.Kind {
	case store.KindMovie:
		d.Type = "Movie"
		d.MediaType = "Video"
		d.ParentID = jellyfin.LibraryID(it.LibraryID)
		d.PrimaryImageAspectRatio = 0.6667
	case store.KindShow:
		d.Type = "Series"
		d.ParentID = jellyfin.LibraryID(it.LibraryID)
		d.PrimaryImageAspectRatio = 0.6667
	case store.KindSeason:
		d.Type = "Season"
		d.MediaType = "Video"
		d.ParentID = jellyfin.ItemID(it.ParentID)
		d.SeriesID = jellyfin.ItemID(it.ParentID)
		d.PrimaryImageAspectRatio = 0.6667
		num := it.SeasonNumber
		d.IndexNumber = &num
	case store.KindEpisode:
		d.Type = "Episode"
		d.MediaType = "Video"
		d.ParentID = jellyfin.ItemID(it.ParentID)
		num := it.EpisodeNumber
		d.IndexNumber = &num
		if it.SeasonNumber > 0 {
			sn := it.SeasonNumber
			d.ParentIndexNumber = &sn
		}
		d.PrimaryImageAspectRatio = 1.7778
		if opts.series != nil {
			d.SeriesID = jellyfin.ItemID(opts.series.SeriesID)
			d.SeasonID = jellyfin.ItemID(opts.series.SeasonID)
			d.SeriesName = opts.series.SeriesName
			d.SeasonName = opts.series.SeasonName
			if tag := jfImageTag(opts.series.PosterPath); tag != "" {
				d.SeriesPrimaryImageTag = tag
			}
			if tag := jfImageTag(opts.series.BackdropPath); tag != "" {
				d.ParentBackdropItemID = d.SeriesID
				d.ParentBackdropImageTags = []string{tag}
			}
		} else if opts.detail {
			if season, err := s.store.GetItem(ctx, it.ParentID); err == nil {
				d.SeasonID = jellyfin.ItemID(season.ID)
				d.SeasonName = season.Title
				d.SeriesID = jellyfin.ItemID(season.ParentID)
			}
		}
	case store.KindExtra:
		d.Type = "Video"
		d.MediaType = "Video"
		d.ParentID = jellyfin.ItemID(it.ParentID)
		d.PrimaryImageAspectRatio = 1.7778
	default: // home video
		d.Type = "Video"
		d.MediaType = "Video"
		d.ParentID = jellyfin.LibraryID(it.LibraryID)
		d.PrimaryImageAspectRatio = 1.7778
	}

	// Watch state.
	playCount := 0
	if it.Played {
		playCount = 1
	}
	d.UserData = &jellyfin.UserItemDataDto{
		Played:                it.Played,
		PlaybackPositionTicks: jfTicks(it.PositionMS),
		PlayCount:             playCount,
		IsFavorite:            opts.favorite,
	}
	if it.DurationMS > 0 && it.PositionMS > 0 {
		pct := float64(it.PositionMS) / float64(it.DurationMS) * 100
		if pct > 100 {
			pct = 100
		}
		d.UserData.PlayedPercentage = pct
	}

	// Artwork tags.
	if tag := jfImageTag(it.PosterPath); tag != "" {
		d.ImageTags = map[string]string{"Primary": tag}
	}
	if tag := jfImageTag(it.BackdropPath); tag != "" {
		d.BackdropImageTags = []string{tag}
	}

	if it.Provider == "tmdb" && it.ProviderID != "" {
		d.ProviderIds = map[string]string{"Tmdb": it.ProviderID}
	}

	if opts.childCount > 0 {
		d.ChildCount = opts.childCount
	}
	if opts.detail {
		if genres, err := s.store.ItemGenres(ctx, it.ID); err == nil && len(genres) > 0 {
			d.Genres = genres
		}
		if people, err := s.store.ItemPeople(ctx, it.ID); err == nil {
			for _, p := range people {
				d.People = append(d.People, jellyfin.PersonDto{
					Name: p.Name, Role: p.Character, Type: jfPersonType(p.Role),
				})
			}
		}
		d.ChildCount, d.RecursiveItemCount = s.jfChildCounts(ctx, it)
	}
	if playableKind(it.Kind) {
		d.MediaSources = s.jfMediaSources(ctx, it)
		d.VideoType = "VideoFile"
		d.Path = ""
		if len(d.MediaSources) > 0 {
			d.Path = d.MediaSources[0].Path
		}
	}
	return d
}

func playableKind(kind string) bool {
	switch kind {
	case store.KindMovie, store.KindEpisode, store.KindHomeVideo, store.KindExtra:
		return true
	}
	return false
}

func jfPersonType(role string) string {
	switch role {
	case "director":
		return "Director"
	case "writer":
		return "Writer"
	case "producer":
		return "Producer"
	}
	return "Actor"
}

// jfChildCounts returns (direct children, recursive children) for shows/seasons.
func (s *Server) jfChildCounts(ctx context.Context, it *store.Item) (int, int) {
	switch it.Kind {
	case store.KindShow:
		seasons, _, err := s.store.ListItems(ctx, store.ItemFilter{ParentID: it.ID, Kind: store.KindSeason, Page: 1, PerPage: 200, Sort: "title"})
		if err != nil {
			return 0, 0
		}
		episodes := 0
		for _, season := range seasons {
			if _, total, err := s.store.ListItems(ctx, store.ItemFilter{ParentID: season.ID, Kind: store.KindEpisode, Page: 1, PerPage: 1}); err == nil {
				episodes += total
			}
		}
		return len(seasons), episodes
	case store.KindSeason:
		_, total, err := s.store.ListItems(ctx, store.ItemFilter{ParentID: it.ID, Kind: store.KindEpisode, Page: 1, PerPage: 1})
		if err != nil {
			return 0, 0
		}
		return total, total
	}
	return 0, 0
}

// jfMediaSources builds MediaSources/MediaStreams from the item's files.
func (s *Server) jfMediaSources(ctx context.Context, it *store.Item) []jellyfin.MediaSourceInfo {
	files, err := s.store.ItemFiles(ctx, it.ID)
	if err != nil {
		return nil
	}
	out := []jellyfin.MediaSourceInfo{}
	for i := range files {
		f := &files[i]
		if f.Missing {
			continue
		}
		container := playback.ContainerFromPath(f.Path, f.Container)
		audioCodes := make([]string, 0, len(f.AudioStreams))
		for _, a := range f.AudioStreams {
			audioCodes = append(audioCodes, a.Codec)
		}
		direct := playback.EvaluateDirectPlay(container, f.VideoCodec, audioCodes)
		src := jellyfin.MediaSourceInfo{
			ID:                   jellyfin.FileID(f.ID),
			Name:                 it.Title,
			Path:                 f.Path,
			Container:            container,
			Size:                 f.Size,
			RunTimeTicks:         jfTicks(f.DurationMS),
			Bitrate:              f.Bitrate,
			SupportsDirectPlay:   direct.OK,
			SupportsDirectStream: true,
			SupportsTranscoding:  s.transcoder.Available(),
			Protocol:             "File",
			VideoType:            "VideoFile",
			ETag:                 jfImageTag(f.Path),
			MediaStreams:         []jellyfin.MediaStream{},
		}
		if f.DurationMS > 0 {
			src.MediaStreams = append(src.MediaStreams, jellyfin.MediaStream{
				Type: "Video", Index: 0, Codec: f.VideoCodec,
				Width: f.Width, Height: f.Height, BitRate: f.Bitrate,
				IsDefault: true, DisplayTitle: f.VideoCodec,
			})
		}
		for _, a := range f.AudioStreams {
			idx := a.Index
			if a.Default && src.DefaultAudioStreamIndex == nil {
				src.DefaultAudioStreamIndex = &idx
			}
			src.MediaStreams = append(src.MediaStreams, jellyfin.MediaStream{
				Type: "Audio", Index: a.Index, Codec: a.Codec, Language: a.Language,
				Title: a.Title, DisplayTitle: a.Codec, Channels: a.Channels,
				ChannelLayout: a.Layout, SampleRate: a.SampleRate, BitRate: a.Bitrate,
				IsDefault: a.Default,
			})
		}
		if src.DefaultAudioStreamIndex == nil && len(f.AudioStreams) > 0 {
			idx := f.AudioStreams[0].Index
			src.DefaultAudioStreamIndex = &idx
		}
		for _, st := range f.SubtitleStreams {
			text := !imageBasedSubtitle(st.Codec)
			stream := jellyfin.MediaStream{
				Type: "Subtitle", Index: st.Index, Codec: st.Codec, Language: st.Language,
				Title: st.Title, DisplayTitle: st.Title, IsForced: st.Forced,
				IsTextSubtitleStream: text,
			}
			if text {
				stream.DeliveryMethod = "External"
				stream.DeliveryURL = fmt.Sprintf("/api/v1/media/%d/subtitle?embedded=%d", f.ID, st.Index)
			} else {
				// Bitmap subtitles can only be rendered by burning them in.
				stream.DeliveryMethod = "Encode"
			}
			src.MediaStreams = append(src.MediaStreams, stream)
		}
		if exts, err := s.store.ExternalSubtitles(ctx, f.ID); err == nil {
			for _, ext := range exts {
				src.MediaStreams = append(src.MediaStreams, jellyfin.MediaStream{
					Type: "Subtitle", Index: 100000 + int(ext.ID), Codec: ext.Format,
					Language: ext.Language, Title: ext.Label, DisplayTitle: ext.Label,
					IsExternal: true, IsTextSubtitleStream: true, DeliveryMethod: "External",
					DeliveryURL: fmt.Sprintf("/api/v1/media/%d/subtitle?ext=%d", f.ID, ext.ID),
				})
			}
		}
		out = append(out, src)
	}
	return out
}

// jfImageTag derives a stable cache tag from an artwork file's mtime/size.
func jfImageTag(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", path, st.ModTime().UnixNano(), st.Size())))
	return hex.EncodeToString(sum[:])[:32]
}

// ---- images ----------------------------------------------------------------

func (s *Server) handleJFImage(w http.ResponseWriter, r *http.Request) {
	itemID, err := jellyfin.DecodeKind(r.PathValue("itemId"), jellyfin.KindItem)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	kind := ""
	switch strings.ToLower(r.PathValue("imageType")) {
	case "primary":
		kind = "poster"
	case "backdrop", "thumb":
		kind = "backdrop"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s.serveItemArt(w, r, itemID, kind)
}

// ---- search ----------------------------------------------------------------

func (s *Server) handleJFSearchHints(w http.ResponseWriter, r *http.Request) {
	me, ok := s.jfAuthenticatedUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := s.userContext(r.Context(), me, "")
	term := firstValue(r.URL.Query(), "searchTerm")
	limit := jfInt(r.URL.Query(), "limit", 20)
	items, err := s.store.Search(ctx, term, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	types := jfCSV(r.URL.Query(), "includeItemTypes")
	hints := make([]jellyfin.SearchHint, 0, len(items))
	for i := range items {
		it := &items[i]
		if len(types) > 0 && !jfTypeMatches(types, it.Kind) {
			continue
		}
		h := jellyfin.SearchHint{
			ItemID:          jellyfin.ItemID(it.ID),
			ID:              jellyfin.ItemID(it.ID),
			Name:            it.Title,
			MatchedTerm:     term,
			ProductionYear:  it.Year,
			Type:            jfItemType(it.Kind),
			MediaType:       "Video",
			RunTimeTicks:    jfTicks(it.RuntimeMS),
			ParentID:        jellyfin.LibraryID(it.LibraryID),
			PrimaryImageTag: jfImageTag(it.PosterPath),
		}
		if it.Kind == store.KindEpisode {
			h.IndexNumber = &it.EpisodeNumber
			h.ParentID = jellyfin.ItemID(it.ParentID)
		}
		hints = append(hints, h)
	}
	writeJSON(w, http.StatusOK, jellyfin.SearchHintResult{SearchHints: hints, TotalRecordCount: len(hints)})
}

func jfItemType(kind string) string {
	switch kind {
	case store.KindMovie:
		return "Movie"
	case store.KindShow:
		return "Series"
	case store.KindSeason:
		return "Season"
	case store.KindEpisode:
		return "Episode"
	case store.KindExtra:
		return "Video"
	}
	return "Video"
}

func jfTypeMatches(types []string, kind string) bool {
	want := jfItemType(kind)
	for _, t := range types {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}
