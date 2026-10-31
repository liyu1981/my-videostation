// Package naming turns file and folder names into media identities.
//
// It is deliberately pure (no I/O) and heavily tested: this is the highest-leverage
// component of media management, and every real-world filename bug fixed here needs a
// regression case in parse_test.go.
package naming

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Kind is the media identity inferred from a filename.
type Kind string

const (
	KindMovie     Kind = "movie"
	KindEpisode   Kind = "episode"
	KindHomeVideo Kind = "home_video"
	KindExtra     Kind = "extra"
	KindUnknown   Kind = "unknown"
)

// Result is the parsed identity of one file.
type Result struct {
	Kind         Kind
	Title        string // show title for episodes, movie title otherwise
	EpisodeTitle string // optional, from "Show - S01E02 - Title"
	Year         int    // 0 when unknown
	Season       int    // 0 when unknown (episodes only)
	Episode      int    // 0 when unknown
	EpisodeEnd   int    // multi-episode files: S01E01-E02 => 2
	Edition      string // "Extended", "Director's Cut", …
	Extra        string // "sample", "trailer", … (Kind == KindExtra)
}

// IsVideoFile reports whether name has a video extension we index.
func IsVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".wmv", ".webm",
		".ts", ".m2ts", ".mpg", ".mpeg", ".flv", ".3gp", ".ogv", ".rmvb", ".rm", ".vob":
		return true
	}
	return false
}

var (
	reSeasonDir = regexp.MustCompile(`(?i)^(season|series|s)[\s._-]*\d{1,2}$|^specials?$`)
	reSxxExx    = regexp.MustCompile(`(?i)\bS(\d{1,2})[\s._-]*E(\d{1,3})(?:[\s._-]*-?E(\d{1,3}))?\b`)
	reNxNN      = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	reSeason    = regexp.MustCompile(`(?i)\bseason[\s._-]*(\d{1,2})[\s._-]*(?:episode|ep|e)[\s._-]*(\d{1,3})\b`)
	reEpOnly    = regexp.MustCompile(`(?i)\bE(?:P|pisode)?[\s._-]?(\d{1,3})\b`)
	reYear      = regexp.MustCompile(`\((19\d{2}|20\d{2})\)`)
	// trailing group tag: "2010-RARBG" => "2010"
	reYearGroup = regexp.MustCompile(`((?:19|20)\d{2})-[A-Za-z0-9]{2,}$`)
	reDigits    = regexp.MustCompile(`^\d{3,4}p$`)

	reBrackets = []*regexp.Regexp{
		regexp.MustCompile(`【[^】]*】`),
		regexp.MustCompile(`\[[^\]]*\]`),
		regexp.MustCompile(`\{[^}]*\}`),
	}
	reParens     = regexp.MustCompile(`\([^)]*\)`)
	reTrailGroup = regexp.MustCompile(`[\s._-]+[A-Za-z0-9]{2,}$`)
	reCJKTags    = regexp.MustCompile(`(?i)(?:国英双语|中英双字|中英字幕|国语中字|国语|粤语|中字|双语|高清|蓝光|完整版|无删减|默认字幕|BD|HD)+$`)
)

// release tags used to trim the tail of scene-style names
var releaseTags = map[string]bool{
	// resolution / quality
	"2160p": true, "1080p": true, "1080i": true, "720p": true, "576p": true, "480p": true,
	"4k": true, "uhd": true, "hd": true, "sd": true, "hq": true,
	// source
	"bluray": true, "blu-ray": true, "bdrip": true, "brrip": true, "bdremux": true,
	"webrip": true, "web-dl": true, "webdl": true, "web": true, "hdtv": true,
	"dvdrip": true, "dvd": true, "hdrip": true, "remux": true, "cam": true, "hdcam": true,
	"telesync": true, "ts": true, "tc": true, "ppv": true,
	// video codec
	"x264": true, "x265": true, "h264": true, "h265": true, "hevc": true, "avc": true,
	"xvid": true, "divx": true, "av1": true, "vp9": true, "mpeg2": true, "mpeg4": true,
	// audio
	"aac": true, "aac2": true, "ac3": true, "eac3": true, "dts": true, "dts-hd": true,
	"truehd": true, "flac": true, "mp3": true, "opus": true, "ddp": true, "dd": true,
	"atmos": true, "5.1": true, "7.1": true, "2.0": true, "1.0": true, "6ch": true,
	// colour / hdr
	"10bit": true, "8bit": true, "hdr": true, "hdr10": true, "dv": true, "dolbyvision": true, "sdr": true,
	// misc
	"proper": true, "repack": true, "limited": true, "internal": true, "subs": true,
	"sub": true, "dubbed": true, "multi": true, "dual": true, "retail": true,
	"imax": true, "cc": true, "uncut": true, "criterion": true, "amzn": true, "nf": true,
	"dsnp": true, "hmax": true, "atvp": true,
	// languages
	"eng": true, "english": true, "chi": true, "chs": true, "cht": true, "zh": true,
	"cn": true, "jpn": true, "kor": true, "fre": true, "fra": true, "ger": true, "deu": true,
	"spa": true, "ita": true, "rus": true, "多语": true,
}

