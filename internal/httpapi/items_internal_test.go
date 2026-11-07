package httpapi

import (
	"testing"

	"github.com/liyu1981/myvideostation/internal/store"
)

func TestShowScopeDir(t *testing.T) {
	lib := &store.Library{Paths: []store.LibraryPath{{Path: "/tv"}}}
	cases := []struct {
		dir  string
		want string
	}{
		{"/tv/Alpha Show/Season 01", "/tv/Alpha Show"},
		{"/tv/Alpha Show/Season 01", "/tv/Alpha Show"},
		{"/tv/Alpha Show/Specials", "/tv/Alpha Show"},
		{"/tv/Alpha Show", "/tv/Alpha Show"},
		{"/tv/Genre/Alpha Show/Season 2", "/tv/Genre/Alpha Show"},
		{"/tv/Alpha Show/S01", "/tv/Alpha Show"},
		{"/tv/Season 01", "/tv/Season 01"}, // never climb out of the library root
	}
	for _, tc := range cases {
		if got := showScopeDir(lib, tc.dir); got != tc.want {
			t.Errorf("showScopeDir(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestCommonDir(t *testing.T) {
	cases := []struct {
		dirs []string
		want string
	}{
		{[]string{"/tv/Show/Season 01"}, "/tv/Show/Season 01"},
		{[]string{"/tv/Show/Season 01", "/tv/Show/Season 02"}, "/tv/Show"},
		{[]string{"/tv/Show/Season 01", "/tv/Show"}, "/tv/Show"},
		{[]string{"/tv/A/Season 01", "/tv/B/Season 01"}, "/tv"},
		{[]string{"/tv/Show/Season 01", "/movies/Other"}, "/"},
		{[]string{"/tv/Show (2020)/Season 01", "/tv/Show (2020)/Specials"}, "/tv/Show (2020)"},
	}
	for _, tc := range cases {
		if got := commonDir(tc.dirs); got != tc.want {
			t.Errorf("commonDir(%v) = %q, want %q", tc.dirs, got, tc.want)
		}
	}
}
