package nfo

import (
	"os"
	"path/filepath"
	"testing"
)

const movieXML = `<?xml version="1.0" encoding="utf-8"?>
<movie>
  <title>The King's Speech</title>
  <originaltitle>The King's Speech</originaltitle>
  <year>2010</year>
  <plot>A king finds his voice.</plot>
  <tagline>Find your voice.</tagline>
  <runtime>118</runtime>
  <mpaa>R</mpaa>
  <ratings>
    <rating name="imdb" max="10"><value>8.0</value></rating>
    <rating name="themoviedb" max="10"><value>7.7</value></rating>
  </ratings>
  <genre>Drama</genre>
  <genre>History</genre>
  <actor><name>Colin Firth</name><role>King George VI</role><order>0</order></actor>
  <actor><name>Geoffrey Rush</name><role>Lionel Logue</role><order>1</order></actor>
  <director>Tom Hooper</director>
  <credits>David Seidler</credits>
  <uniqueid type="tmdb">45269</uniqueid>
  <uniqueid type="imdb">tt1504320</uniqueid>
  <thumb aspect="poster">poster.jpg</thumb>
  <fanart><thumb>fanart.jpg</thumb></fanart>
</movie>`

func TestParseMovieFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "poster.jpg"), "jpg")
	write(t, filepath.Join(dir, "fanart.jpg"), "jpg")
	path := filepath.Join(dir, "Movie.nfo")
	write(t, path, movieXML)

	m, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != "movie" || m.Title != "The King's Speech" || m.Year != 2010 {
		t.Fatalf("meta = %+v", m)
	}
	if m.Overview != "A king finds his voice." || m.Tagline != "Find your voice." || m.Certification != "R" {
		t.Errorf("text fields = %+v", m)
	}
	if m.RuntimeMS != 118*60_000 {
		t.Errorf("runtime = %d", m.RuntimeMS)
	}
	if m.Rating != 7.7 {
		t.Errorf("rating = %v, want TMDB 7.7", m.Rating)
	}
	if len(m.Genres) != 2 || len(m.Cast) != 2 || len(m.Directors) != 1 || len(m.Writers) != 1 {
		t.Errorf("credits = %+v", m)
	}
	if m.ProviderIDs["tmdb"] != "45269" || m.ProviderIDs["imdb"] != "tt1504320" {
		t.Errorf("ids = %v", m.ProviderIDs)
	}
	if filepath.Base(m.PosterPath) != "poster.jpg" || filepath.Base(m.BackdropPath) != "fanart.jpg" {
		t.Errorf("artwork = %q / %q", m.PosterPath, m.BackdropPath)
	}
}

func TestParseEpisodeAndShow(t *testing.T) {
	ep, err := Parse([]byte(`<episodedetails>
		<title>Red Moon</title><season>1</season><episode>1</episode>
		<plot>Pilot.</plot><year>2019</year><genre>Drama</genre>
	</episodedetails>`))
	if err != nil {
		t.Fatal(err)
	}
	if ep.Kind != "episode" || ep.Title != "Red Moon" || ep.Season != 1 || ep.Episode != 1 {
		t.Fatalf("episode = %+v", ep)
	}

	show, err := Parse([]byte(`<tvshow><title>For All Mankind</title><year>2019</year>
		<plot>Space race.</plot><genre>Drama</genre><genre>Sci-Fi</genre>
		<actor><name>Joel Kinnaman</name></actor>
	</tvshow>`))
	if err != nil {
		t.Fatal(err)
	}
	if show.Kind != "tvshow" || show.Title != "For All Mankind" || len(show.Genres) != 2 || len(show.Cast) != 1 {
		t.Fatalf("show = %+v", show)
	}
}

func TestRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "not xml", "<foo><title>x</title></foo>", "<movie></movie>"} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

func TestRemoteThumbsIgnored(t *testing.T) {
	m, err := Parse([]byte(`<movie><title>X</title><thumb>https://example.com/p.jpg</thumb></movie>`))
	if err != nil {
		t.Fatal(err)
	}
	if m.PosterPath != "" {
		t.Errorf("remote thumb should be ignored, got %q", m.PosterPath)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
