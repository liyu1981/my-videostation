package scanrules

import (
	"strings"
	"testing"
)

func TestExcludeSegmentPatterns(t *testing.T) {
	r := New(Options{ExcludeGlobs: []string{"Sample", "@eaDir", "*.sample.mkv"}})
	cases := []struct {
		rel  string
		want bool
	}{
		{"Movies/Film/Sample/clip.mkv", true},
		{"Movies/Film/sample", true}, // case-insensitive
		{"Movies/@eaDir/poster.mkv", true},
		{"Movies/Film/Sample.mkv", false},
		{"Movies/Film/Film.sample.mkv", true},
		{"Movies/Film/Other.sample.mkv", true},
		{"Movies/Film/film.mkv", false},
	}
	for _, tc := range cases {
		if got := r.Excluded(tc.rel); got != tc.want {
			t.Errorf("Excluded(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestExcludePathPatterns(t *testing.T) {
	r := New(Options{ExcludeGlobs: []string{
		"Extras",
		"Shows/*/Season 0/**",
		"**/Extras/**",
		"Movies/*.mkv",
	}})
	cases := []struct {
		rel  string
		want bool
	}{
		{"Extras/deleted.mkv", true},
		{"Shows/Show A/Extras/behind.mkv", true}, // anywhere pattern
		{"Shows/Show A/Season 0/pilot.mkv", true},
		{"Shows/Show A/Season 1/pilot.mkv", false},
		{"Shows/Show A/Season 0", true},
		{"Movies/Film.mkv", true},
		{"Movies/Sub/Film.mkv", false}, // '*' matches exactly one segment
	}
	for _, tc := range cases {
		if got := r.Excluded(tc.rel); got != tc.want {
			t.Errorf("Excluded(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestDoubleStarVariants(t *testing.T) {
	r := New(Options{ExcludeGlobs: []string{"**/Sample/**"}})
	for _, rel := range []string{
		"Sample/a.mkv",
		"Film/Sample/a.mkv",
		"a/b/c/Sample/d/e.mkv",
	} {
		if !r.Excluded(rel) {
			t.Errorf("Excluded(%q) = false, want true", rel)
		}
	}
	if r.Excluded("Film/Samples/a.mkv") {
		t.Error("Samples should not match Sample")
	}
}

func TestIncludeWhitelist(t *testing.T) {
	r := New(Options{IncludeGlobs: []string{"*.mkv", "Movies/**/*.mp4"}})
	cases := []struct {
		rel  string
		want bool
	}{
		{"Movies/Film/film.mkv", false},
		{"Movies/Film/film.mp4", false},
		{"Movies/Film/film.avi", true},
		{"Movies/Sub/film.mp4", false},
		{"Music/song.mp4", true},
	}
	for _, tc := range cases {
		skip, _ := r.SkipFile(tc.rel, 1024)
		if skip != tc.want {
			t.Errorf("SkipFile(%q) = %v, want %v", tc.rel, skip, tc.want)
		}
	}
}

func TestMinSize(t *testing.T) {
	r := New(Options{MinFileSizeMB: 2})
	if skip, why := r.SkipFile("a.mkv", 1024); !skip || why == "" {
		t.Fatalf("small file should be skipped with a reason, got %v %q", skip, why)
	}
	if skip, _ := r.SkipFile("a.mkv", 3*1024*1024); skip {
		t.Fatal("large file should be kept")
	}
	if skip, _ := New(Options{}).SkipFile("a.mkv", 0); skip {
		t.Fatal("no rules should not skip empty files")
	}
}

func TestSkipExtras(t *testing.T) {
	if New(Options{}).SkipExtras() {
		t.Fatal("extras are indexed by default")
	}
	if !New(Options{SkipExtras: true}).SkipExtras() {
		t.Fatal("skipExtras must be honoured")
	}
}

func TestNormalisation(t *testing.T) {
	r := New(Options{ExcludeGlobs: []string{"./Foo/", `Bar\Baz`, "  qux  "}})
	for _, rel := range []string{"Foo/a.mkv", "Bar/Baz", "qux/a.mkv"} {
		if !r.Excluded(rel) {
			t.Errorf("Excluded(%q) = false after normalisation", rel)
		}
	}
}

func TestMalformedPatternDoesNotPanic(t *testing.T) {
	// Validate() rejects these through the API; the matcher must stay safe if
	// one ever reaches it (e.g. a row written by an older version).
	r := New(Options{ExcludeGlobs: []string{"[unclosed", "a[b"}})
	_ = r.Excluded("[unclosed/x.mkv")
	_ = r.Excluded("a[b/x.mkv")
}

func TestValidate(t *testing.T) {
	got, err := Validate([]string{" Sample ", "", "sample", "./Extras/", "**/x"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Sample", "Extras", "**/x"}
	if len(got) != len(want) {
		t.Fatalf("Validate = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Validate = %v, want %v", got, want)
		}
	}

	for _, bad := range []string{"/etc/passwd", "../escape", "foo/[unclosed"} {
		if _, err := Validate([]string{bad}); err == nil {
			t.Errorf("Validate(%q) should fail", bad)
		}
	}

	long := strings.Repeat("a", 201)
	if _, err := Validate([]string{long}); err == nil {
		t.Error("long pattern should fail")
	}
}
