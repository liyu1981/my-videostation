// Package sidecar discovers and merges local metadata files (.nfo and .vsmeta).
//
// Precedence: Synology .vsmeta wins field-by-field over Jellyfin/Kodi .nfo (the
// opposite of Kodi, matching Video Station). Embedded .vsmeta artwork wins over
// local NFO artwork; NFO values are used only when .vsmeta has no value.
package sidecar

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/liyu1981/myvideostation/internal/nfo"
	"github.com/liyu1981/myvideostation/internal/vsmeta"
)

// Meta is merged sidecar metadata ready to be applied to an item.
type Meta struct {
	Source        string
	Title         string
	OriginalTitle string
	SortTitle     string
	Tagline       string
	Overview      string
	Certification string
	Year          int
	ReleaseDate   string
	Rating        float64
	RuntimeMS     int64
	Genres        []string
	Cast          []string
	Directors     []string
	Writers       []string
	PosterPath    string
	BackdropPath  string
	ProviderIDs   map[string]string
}

// Result separates metadata for the file itself and for its parent show.
type Result struct {
	File *Meta
	Show *Meta
}

// Load looks for sidecar metadata next to a video file and in its parent folders.
// cacheDir receives embedded .vsmeta artwork.
func Load(videoPath, cacheDir string) *Result {
	res := &Result{}

	// ---- .nfo (highest priority) -----------------------------------------
	var fileNFO *nfo.Meta
	for _, candidate := range nfoCandidates(videoPath) {
		if m, err := nfo.ParseFile(candidate); err == nil {
			fileNFO = m
			break
		}
	}

	// ---- .vsmeta ---------------------------------------------------------
	var fileVS *vsmeta.Meta
	for _, candidate := range vsmetaCandidates(videoPath) {
		raw, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if m, err := vsmeta.Parse(raw); err == nil {
			fileVS = m
			break
		}
	}

	if fileNFO != nil || fileVS != nil {
		res.File = merge(fileNFO, fileVS, cacheDir)
	}

	// ---- tvshow.nfo for episodes ----------------------------------------
	if showNFO := findShowNFO(videoPath); showNFO != nil {
		res.Show = &Meta{
			Source:        "nfo",
			Title:         showNFO.Title,
			OriginalTitle: showNFO.OriginalTitle,
			SortTitle:     showNFO.SortTitle,
			Overview:      showNFO.Overview,
			Tagline:       showNFO.Tagline,
			Certification: showNFO.Certification,
			Year:          showNFO.Year,
			ReleaseDate:   showNFO.ReleaseDate,
			Rating:        showNFO.Rating,
			Genres:        showNFO.Genres,
			Cast:          showNFO.Cast,
			Directors:     showNFO.Directors,
			Writers:       showNFO.Writers,
			PosterPath:    showNFO.PosterPath,
			BackdropPath:  showNFO.BackdropPath,
			ProviderIDs:   showNFO.ProviderIDs,
		}
	}
	if res.File == nil && res.Show == nil {
		return nil
	}
	return res
}

func nfoCandidates(videoPath string) []string {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	out := []string{
		filepath.Join(dir, base+".nfo"),
		videoPath + ".nfo",
	}
	// Folder-level NFOs used by Kodi/Jellyfin for movies and shows.
	for _, folderName := range []string{"movie.nfo", "tvshow.nfo"} {
		p := filepath.Join(dir, folderName)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func vsmetaCandidates(videoPath string) []string {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	return []string{
		videoPath + ".vsmeta",              // Synology style: <file>.mkv.vsmeta
		filepath.Join(dir, base+".vsmeta"), // alternative
	}
}

func findShowNFO(videoPath string) *nfo.Meta {
	dir := filepath.Dir(videoPath)
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, "tvshow.nfo")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if m, err := nfo.ParseFile(p); err == nil {
				return m
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

func merge(n *nfo.Meta, v *vsmeta.Meta, cacheDir string) *Meta {
	m := &Meta{ProviderIDs: map[string]string{}}
	sources := []string{}
	if v != nil {
		sources = append(sources, "vsmeta")
	}
	if n != nil {
		sources = append(sources, "nfo")
	}

	// NFO is the base layer.
	if n != nil {
		m.Title = n.Title
		m.OriginalTitle = n.OriginalTitle
		m.SortTitle = n.SortTitle
		m.Overview = n.Overview
		m.Tagline = n.Tagline
		m.Certification = n.Certification
		m.Year = n.Year
		m.ReleaseDate = n.ReleaseDate
		m.Rating = n.Rating
		m.RuntimeMS = n.RuntimeMS
		m.Genres = n.Genres
		m.Cast = n.Cast
		m.Directors = n.Directors
		m.Writers = n.Writers
		for k, val := range n.ProviderIDs {
			m.ProviderIDs[k] = val
		}
		m.PosterPath = n.PosterPath
		m.BackdropPath = n.BackdropPath
	}

	// .vsmeta has priority over .nfo, field by field.
	if v != nil {
		m.Title = first(v.Title, m.Title)
		m.OriginalTitle = first(v.OriginalTitle, m.OriginalTitle)
		m.Overview = first(v.Overview, m.Overview)
		m.Tagline = first(v.Tagline, m.Tagline)
		m.Certification = first(v.Certification, m.Certification)
		if v.Year > 0 {
			m.Year = v.Year
		}
		if v.ReleaseDate != "" {
			m.ReleaseDate = v.ReleaseDate
		}
		if v.Rating > 0 {
			m.Rating = v.Rating
		}
		if len(v.Genres) > 0 {
			m.Genres = v.Genres
		}
		if len(v.Cast) > 0 {
			m.Cast = v.Cast
		}
		if len(v.Directors) > 0 {
			m.Directors = v.Directors
		}
		if len(v.Writers) > 0 {
			m.Writers = v.Writers
		}
		for k, val := range v.ProviderIDs {
			m.ProviderIDs[k] = val
		}
		if p := cacheEmbeddedPoster(v, cacheDir); p != "" {
			m.PosterPath = p
		}
		if b := cacheEmbeddedBackdrop(v, cacheDir); b != "" {
			m.BackdropPath = b
		}
	}

	m.Source = strings.Join(sources, "+")
	return m
}

func cacheEmbeddedPoster(v *vsmeta.Meta, cacheDir string) string {
	data, err := v.Poster()
	return cacheEmbedded(data, err, v.PosterHash, cacheDir)
}

func cacheEmbeddedBackdrop(v *vsmeta.Meta, cacheDir string) string {
	data, err := v.Backdrop()
	return cacheEmbedded(data, err, v.BackdropHash, cacheDir)
}

// cacheEmbedded writes an embedded image to the cache and returns its path.
func cacheEmbedded(data []byte, err error, hash, cacheDir string) string {
	if err != nil || len(data) == 0 || cacheDir == "" {
		return ""
	}
	ext := ".jpg"
	if len(data) > 1 && data[0] == 0x89 && data[1] == 'P' {
		ext = ".png"
	}
	name := hash
	if name == "" {
		sum := sha1.Sum(data)
		name = hex.EncodeToString(sum[:])
	}
	dir := filepath.Join(cacheDir, "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "vsmeta-"+name+ext)
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path
	}
	tmp, err := os.CreateTemp(dir, "vs-*")
	if err != nil {
		return ""
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return ""
	}
	if err := tmp.Close(); err != nil {
		return ""
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return ""
	}
	return path
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Describe is a helper for logs.
func (m *Meta) Describe() string {
	if m == nil {
		return "none"
	}
	return fmt.Sprintf("%s title=%q year=%d cast=%d", m.Source, m.Title, m.Year, len(m.Cast))
}
