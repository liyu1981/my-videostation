// Package vsmeta parses Synology Video Station .vsmeta sidecar files.
//
// The format is a protobuf message without a published schema. Field numbers were
// recovered by inspecting real files (see docs/VSMETA-FORMAT.md):
//
//	1  varint   media kind (1 = movie)
//	2  string   title (localised)
//	3  string   original title
//	4  string   tagline
//	5  varint   year
//	6  string   release date (YYYY-MM-DD)
//	7  varint   unknown flag (1 in movie samples)
//	8  string   overview / plot
//	9  string   provider JSON, e.g. {"com.synology.TheMovieDb":{...}}
//	10 message  credits: 1=cast[], 2=directors[], 3=genres[], 4=writers[]
//	11 string   content rating ("PG-13")
//	12 varint   rating × 10
//	17 string   poster JPEG, base64
//	18 string   poster MD5
//	21 message  backdrop: 1=base64 JPEG, 2=MD5, 3=unix timestamp
package vsmeta

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Meta is the subset of .vsmeta we understand.
type Meta struct {
	Kind           int
	Title          string
	OriginalTitle  string
	Tagline        string
	Overview       string
	Certification  string
	Year           int
	ReleaseDate    string
	Rating         float64
	Genres         []string
	Cast           []string
	Directors      []string
	Writers        []string
	PosterBase64   string
	PosterHash     string
	BackdropBase64 string
	BackdropHash   string
	ProviderIDs    map[string]string
	CollectionID   int64
}

// ErrInvalid is returned when the payload is not a plausible .vsmeta document.
var ErrInvalid = errors.New("not a vsmeta file")

type reader struct {
	b []byte
	i int
}

func (r *reader) varint() (uint64, error) {
	var v uint64
	var shift uint
	for {
		if r.i >= len(r.b) {
			return 0, ioErr()
		}
		c := r.b[r.i]
		r.i++
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return v, nil
		}
		shift += 7
		if shift > 63 {
			return 0, fmt.Errorf("vsmeta: varint overflow")
		}
	}
}

func (r *reader) bytes() ([]byte, error) {
	n, err := r.varint()
	if err != nil {
		return nil, err
	}
	if n > uint64(len(r.b)-r.i) {
		return nil, ioErr()
	}
	out := r.b[r.i : r.i+int(n)]
	r.i += int(n)
	return out, nil
}

func (r *reader) skip(wire uint64) error {
	switch wire {
	case 0:
		_, err := r.varint()
		return err
	case 1:
		if r.i+8 > len(r.b) {
			return ioErr()
		}
		r.i += 8
		return nil
	case 2:
		_, err := r.bytes()
		return err
	case 5:
		if r.i+4 > len(r.b) {
			return ioErr()
		}
		r.i += 4
		return nil
	default:
		return fmt.Errorf("vsmeta: unsupported wire type %d", wire)
	}
}

func ioErr() error { return errors.New("vsmeta: truncated message") }

// Parse decodes a .vsmeta payload.
func Parse(data []byte) (*Meta, error) {
	if len(data) == 0 {
		return nil, ErrInvalid
	}
	m := &Meta{ProviderIDs: map[string]string{}}
	r := &reader{b: data}
	fields := 0

	for r.i < len(r.b) {
		key, err := r.varint()
		if err != nil {
			return nil, err
		}
		field, wire := key>>3, key&7
		switch field {
		case 1:
			v, err := r.varint()
			if err != nil {
				return nil, err
			}
			m.Kind = int(v)
			fields++
		case 2, 3, 4, 6, 8, 11:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			s := strings.TrimSpace(string(b))
			switch field {
			case 2:
				m.Title = s
			case 3:
				m.OriginalTitle = s
			case 4:
				m.Tagline = s
			case 6:
				m.ReleaseDate = s
			case 8:
				m.Overview = s
			case 11:
				m.Certification = s
			}
			fields++
		case 5:
			v, err := r.varint()
			if err != nil {
				return nil, err
			}
			m.Year = int(v)
			fields++
		case 7:
			if _, err := r.varint(); err != nil {
				return nil, err
			}
		case 9:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			parseProviderJSON(m, b)
		case 10:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			parseCredits(m, b)
			fields++
		case 12:
			v, err := r.varint()
			if err != nil {
				return nil, err
			}
			if m.Rating == 0 {
				m.Rating = float64(v) / 10
			}
			fields++
		case 17:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			m.PosterBase64 = string(b)
		case 18:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			m.PosterHash = strings.TrimSpace(string(b))
		case 21:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			parseBackdrop(m, b)
		default:
			if err := r.skip(wire); err != nil {
				return nil, err
			}
		}
	}

	if fields == 0 || (m.Title == "" && m.Overview == "") {
		return nil, ErrInvalid
	}
	return m, nil
}

