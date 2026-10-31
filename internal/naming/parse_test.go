package naming

import "testing"

type want struct {
	kind    Kind
	title   string
	year    int
	season  int
	episode int
	end     int
	extra   string
	edition string
	epTitle string
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		lib  string
		path string
		want want
	}{
		// ---------- movies: scene & release styles ----------
		{"bracketed release name", "movie", "/m/The King's Speech (2010) (1080p BluRay x265 10bit Tigole).mkv",
			want{kind: KindMovie, title: "The King's Speech", year: 2010}},
		{"scene dots", "movie", "/m/The.Kings.Speech.2010.1080p.BluRay.x264-GROUP.mkv",
			want{kind: KindMovie, title: "The Kings Speech", year: 2010}},
		{"year in title", "movie", "/m/Blade Runner 2049 (2017) 2160p.mkv",
			want{kind: KindMovie, title: "Blade Runner 2049", year: 2017}},
		{"numeric title", "movie", "/m/1917.2019.1080p.WEB-DL.x264.mkv",
			want{kind: KindMovie, title: "1917", year: 2019}},
		{"2001 title", "movie", "/m/2001 A Space Odyssey (1968).mkv",
			want{kind: KindMovie, title: "2001 A Space Odyssey", year: 1968}},
		{"accented", "movie", "/m/Amélie (2001).mkv",
			want{kind: KindMovie, title: "Amélie", year: 2001}},
		{"cjk title", "movie", "/m/霸王别姬(1993).mkv",
			want{kind: KindMovie, title: "霸王别姬", year: 1993}},
		{"cjk junk suffix", "movie", "/m/【高清MP4电影www.domp4.cc】哈利波特7：死神的圣物2BD国英双语中英双字.mkv",
			want{kind: KindMovie, title: "哈利波特7：死神的圣物2"}},
		{"hyphenated + group", "movie", "/m/Spider-Man.No.Way.Home.2021.1080p.BluRay.x264-SPARKS.mkv",
			want{kind: KindMovie, title: "Spider-Man No Way Home", year: 2021}},
		{"dotted title kept", "movie", "/m/Dr. Strangelove or How I Learned to Stop Worrying and Love the Bomb (1964).mkv",
			want{kind: KindMovie, title: "Dr. Strangelove or How I Learned to Stop Worrying and Love the Bomb", year: 1964}},
		{"remux stack", "movie", "/m/Oppenheimer.2023.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.7.1.Atmos-FraMeSToR.mkv",
			want{kind: KindMovie, title: "Oppenheimer", year: 2023}},
		{"extended edition", "movie", "/m/The Lord of the Rings The Fellowship of the Ring (2001) Extended Edition.mkv",
			want{kind: KindMovie, title: "The Lord of the Rings The Fellowship of the Ring", year: 2001, edition: "Extended"}},
		{"bracket tags", "movie", "/m/Parasite (2019) [1080p] [BluRay] [x265].mkv",
			want{kind: KindMovie, title: "Parasite", year: 2019}},
		{"imdb brace", "movie", "/m/Joker (2019) {imdb-tt7286456}.mkv",
			want{kind: KindMovie, title: "Joker", year: 2019}},
		{"underscores", "movie", "/m/Spirited_Away_2001_1080p.mkv",
			want{kind: KindMovie, title: "Spirited Away", year: 2001}},
		{"pixar short-ish", "movie", "/m/Up (2009).mkv",
			want{kind: KindMovie, title: "Up", year: 2009}},
		{"no year", "movie", "/m/Wedding Video.mp4",
			want{kind: KindMovie, title: "Wedding Video"}},
		{"subdirectory ignored", "movie", "/m/Dune Part Two (2024)/Dune.Part.Two.2024.1080p.mkv",
			want{kind: KindMovie, title: "Dune Part Two", year: 2024}},

		// ---------- tv ----------
		{"tv dash title", "tv", "/tv/For All Mankind/Season 1/For All Mankind - S01E01 - Red Moon.mkv",
			want{kind: KindEpisode, title: "For All Mankind", season: 1, episode: 1, end: 1, epTitle: "Red Moon"}},
		{"tv scene", "tv", "/tv/For.All.Mankind.S01E01.1080p.ATVP.WEB-DL.DDP5.1.x264-GROUP.mkv",
			want{kind: KindEpisode, title: "For All Mankind", season: 1, episode: 1, end: 1}},
		{"tv 4k", "tv", "/tv/Foundation.S02E03.2160p.WEB.mkv",
			want{kind: KindEpisode, title: "Foundation", season: 2, episode: 3, end: 3}},
		{"tv 1x02", "tv", "/tv/Show - 1x02 - Title.mkv",
			want{kind: KindEpisode, title: "Show", season: 1, episode: 2, end: 2, epTitle: "Title"}},
		{"tv verbose", "tv", "/tv/Show Season 2 Episode 4.mkv",
			want{kind: KindEpisode, title: "Show", season: 2, episode: 4, end: 4}},
		{"tv multi ep glued", "tv", "/tv/Show.S01E01E02.1080p.mkv",
			want{kind: KindEpisode, title: "Show", season: 1, episode: 1, end: 2}},
		{"tv multi ep dash", "tv", "/tv/Show.S01E01-E02.mkv",
			want{kind: KindEpisode, title: "Show", season: 1, episode: 1, end: 2}},
		{"tv show year", "tv", "/tv/Show (2019)/Season 01/Show - S01E05.mkv",
			want{kind: KindEpisode, title: "Show", year: 2019, season: 1, episode: 5, end: 5}},
		{"tv episode only", "tv", "/tv/Anime - E05.mkv",
			want{kind: KindEpisode, title: "Anime", season: 1, episode: 5, end: 5}},
		{"tv cjk episode title", "tv", "/tv/Show - S01E02 - 第2集.mkv",
			want{kind: KindEpisode, title: "Show", season: 1, episode: 2, end: 2, epTitle: "第2集"}},
		{"tv dotted tail tags", "tv", "/tv/For All Mankind.S02E01.1080p.WEB-DL.mkv",
			want{kind: KindEpisode, title: "For All Mankind", season: 2, episode: 1, end: 1}},
		{"tv lowercase", "tv", "/tv/show.s01e02.720p.mkv",
			want{kind: KindEpisode, title: "show", season: 1, episode: 2, end: 2}},
		{"tv unidentifiable", "tv", "/tv/Some Movie (2010).mkv",
			want{kind: KindUnknown, title: "Some Movie"}},

		// ---------- home videos ----------
		{"home video", "home_video", "/hv/fiji-2023Sep.mp4",
			want{kind: KindHomeVideo, title: "fiji-2023Sep"}},
		{"home video underscores", "home_video", "/hv/hamilton_island_2021-Dec.mp4",
			want{kind: KindHomeVideo, title: "hamilton island 2021-Dec"}},

		// ---------- extras ----------
		{"sample", "movie", "/m/Movie/sample.mkv",
			want{kind: KindExtra, extra: "sample", title: "sample"}},
		{"trailer", "movie", "/m/Movie/trailer.mp4",
			want{kind: KindExtra, extra: "trailer", title: "trailer"}},
		{"deleted scenes", "movie", "/m/Movie/Extras/deleted-scenes.mkv",
			want{kind: KindExtra, extra: "deleted_scenes", title: "deleted-scenes"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.lib, tc.path)
			w := tc.want
			if got.Kind != w.kind {
				t.Fatalf("kind = %q, want %q (result %+v)", got.Kind, w.kind, got)
			}
			if got.Title != w.title {
				t.Errorf("title = %q, want %q", got.Title, w.title)
			}
			if got.Year != w.year {
				t.Errorf("year = %d, want %d", got.Year, w.year)
			}
			if got.Season != w.season {
				t.Errorf("season = %d, want %d", got.Season, w.season)
			}
			if got.Episode != w.episode {
				t.Errorf("episode = %d, want %d", got.Episode, w.episode)
			}
			if w.end != 0 && got.EpisodeEnd != w.end {
				t.Errorf("episodeEnd = %d, want %d", got.EpisodeEnd, w.end)
			}
			if got.Extra != w.extra {
				t.Errorf("extra = %q, want %q", got.Extra, w.extra)
			}
			if w.edition != "" && got.Edition != w.edition {
				t.Errorf("edition = %q, want %q", got.Edition, w.edition)
			}
			if w.epTitle != "" && got.EpisodeTitle != w.epTitle {
				t.Errorf("episodeTitle = %q, want %q", got.EpisodeTitle, w.epTitle)
			}
		})
	}
}

func TestIsVideoFile(t *testing.T) {
	yes := []string{"a.mkv", "b.MP4", "c.webm", "d.ts", "e.m2ts", "f.avi", "g.rmvb"}
	no := []string{"a.srt", "b.jpg", "c.nfo", "d", "e.txt", "f.iso"}
	for _, n := range yes {
		if !IsVideoFile(n) {
			t.Errorf("IsVideoFile(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if IsVideoFile(n) {
			t.Errorf("IsVideoFile(%q) = true, want false", n)
		}
	}
}

func TestSortTitle(t *testing.T) {
	cases := map[string]string{
		"The Matrix":     "matrix",
		"A Quiet Place":  "quiet place",
		"An Education":   "education",
		"Theatre of War": "theatre of war",
		"2046":           "2046",
	}
	for in, want := range cases {
		if got := SortTitle(in); got != want {
			t.Errorf("SortTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