// editions are preserved but removed from the title
var editionWords = map[string]string{
	"extended":    "Extended",
	"unrated":     "Unrated",
	"uncut":       "Uncut",
	"remastered":  "Remastered",
	"directors":   "Director's Cut",
	"director's":  "Director's Cut",
	"theatrical":  "Theatrical",
	"special":     "Special Edition",
	"imax":        "IMAX",
	"criterion":   "Criterion",
	"anniversary": "Anniversary",
	"restored":    "Restored",
}

// ordered: more specific markers win over the generic "extras" directory.
var extraMarkers = []struct {
	word, kind  string
	segmentOnly bool
}{
	{"behindthescenes", "behind_the_scenes", false},
	{"deletedscene", "deleted_scenes", false},
	{"deleted", "deleted_scenes", false},
	{"featurette", "featurette", false},
	{"interview", "interview", false},
	{"blooper", "bloopers", false},
	{"gagreel", "bloopers", false},
	{"trailer", "trailer", false},
	{"teaser", "trailer", false},
	{"sample", "sample", false},
	{"extras", "extra", true},
	{"bonus", "extra", true},
}

var leadingArticles = []string{"the ", "a ", "an "}

// Parse classifies one file path for a given library type
// (movie, tv, home_video, tv_recording).
func Parse(libraryType, path string) Result {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	dir := filepath.Dir(path)

	clean := normalize(stem)

	// Extras are detected on the whole path so that "Extras/foo.mkv" works.
	if kind := detectExtra(dir, clean); kind != "" {
		return Result{Kind: KindExtra, Extra: kind, Title: cleanTitle(stripReleaseTags(clean))}
	}

	switch libraryType {
	case "home_video", "tv_recording":
		title := stripReleaseTags(clean)
		title, year := extractYear(title)
		t := cleanTitle(title)
		if t == "" || isGenericTitle(t) {
			t = cleanTitle(stripReleaseTags(showFolder(path)))
		}
		if year == 0 {
			if _, y := extractYear(filepath.Base(filepath.Dir(path))); y > 0 {
				year = y
			}
		}
		return Result{Kind: KindHomeVideo, Title: fallbackTitle(t, stem), Year: year}
	case "tv":
		if r, ok := parseEpisode(clean); ok {
			finishEpisode(&r, showFolder(path))
			return r
		}
		// A file in a TV library without SxxExx cannot be identified reliably.
		return Result{Kind: KindUnknown, Title: cleanTitle(stripReleaseTags(clean))}
	default: // movie
		clean = reYearGroup.ReplaceAllString(clean, "$1")
		title := stripReleaseTags(clean)
		title, year := extractYear(title)
		title = stripReleaseTags(title)
		title, edition := stripEdition(title)
		title = cleanTitle(title)
		if title == "" || isGenericTitle(title) {
			folder := showFolder(path)
			if ft, fy := extractYear(reYearGroup.ReplaceAllString(folder, "$1")); folder != "" {
				title = cleanTitle(stripReleaseTags(ft))
				if year == 0 {
					year = fy
				}
			}
		}
		return Result{Kind: KindMovie, Title: fallbackTitle(title, stem), Year: year, Edition: edition}
	}
}

// normalize turns scene-style separators into spaces while preserving dotted
// acronyms and titles that already use spaces ("Dr. Strangelove").
func normalize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if strings.Contains(s, " ") {
		return strings.TrimSpace(s)
	}
	// Scene names use dots as separators, but "7.1" / "5.1.4" are audio layouts.
	r := []rune(s)
	for i, c := range r {
		if c != '.' {
			continue
		}
		left := 0
		for j := i - 1; j >= 0 && unicode.IsDigit(r[j]); j-- {
			left++
		}
		right := 0
		for j := i + 1; j < len(r) && unicode.IsDigit(r[j]); j++ {
			right++
		}
		if left >= 1 && left <= 2 && right >= 1 && right <= 2 {
			continue // audio layout, keep the dot
		}
		r[i] = ' '
	}
	return strings.Join(strings.Fields(string(r)), " ")
}

