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
