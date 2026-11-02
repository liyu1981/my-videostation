package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("garbage", "anything") {
		t.Fatal("malformed hash accepted")
	}
}

func TestValidateUsernameAndPassword(t *testing.T) {
	if err := ValidateUsername("admin"); err != nil {
		t.Errorf("admin rejected: %v", err)
	}
	if err := ValidateUsername("a"); err == nil {
		t.Error("short username accepted")
	}
	if err := ValidateUsername("bad name"); err == nil {
		t.Error("username with space accepted")
	}
	if err := ValidatePassword("short"); err == nil {
		t.Error("short password accepted")
	}
	if err := ValidatePassword("long enough"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}
}

func TestSessionToken(t *testing.T) {
	raw, hash, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || hash == "" || raw == hash {
		t.Fatalf("unexpected token values: %q %q", raw, hash)
	}
	if HashToken(raw) != hash {
		t.Fatal("token hash mismatch")
	}
	raw2, _, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw2 == raw {
		t.Fatal("session tokens repeat")
	}
}