// showFolder returns the nearest ancestor directory that is not a season folder.
func showFolder(path string) string {
	dir := filepath.Dir(path)
	for i := 0; i < 4; i++ {
		name := filepath.Base(dir)
		if name == "." || name == "" || name == string(filepath.Separator) || name == "/" {
			break
		}
		if !reSeasonDir.MatchString(name) {
			return name
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func parseEpisode(s string) (Result, bool) {
	if m := reSxxExx.FindStringSubmatchIndex(s); m != nil {
		season, _ := strconv.Atoi(s[m[2]:m[3]])
		ep, _ := strconv.Atoi(s[m[4]:m[5]])
		res := Result{Kind: KindEpisode, Season: season, Episode: ep, EpisodeEnd: ep}
		if m[6] >= 0 {
			end, _ := strconv.Atoi(s[m[6]:m[7]])
			if end >= ep {
				res.EpisodeEnd = end
			}
		}
		res.Title, res.Year = extractYear(s[:m[0]])
		res.Title = cleanTitle(stripReleaseTags(res.Title))
		res.EpisodeTitle = episodeTitle(s[m[1]:])
		return res, res.Title != ""
	}
	if m := reNxNN.FindStringSubmatchIndex(s); m != nil {
		season, _ := strconv.Atoi(s[m[2]:m[3]])
		ep, _ := strconv.Atoi(s[m[4]:m[5]])
		res := Result{Kind: KindEpisode, Season: season, Episode: ep, EpisodeEnd: ep}
		res.Title, res.Year = extractYear(s[:m[0]])
		res.Title = cleanTitle(stripReleaseTags(res.Title))
		res.EpisodeTitle = episodeTitle(s[m[1]:])
		return res, res.Title != ""
	}
	if m := reSeason.FindStringSubmatchIndex(s); m != nil {
		season, _ := strconv.Atoi(s[m[2]:m[3]])
		ep, _ := strconv.Atoi(s[m[4]:m[5]])
		res := Result{Kind: KindEpisode, Season: season, Episode: ep, EpisodeEnd: ep}
		res.Title, res.Year = extractYear(s[:m[0]])
		res.Title = cleanTitle(stripReleaseTags(res.Title))
		return res, res.Title != ""
	}
	if m := reEpOnly.FindStringSubmatchIndex(s); m != nil {
		ep, _ := strconv.Atoi(s[m[2]:m[3]])
		if ep > 0 {
			res := Result{Kind: KindEpisode, Season: 1, Episode: ep, EpisodeEnd: ep}
			res.Title, res.Year = extractYear(s[:m[0]])
			res.Title = cleanTitle(stripReleaseTags(res.Title))
			return res, res.Title != ""
		}
	}
	return Result{}, false
}

func finishEpisode(res *Result, folder string) {
	if res.Title == "" && folder != "" {
		t, y := extractYear(reYearGroup.ReplaceAllString(folder, "$1"))
		res.Title = cleanTitle(stripReleaseTags(t))
		if res.Year == 0 {
			res.Year = y
		}
	}
	if res.Year == 0 && folder != "" {
		if _, y := extractYear(folder); y > 0 {
			res.Year = y
		}
	}
}

var reReleaseJunk = regexp.MustCompile(`(?i)(1080p|720p|2160p|480p|web[- .]?dl|webrip|bluray|blu-ray|bdrip|hdtv|x ?26[45]|h ?26[45]|hevc|ddp? ?5|ac3|dts|remux|proper|repack|hdr)`)

func episodeTitle(tail string) string {
	if !strings.Contains(tail, " ") {
		// Mixed separators: "Show.S02E01.1080p.WEB-DL" leaves a dot-joined tail.
		tail = strings.ReplaceAll(tail, ".", " ")
	}
	t, _ := extractYear(tail)
	t = stripReleaseTags(t)
	t = cleanTitle(t)
	if reReleaseJunk.MatchString(t) {
		// Still looks like scene junk -> let the caller fall back to "Episode N".
		return ""
	}
	return t
}

// extractYear finds a release year: parenthesised first, then a trailing year token
// in the plausible range.
func extractYear(s string) (string, int) {
	if loc := reYear.FindAllStringSubmatchIndex(s, -1); len(loc) > 0 {
		last := loc[len(loc)-1]
		year, err := strconv.Atoi(s[last[2]:last[3]])
		if err == nil && plausibleYear(year) {
			return strings.TrimSpace(s[:last[0]] + " " + s[last[1]:]), year
		}
	}
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		last := strings.Trim(fields[len(fields)-1], "()[]{}")
		if n, err := strconv.Atoi(last); err == nil && len(last) == 4 && plausibleYear(n) {
			return strings.Join(fields[:len(fields)-1], " "), n
		}
	}
	return s, 0
}

func plausibleYear(y int) bool {
	return y >= 1900 && y <= time.Now().Year()+2
}

// stripReleaseTags removes trailing scene tags (and groups like "-RARBG").
func stripReleaseTags(s string) string {
	fields := strings.Fields(s)
	for len(fields) > 0 {
		raw := fields[len(fields)-1]
		tok := strings.ToLower(strings.Trim(raw, "()[]{}.,;"))
		// "Atmos-FraMeSToR" / "x264-GROUP": drop when the prefix is a known tag.
		dropHyphenated := false
		if i := strings.LastIndex(tok, "-"); i > 0 {
			if releaseTags[strings.Trim(tok[:i], "()[]{}.,;")] {
				dropHyphenated = true
			}
		}
		switch {
		case tok == "":
			fields = fields[:len(fields)-1]
		case releaseTags[tok], reDigits.MatchString(tok), dropHyphenated:
			fields = fields[:len(fields)-1]
		case strings.HasPrefix(tok, "-") && len(tok) > 2:
			fields = fields[:len(fields)-1]
		default:
			goto done
		}
	}
done:
	out := strings.Join(fields, " ")
	out = reCJKTags.ReplaceAllString(out, "")
	out = strings.TrimRight(out, " ._-")
	return strings.TrimSpace(out)
}

func stripEdition(s string) (string, string) {
	fields := strings.Fields(s)
	edition := ""
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		key := strings.ToLower(strings.Trim(f, "()[]{}.,;"))
		key = strings.TrimSuffix(key, "'s")
		if e, ok := editionWords[key]; ok {
			if edition == "" {
				edition = e
			}
			continue
		}
		kept = append(kept, f)
	}
	if edition != "" {
		filters := map[string]bool{"edition": true, "cut": true, "version": true, "ed": true}
		out := kept[:0]
		for _, f := range kept {
			if filters[strings.ToLower(strings.Trim(f, "()[]{}.,;"))] {
				continue
			}
			out = append(out, f)
		}
		kept = out
	}
	return strings.Join(kept, " "), edition
}

// cleanTitle removes bracketed noise, collapses whitespace and strips separators.
func cleanTitle(s string) string {
	for _, re := range reBrackets {
		s = re.ReplaceAllString(s, " ")
	}
	s = reParens.ReplaceAllString(s, " ")
	s = strings.NewReplacer("“", "\"", "”", "\"", "–", "-", "—", "-").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " ._-–—:|")
	// A leading dash is a separator in " - Title", not part of the title.
	s = strings.TrimSpace(strings.TrimLeft(s, "- "))
	return s
}

