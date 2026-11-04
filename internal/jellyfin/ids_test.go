package jellyfin

import (
	"strings"
	"testing"
)

func TestEncodedIDsRoundTrip(t *testing.T) {
	c := NewCodec("test-secret")
	cases := []struct {
		kind Kind
		id   int64
	}{
		{KindUser, 1},
		{KindLibrary, 7},
		{KindItem, 123456789},
		{KindFile, 42},
		{KindGenre, 3},
		{KindPerson, 99},
	}
	for _, tc := range cases {
		enc := c.Encode(tc.kind, tc.id)
		if len(enc) != idLen {
			t.Fatalf("Encode(%d,%d) = %q, len %d", tc.kind, tc.id, enc, len(enc))
		}
		k, id, err := c.Decode(enc)
		if err != nil {
			t.Fatalf("Decode(%q): %v", enc, err)
		}
		if k != tc.kind || id != tc.id {
			t.Fatalf("round trip = (%d,%d), want (%d,%d)", k, id, tc.kind, tc.id)
		}
	}
}

func TestEncodedIDsAreOpaque(t *testing.T) {
	c := NewCodec("test-secret")
	a := c.Encode(KindItem, 1)
	b := c.Encode(KindItem, 2)
	if a == b {
		t.Fatal("ids must differ")
	}
	// Sequential numeric ids must not produce sequential-looking hex ids.
	if strings.TrimPrefix(b, a[:8]) == "" {
		t.Fatal("unexpected common prefix")
	}
	// The encoded id must not contain the plaintext numeric id.
	if strings.Contains(b, "0000000000000002") || strings.Contains(b, "00000001") {
		t.Fatalf("encoded id leaks the numeric id: %q", b)
	}
}

func TestEncodedIDsAreStableAndDistinct(t *testing.T) {
	c := NewCodec("test-secret")
	if c.Encode(KindItem, 1) != c.Encode(KindItem, 1) {
		t.Fatal("ids must be deterministic for one key")
	}
	if c.Encode(KindItem, 1) == c.Encode(KindFile, 1) {
		t.Fatal("different kinds must not collide")
	}
	// The process-wide helpers use one active codec.
	if ItemID(1) != ItemID(1) {
		t.Fatal("package helpers must be deterministic")
	}
	if ItemID(0) != "" {
		t.Fatal("zero id must encode to empty")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	c := NewCodec("test-secret")
	for _, raw := range []string{"", "abc", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "00000000000000000000000000000000"} {
		if _, _, err := c.Decode(raw); err == nil {
			t.Errorf("Decode(%q) should fail", raw)
		}
	}
	if _, err := c.DecodeKind(c.Encode(KindFile, 3), KindItem); err == nil {
		t.Error("DecodeKind must reject the wrong kind")
	}
	if id, err := c.DecodeKind(c.Encode(KindItem, 3), KindItem); err != nil || id != 3 {
		t.Errorf("DecodeKind(Item(3)) = %d, %v", id, err)
	}
}
