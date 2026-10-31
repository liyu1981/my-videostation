package sidecar

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func varint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}
func tag(field, wire int) []byte { return varint(uint64(field<<3 | wire)) }
func str(field int, s string) []byte {
	b := []byte(s)
	return append(append(tag(field, 2), varint(uint64(len(b)))...), b...)
}
func num(field int, v uint64) []byte { return append(tag(field, 0), varint(v)...) }
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func vsmetaFixture(title string) []byte {
	img := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1}, make([]byte, 32)...)
	credits := cat(str(1, "Actor One"), str(1, "Actor Two"), str(2, "Director One"), str(3, "Drama"))
	return cat(
		num(1, 1),
		str(2, title),
		num(5, 2021),
		str(8, "VS overview."),
		str(10, string(credits)),
		num(12, 75),
		str(17, base64.StdEncoding.EncodeToString(img)),
		str(18, "poster-md5"),
	)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergesNFOOverVSMeta(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	video := filepath.Join(dir, "Movie (2021)", "Movie.mkv")
	writeFile(t, video, []byte("video"))
	writeFile(t, video+".vsmeta", vsmetaFixture("VS Title"))
	writeFile(t, filepath.Join(dir, "Movie (2021)", "Movie.nfo"), []byte(
		`<movie><title>NFO Title</title><year>2021</year><plot>NFO plot.</plot><genre>History</genre></movie>`))

	res := Load(video, cache)
	if res == nil || res.File == nil {
		t.Fatal("no file metadata")
	}
	m := res.File
	if m.Title != "NFO Title" {
		t.Errorf("title = %q, want NFO title", m.Title)
	}
	if m.Overview != "NFO plot." {
		t.Errorf("overview = %q", m.Overview)
	}
	if len(m.Genres) != 1 || m.Genres[0] != "History" {
		t.Errorf("genres = %v", m.Genres)
	}
	if m.Year != 2021 {
		t.Errorf("year = %d", m.Year)
	}
	if m.Rating != 7.5 {
		t.Errorf("rating = %v, want vsmeta 7.5", m.Rating)
	}
	if len(m.Cast) != 2 {
		t.Errorf("cast should come from vsmeta when NFO has none: %v", m.Cast)
	}
	if m.Source != "vsmeta+nfo" {
		t.Errorf("source = %q", m.Source)
	}
	if m.PosterPath == "" {
		t.Fatal("expected vsmeta poster to be cached")
	}
	if _, err := os.Stat(m.PosterPath); err != nil {
		t.Fatalf("cached poster missing: %v", err)
	}
}

func TestLoadPrefersNFOArtwork(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "M", "M.mkv")
	writeFile(t, video, []byte("video"))
	writeFile(t, video+".vsmeta", vsmetaFixture("M"))
	writeFile(t, filepath.Join(dir, "M", "poster.jpg"), []byte("jpg"))
	writeFile(t, filepath.Join(dir, "M", "M.nfo"), []byte(
		`<movie><title>M</title><thumb>poster.jpg</thumb></movie>`))

	res := Load(video, filepath.Join(dir, "cache"))
	if res == nil || res.File == nil || res.File.PosterPath == "" {
		t.Fatal("no metadata")
	}
	if filepath.Base(res.File.PosterPath) != "poster.jpg" {
		t.Errorf("poster = %q, want local NFO thumb", res.File.PosterPath)
	}
}

func TestLoadShowMetadata(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Show", "Season 1", "Show.S01E01.mkv")
	writeFile(t, video, []byte("video"))
	writeFile(t, filepath.Join(dir, "Show", "tvshow.nfo"), []byte(
		`<tvshow><title>Show Title</title><year>2019</year><plot>Show plot.</plot></tvshow>`))

	res := Load(video, "")
	if res == nil || res.Show == nil {
		t.Fatal("no show metadata")
	}
	if res.Show.Title != "Show Title" || res.Show.Year != 2019 {
		t.Fatalf("show = %+v", res.Show)
	}
	if res.File != nil {
		t.Errorf("episode should not have file metadata: %+v", res.File)
	}
}

func TestLoadNoSidecars(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "plain.mkv")
	writeFile(t, video, []byte("video"))
	if res := Load(video, ""); res != nil {
		t.Fatalf("expected nil, got %+v", res)
	}
}
