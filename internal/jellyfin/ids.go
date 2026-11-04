// Package jellyfin holds the pure pieces of the Jellyfin compatibility adapter:
// identifier encoding, DTO shapes and constants. HTTP handlers live in
// internal/httpapi.
package jellyfin

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind identifies what an encoded id refers to.
type Kind uint32

const (
	KindUser    Kind = 1
	KindLibrary Kind = 2
	KindItem    Kind = 3
	KindFile    Kind = 4 // media source (one playable file)
)

// idLen is the length of an encoded id (32 lowercase hex characters).
const idLen = 32

// Encode renders a stable, reversible 32-hex id. Clients treat ids as opaque;
// this keeps numeric ids out of URLs while allowing cheap decoding.
func Encode(k Kind, id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%08x%08x%016x", uint64(k), 0, uint64(id))
}

// Decode parses an encoded id and returns its kind and numeric id.
func Decode(raw string) (Kind, int64, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if len(raw) != idLen {
		return 0, 0, fmt.Errorf("invalid id length")
	}
	kindPart, idPart := raw[:8], raw[idLen-16:]
	kv, err := strconv.ParseUint(kindPart, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid id")
	}
	iv, err := strconv.ParseUint(idPart, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid id")
	}
	if iv == 0 {
		return 0, 0, fmt.Errorf("invalid id")
	}
	return Kind(kv), int64(iv), nil
}

// DecodeKind parses raw and verifies the expected kind.
func DecodeKind(raw string, want Kind) (int64, error) {
	k, id, err := Decode(raw)
	if err != nil {
		return 0, err
	}
	if k != want {
		return 0, fmt.Errorf("wrong id kind")
	}
	return id, nil
}

// User/ Library/ Item/ File are convenience encoders.
func UserID(id int64) string    { return Encode(KindUser, id) }
func LibraryID(id int64) string { return Encode(KindLibrary, id) }
func ItemID(id int64) string    { return Encode(KindItem, id) }
func FileID(id int64) string    { return Encode(KindFile, id) }
