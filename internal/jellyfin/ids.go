// Package jellyfin holds the pure pieces of the Jellyfin compatibility adapter:
// identifier encoding, DTO shapes and constants. HTTP handlers live in
// internal/httpapi.
package jellyfin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// Kind identifies what an encoded id refers to.
type Kind uint32

const (
	KindUser    Kind = 1
	KindLibrary Kind = 2
	KindItem    Kind = 3
	KindFile    Kind = 4 // media source (one playable file)
	KindGenre   Kind = 5
	KindPerson  Kind = 6
)

// idLen is the length of an encoded id (32 lowercase hex characters).
const idLen = 32

// Codec turns numeric ids into opaque, non-guessable 32-hex ids.
//
// The numeric part is encrypted with a 4-round Feistel network keyed by the
// server secret (HMAC-SHA256). Sequential database ids therefore look random,
// which is what lets Jellyfin clients fetch artwork/streams without auth
// headers (the same model as Jellyfin, whose ids are GUIDs).
type Codec struct {
	key []byte
}

// NewCodec derives a codec key from a server secret.
func NewCodec(secret string) *Codec {
	sum := sha256.Sum256([]byte("mvs-jellyfin-ids:" + secret))
	return &Codec{key: sum[:]}
}

var (
	defaultCodec = NewCodec("mvs-default")
	activeCodec  atomic.Pointer[Codec]
)

// SetCodec installs the process-wide codec. Called once from the composition
// root with a key derived from the persisted server id; tests use the default.
func SetCodec(c *Codec) {
	if c != nil {
		activeCodec.Store(c)
	}
}

func codec() *Codec {
	if c := activeCodec.Load(); c != nil {
		return c
	}
	return defaultCodec
}

// Encode renders an opaque, reversible 32-hex id.
func Encode(k Kind, id int64) string {
	return codec().Encode(k, id)
}

// Decode parses an encoded id and returns its kind and numeric id.
func Decode(raw string) (Kind, int64, error) {
	return codec().Decode(raw)
}

// DecodeKind parses raw and verifies the expected kind.
func DecodeKind(raw string, want Kind) (int64, error) {
	return codec().DecodeKind(raw, want)
}

// DecodeKind parses raw and verifies the expected kind.
func (c *Codec) DecodeKind(raw string, want Kind) (int64, error) {
	k, id, err := c.Decode(raw)
	if err != nil {
		return 0, err
	}
	if k != want {
		return 0, fmt.Errorf("wrong id kind")
	}
	return id, nil
}

// Encode renders kind + encrypted id as 32 hex characters.
func (c *Codec) Encode(k Kind, id int64) string {
	if id <= 0 {
		return ""
	}
	var buf [16]byte
	binary.BigEndian.PutUint32(buf[0:4], uint32(k))
	binary.BigEndian.PutUint64(buf[4:12], c.encrypt(uint64(id)))
	return hex.EncodeToString(buf[:])
}

// Decode reverses Encode and validates the structure. Jellyfin clients turn ids
// into UUIDs and send them back hyphenated, so both the simple (32 hex) and
// hyphenated (8-4-4-4-12) notations are accepted.
func (c *Codec) Decode(raw string) (Kind, int64, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != idLen {
		return 0, 0, fmt.Errorf("invalid id length")
	}
	blob, err := hex.DecodeString(raw)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid id")
	}
	kind := Kind(binary.BigEndian.Uint32(blob[0:4]))
	id := int64(c.decrypt(binary.BigEndian.Uint64(blob[4:12])))
	if id <= 0 {
		return 0, 0, fmt.Errorf("invalid id")
	}
	return kind, id, nil
}

// encrypt runs 4 Feistel rounds: (L,R) = (R, L ^ F(R,i)).
func (c *Codec) encrypt(v uint64) uint64 {
	l, r := uint32(v>>32), uint32(v)
	for i := 0; i < 4; i++ {
		l, r = r, l^c.round(r, i)
	}
	return uint64(l)<<32 | uint64(r)
}

// decrypt runs the rounds backwards: (L,R) = (R ^ F(L,i), L).
func (c *Codec) decrypt(v uint64) uint64 {
	l, r := uint32(v>>32), uint32(v)
	for i := 3; i >= 0; i-- {
		l, r = r^c.round(l, i), l
	}
	return uint64(l)<<32 | uint64(r)
}

func (c *Codec) round(v uint32, round int) uint32 {
	var msg [5]byte
	msg[0] = byte(round)
	binary.BigEndian.PutUint32(msg[1:], v)
	mac := hmac.New(sha256.New, c.key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

// User/ Library/ Item/ File/Genre/Person are convenience encoders.
func UserID(id int64) string    { return Encode(KindUser, id) }
func LibraryID(id int64) string { return Encode(KindLibrary, id) }
func ItemID(id int64) string    { return Encode(KindItem, id) }
func FileID(id int64) string    { return Encode(KindFile, id) }
func GenreID(id int64) string   { return Encode(KindGenre, id) }
func PersonID(id int64) string  { return Encode(KindPerson, id) }

var _ = strconv.Itoa
