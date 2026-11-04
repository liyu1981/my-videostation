package jellyfin

import "testing"

func TestEncodedIDsRoundTrip(t *testing.T) {
	cases := []struct {
		kind Kind
		id   int64
	}{
		{KindUser, 1},
		{KindLibrary, 7},
		{KindItem, 123456789},
		{KindFile, 42},
	}
	for _, tc := range cases {
		enc := Encode(tc.kind, tc.id)
		if len(enc) != idLen {
			t.Fatalf("Encode(%d,%d) = %q, len %d", tc.kind, tc.id, enc, len(enc))
		}
		k, id, err := Decode(enc)
		if err != nil {
			t.Fatalf("Decode(%q): %v", enc, err)
		}
		if k != tc.kind || id != tc.id {
			t.Fatalf("round trip = (%d,%d), want (%d,%d)", k, id, tc.kind, tc.id)
		}
	}
}

func TestEncodedIDsAreStableAndDistinct(t *testing.T) {
	if ItemID(1) != ItemID(1) {
		t.Fatal("ids must be deterministic")
	}
	if ItemID(1) == FileID(1) {
		t.Fatal("different kinds must not collide")
	}
	if ItemID(0) != "" {
		t.Fatal("zero id must encode to empty")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "abc", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "00000000000000000000000000000000"} {
		if _, _, err := Decode(raw); err == nil {
			t.Errorf("Decode(%q) should fail", raw)
		}
	}
	if _, err := DecodeKind(FileID(3), KindItem); err == nil {
		t.Error("DecodeKind must reject the wrong kind")
	}
	if id, err := DecodeKind(ItemID(3), KindItem); err != nil || id != 3 {
		t.Errorf("DecodeKind(ItemID(3)) = %d, %v", id, err)
	}
}
