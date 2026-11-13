package naming

import "testing"

func TestInferEpisode(t *testing.T) {
	cases := []struct {
		show, path string
		season     int
		episode    int
		end        int
		ok         bool
	}{
		{"低智商犯罪", "/tv/低智商犯罪/01.mkv", 1, 1, 1, true},
		{"低智商犯罪", "/tv/低智商犯罪/18-4K.高码率.mkv", 1, 18, 18, true},
		{"我本善良", "/tv/我本善良/我本善良 07.mp4", 1, 7, 7, true},
		{"Show", "/tv/Show/Show 01-02.mkv", 1, 1, 2, true},
		{"Show", "/tv/Show/第5集.mkv", 1, 5, 5, true},
		{"Show", "/tv/Show/Season 02/03.mkv", 2, 3, 3, true},
		{"Show", "/tv/Show/2024.mkv", 0, 0, 0, false},
		{"Show", "/tv/Show/1080p.mkv", 0, 0, 0, false},
		{"Show", "/tv/Show/trailer.mkv", 0, 0, 0, false},
	}
	for _, tc := range cases {
		season, episode, end, ok := InferEpisode(tc.show, tc.path)
		if ok != tc.ok || (ok && (season != tc.season || episode != tc.episode || end != tc.end)) {
			t.Errorf("InferEpisode(%q, %q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				tc.show, tc.path, season, episode, end, ok, tc.season, tc.episode, tc.end, tc.ok)
		}
	}
}

func TestConsensusEpisodeNumbers(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  map[string]int
	}{
		{
			"chapter markers with reversed title",
			[]string{
				"/tv/Show/The Greed of Man - 大时代 CH01.mkv",
				"/tv/Show/The Greed of Man - 大时代 CH02.mkv",
				"/tv/Show/The Greed of Man - 大时代 CH40 End.mkv",
			},
			map[string]int{
				"/tv/Show/The Greed of Man - 大时代 CH01.mkv":     1,
				"/tv/Show/The Greed of Man - 大时代 CH02.mkv":     2,
				"/tv/Show/The Greed of Man - 大时代 CH40 End.mkv": 40,
			},
		},
		{
			"bare numbers and 4K suffix",
			[]string{"/tv/Show/01.mkv", "/tv/Show/02.mkv", "/tv/Show/18-4K.高码率.mkv"},
			map[string]int{
				"/tv/Show/01.mkv":        1,
				"/tv/Show/02.mkv":        2,
				"/tv/Show/18-4K.高码率.mkv": 18,
			},
		},
		{
			"digit boundary",
			[]string{"/tv/Show/Show 1.mkv", "/tv/Show/Show 10.mkv"},
			map[string]int{"/tv/Show/Show 1.mkv": 1, "/tv/Show/Show 10.mkv": 10},
		},
	}
	for _, tc := range cases {
		got := ConsensusEpisodeNumbers(tc.paths)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for p, want := range tc.want {
			if got[p] != want {
				t.Errorf("%s: %s = %d, want %d", tc.name, p, got[p], want)
			}
		}
	}
}
