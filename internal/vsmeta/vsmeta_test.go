package vsmeta

import (
	"encoding/base64"
	"os"
	"testing"
)

// --- tiny protobuf encoder used to build fixtures -------------------------

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

func jpegBytes(t *testing.T, size int) []byte {
	t.Helper()
	b := make([]byte, size)
	copy(b, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01})
	return b
}

func fixture(t *testing.T) []byte {
	t.Helper()
	poster := jpegBytes(t, 64)
	backdrop := jpegBytes(t, 80)
	credits := cat(
		str(1, "Daniel Craig"), str(1, "Ana de Armas"),
		str(2, "Cary Joji Fukunaga"),
		str(3, "Action"), str(3, "Thriller"),
		str(4, "Neal Purvis"), str(4, "Robert Wade"),
	)
	backdropMsg := cat(
		str(1, base64.StdEncoding.EncodeToString(backdrop)),
		str(2, "backdrophash"),
		num(3, 1644315157),
	)
	return cat(
		num(1, 1),
		str(2, "007：无暇赴死"),
		str(3, "No Time to Die"),
		str(4, "The mission that changes everything."),
		num(5, 2021),
		str(6, "2021-09-29"),
		num(7, 1),
		str(8, "Bond has left active service."),
		str(9, `{"com.synology.TheMovieDb":{"poster":["https://img/p.jpg"],"rating":{"themoviedb":7.5},"reference":{"imdb":"tt2382320","themoviedb":370172},"collection_id":{"themoviedb":645}}}`),
		str(10, string(credits)),
		str(11, "PG-13"),
		num(12, 75),
		str(17, base64.StdEncoding.EncodeToString(poster)),
		str(18, "posterhash"),
		str(21, string(backdropMsg)),
	)
}

func TestParseFixture(t *testing.T) {
	m, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != 1 || m.Title != "007：无暇赴死" || m.OriginalTitle != "No Time to Die" {
		t.Fatalf("meta = %+v", m)
	}
	if m.Year != 2021 || m.Rating != 7.5 || m.Certification != "PG-13" {
		t.Errorf("year/rating/cert = %d/%v/%q", m.Year, m.Rating, m.Certification)
	}
	if len(m.Cast) != 2 || len(m.Directors) != 1 || len(m.Genres) != 2 || len(m.Writers) != 2 {
		t.Errorf("credits = cast:%v dir:%v genres:%v writers:%v", m.Cast, m.Directors, m.Genres, m.Writers)
	}
	if m.ProviderIDs["imdb"] != "tt2382320" || m.ProviderIDs["themoviedb"] != "370172" {
		t.Errorf("provider ids = %v", m.ProviderIDs)
	}
	if m.CollectionID != 645 {
		t.Errorf("collection = %d", m.CollectionID)
	}
	poster, err := m.Poster()
	if err != nil || len(poster) != 64 {
		t.Fatalf("poster = %d bytes, err %v", len(poster), err)
	}
	backdrop, err := m.Backdrop()
	if err != nil || len(backdrop) != 80 {
		t.Fatalf("backdrop = %d bytes, err %v", len(backdrop), err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte("not a protobuf"),
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		cat(num(1, 1)), // no title/overview
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("expected error for %q", string(c))
		}
	}
}

// TestParseRealSample validates against a real .vsmeta when provided:
//
//	MVS_VSMETA_SAMPLE=/path/to/file.vsmeta go test ./internal/vsmeta -run RealSample -v
func TestParseRealSample(t *testing.T) {
	path := os.Getenv("MVS_VSMETA_SAMPLE")
	if path == "" {
		t.Skip("set MVS_VSMETA_SAMPLE to a real .vsmeta file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	poster, err := m.Poster()
	if err != nil {
		t.Fatalf("poster: %v", err)
	}
	t.Logf("title=%q original=%q year=%d rating=%.1f cert=%q cast=%d genres=%v poster=%dB backdrop=%dB refs=%v",
		m.Title, m.OriginalTitle, m.Year, m.Rating, m.Certification, len(m.Cast), m.Genres, len(poster), len(mustBackdrop(t, m)), m.ProviderIDs)
	if m.Title == "" || m.Year == 0 || len(m.Cast) == 0 || len(poster) < 1000 {
		t.Fatalf("unexpected parse result: %+v", m)
	}
}

func mustBackdrop(t *testing.T, m *Meta) []byte {
	t.Helper()
	b, err := m.Backdrop()
	if err != nil {
		t.Fatalf("backdrop: %v", err)
	}
	return b
}