func parseCredits(m *Meta, b []byte) {
	r := &reader{b: b}
	for r.i < len(r.b) {
		key, err := r.varint()
		if err != nil {
			return
		}
		field, wire := key>>3, key&7
		if wire != 2 {
			if err := r.skip(wire); err != nil {
				return
			}
			continue
		}
		chunk, err := r.bytes()
		if err != nil {
			return
		}
		s := strings.TrimSpace(string(chunk))
		if s == "" {
			continue
		}
		switch field {
		case 1:
			m.Cast = append(m.Cast, s)
		case 2:
			m.Directors = append(m.Directors, s)
		case 3:
			m.Genres = append(m.Genres, s)
		case 4:
			m.Writers = append(m.Writers, s)
		}
	}
}

func parseBackdrop(m *Meta, b []byte) {
	r := &reader{b: b}
	for r.i < len(r.b) {
		key, err := r.varint()
		if err != nil {
			return
		}
		field, wire := key>>3, key&7
		switch {
		case field == 1 && wire == 2:
			chunk, err := r.bytes()
			if err != nil {
				return
			}
			m.BackdropBase64 = string(chunk)
		case field == 2 && wire == 2:
			chunk, err := r.bytes()
			if err != nil {
				return
			}
			m.BackdropHash = strings.TrimSpace(string(chunk))
		default:
			if err := r.skip(wire); err != nil {
				return
			}
		}
	}
}

type providerJSON struct {
	Backdrop     []string           `json:"backdrop"`
	Poster       []string           `json:"poster"`
	Rating       map[string]float64 `json:"rating"`
	Reference    map[string]any     `json:"reference"`
	CollectionID map[string]int64   `json:"collection_id"`
}

func parseProviderJSON(m *Meta, b []byte) {
	var raw map[string]providerJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return
	}
	for _, p := range raw {
		for k, raw := range p.Reference {
			var val string
			switch t := raw.(type) {
			case string:
				val = t
			case float64:
				val = strconv.FormatInt(int64(t), 10)
			case json.Number:
				val = t.String()
			}
			if val != "" {
				m.ProviderIDs[strings.ToLower(k)] = val
			}
		}
		if m.Rating == 0 {
			for _, v := range p.Rating {
				if v > 0 {
					m.Rating = v
					break
				}
			}
		}
		for _, v := range p.CollectionID {
			if v > 0 && m.CollectionID == 0 {
				m.CollectionID = v
			}
		}
	}
}

// Poster decodes the embedded poster image (nil when absent or malformed).
func (m *Meta) Poster() ([]byte, error) { return decodeImage(m.PosterBase64) }

// Backdrop decodes the embedded backdrop image.
func (m *Meta) Backdrop() ([]byte, error) { return decodeImage(m.BackdropBase64) }

func decodeImage(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("vsmeta: no image")
	}
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("vsmeta: bad base64 image: %w", err)
		}
	}
	if len(raw) < 12 {
		return nil, errors.New("vsmeta: image too small")
	}
	if !(raw[0] == 0xFF && raw[1] == 0xD8) && !(raw[0] == 0x89 && raw[1] == 'P') {
		return nil, errors.New("vsmeta: image is not JPEG/PNG")
	}
	return raw, nil
}
