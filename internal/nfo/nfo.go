// Package nfo parses Jellyfin/Kodi style .nfo sidecar metadata (XML).
package nfo

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Meta is the sidecar metadata we import.
type Meta struct {
	Kind          string // movie | tvshow | episode
	Title         string
	OriginalTitle string
	SortTitle     string
	Overview      string
	Tagline       string
	Certification string
	Year          int
	Rating        float64
	RuntimeMS     int64
	Genres        []string
	Cast          []string
	Directors     []string
	Writers       []string
	PosterPath    string // absolute local path (URLs are ignored in M2.5)
	BackdropPath  string
	ProviderIDs   map[string]string
	Season        int
	Episode       int
}

// ErrInvalid is returned when the file is not a recognised NFO document.
var ErrInvalid = errors.New("not a recognised nfo file")

type xmlNFO struct {
	XMLName       xml.Name
	Title         string  `xml:"title"`
	OriginalTitle string  `xml:"originaltitle"`
	SortTitle     string  `xml:"sorttitle"`
	Year          int     `xml:"year"`
	Plot          string  `xml:"plot"`
	Outline       string  `xml:"outline"`
	Tagline       string  `xml:"tagline"`
	Runtime       int     `xml:"runtime"`
	MPAA          string  `xml:"mpaa"`
	Rating        float64 `xml:"rating"`
	Ratings       struct {
		Entries []struct {
			Name  string  `xml:"name,attr"`
			Value float64 `xml:"value"`
		} `xml:"rating"`
	} `xml:"ratings"`
	Genres []string `xml:"genre"`
	Actors []struct {
		Name  string `xml:"name"`
		Role  string `xml:"role"`
		Order int    `xml:"order"`
	} `xml:"actor"`
	Directors []string `xml:"director"`
	Credits   []string `xml:"credits"`
	Writers   []string `xml:"writer"`
	UniqueIDs []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
	TMDBID string `xml:"tmdbid"`
	IMDBID string `xml:"imdbid"`
	Thumbs []struct {
		Aspect string `xml:"aspect,attr"`
		Text   string `xml:",chardata"`
	} `xml:"thumb"`
	Fanart struct {
		Thumbs []struct {
			Text string `xml:",chardata"`
		} `xml:"thumb"`
	} `xml:"fanart"`
	Season  int `xml:"season"`
	Episode int `xml:"episode"`
}

// ParseFile reads and parses an NFO file.
func ParseFile(path string) (*Meta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	m.resolveArt(filepath.Dir(path))
	return m, nil
}

// Parse decodes NFO XML (movie, tvshow or episodedetails).
func Parse(raw []byte) (*Meta, error) {
	raw = trimBOM(raw)
	var doc xmlNFO
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("nfo: %w", err)
	}
	kind := strings.ToLower(doc.XMLName.Local)
	switch kind {
	case "movie", "tvshow", "episodedetails":
	case "episodedetail":
		kind = "episode"
	default:
		return nil, ErrInvalid
	}
	if kind == "episodedetails" {
		kind = "episode"
	}

	m := &Meta{
		Kind:          kind,
		Title:         strings.TrimSpace(doc.Title),
		OriginalTitle: strings.TrimSpace(doc.OriginalTitle),
		SortTitle:     strings.TrimSpace(doc.SortTitle),
		Overview:      firstNonEmpty(strings.TrimSpace(doc.Plot), strings.TrimSpace(doc.Outline)),
		Tagline:       strings.TrimSpace(doc.Tagline),
		Certification: strings.TrimSpace(doc.MPAA),
		Year:          doc.Year,
		Rating:        pickRating(doc),
		RuntimeMS:     int64(doc.Runtime) * 60_000,
		Genres:        cleanList(doc.Genres),
		Directors:     cleanList(doc.Directors),
		Writers:       cleanList(append(append([]string{}, doc.Credits...), doc.Writers...)),
		ProviderIDs:   map[string]string{},
		Season:        doc.Season,
		Episode:       doc.Episode,
	}
	for _, a := range doc.Actors {
		if name := strings.TrimSpace(a.Name); name != "" {
			m.Cast = append(m.Cast, name)
		}
	}
	for _, id := range doc.UniqueIDs {
		if v := strings.TrimSpace(id.Value); v != "" {
			m.ProviderIDs[strings.ToLower(strings.TrimSpace(id.Type))] = v
		}
	}
	if v := strings.TrimSpace(doc.TMDBID); v != "" {
		m.ProviderIDs["tmdb"] = v
	}
	if v := strings.TrimSpace(doc.IMDBID); v != "" {
		m.ProviderIDs["imdb"] = v
	}

	// Artwork: prefer an explicit poster thumb, then any thumb; backdrop from fanart.
	for _, t := range doc.Thumbs {
		p := strings.TrimSpace(t.Text)
		if p == "" {
			continue
		}
		if strings.EqualFold(t.Aspect, "poster") && m.PosterPath == "" {
			m.PosterPath = p
		}
	}
	if m.PosterPath == "" {
		for _, t := range doc.Thumbs {
			if p := strings.TrimSpace(t.Text); p != "" {
				m.PosterPath = p
				break
			}
		}
	}
	for _, t := range doc.Fanart.Thumbs {
		if p := strings.TrimSpace(t.Text); p != "" {
			m.BackdropPath = p
			break
		}
	}

	if strings.HasPrefix(m.PosterPath, "http://") || strings.HasPrefix(m.PosterPath, "https://") {
		m.PosterPath = ""
	}
	if strings.HasPrefix(m.BackdropPath, "http://") || strings.HasPrefix(m.BackdropPath, "https://") {
		m.BackdropPath = ""
	}
	if m.Title == "" && m.Overview == "" && len(m.Cast) == 0 {
		return nil, ErrInvalid
	}
	return m, nil
}

// resolveArt turns relative artwork references into absolute paths and drops URLs.
func (m *Meta) resolveArt(dir string) {
	m.PosterPath = resolveLocal(dir, m.PosterPath)
	m.BackdropPath = resolveLocal(dir, m.BackdropPath)
}

func resolveLocal(dir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return "" // remote artwork is not downloaded for NFO in this milestone
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	p = filepath.Clean(p)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return ""
	}
	return p
}

func pickRating(doc xmlNFO) float64 {
	preferred := map[string]bool{"themoviedb": true, "tmdb": true}
	for _, e := range doc.Ratings.Entries {
		if preferred[strings.ToLower(e.Name)] && e.Value > 0 {
			return e.Value
		}
	}
	for _, e := range doc.Ratings.Entries {
		if e.Value > 0 {
			return e.Value
		}
	}
	return doc.Rating
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToLower(s)] {
			continue
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