var genericTitles = map[string]bool{
	"video": true, "movie": true, "film": true, "main": true, "browse": true,
	"title": true, "index": true, "clip": true, "untitled": true, "sample": true,
}

func isGenericTitle(t string) bool {
	low := strings.ToLower(strings.TrimSpace(t))
	if genericTitles[low] {
		return true
	}
	// camera / phone dumps: MVI_1234, IMG_0042, VID_20200101, DSC_0001
	if len(low) >= 4 {
		switch {
		case strings.HasPrefix(low, "mvi"), strings.HasPrefix(low, "img"),
			strings.HasPrefix(low, "vid"), strings.HasPrefix(low, "dsc"),
			strings.HasPrefix(low, "gopr"), strings.HasPrefix(low, "pxl"):
			rest := low[3:]
			rest = strings.TrimLeft(rest, "_ -0123456789")
			return rest == ""
		}
	}
	return false
}

func fallbackTitle(title, stem string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return cleanTitle(stem)
}

func detectExtra(dir, stem string) string {
	hay := strings.ToLower(dir + "/" + stem)
	hay = strings.NewReplacer(" ", "", "_", "", ".", "", "-", "").Replace(hay)
	for _, m := range extraMarkers {
		if m.segmentOnly {
			// require a path segment, not a substring inside a title
			if strings.Contains("/"+hay+"/", "/"+m.word+"/") {
				return m.kind
			}
			continue
		}
		if strings.Contains(hay, m.word) {
			return m.kind
		}
	}
	return ""
}

// SortTitle produces a key for alphabetising (articles stripped, case-folded).
func SortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, a := range leadingArticles {
		if strings.HasPrefix(t, a) {
			t = strings.TrimPrefix(t, a)
			break
		}
	}
	return strings.TrimLeftFunc(t, unicode.IsSpace)
}
