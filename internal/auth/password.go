// Package auth implements password hashing and session tokens.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	pbkdf2Iterations = 210_000
	pbkdf2KeyLength  = 32
	saltLength       = 16
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$`)

// ValidateUsername accepts 3–32 letters/digits plus . _ -.
func ValidateUsername(username string) error {
	if !usernameRe.MatchString(strings.TrimSpace(username)) {
		return errors.New("username must be 3–32 characters (letters, digits, . _ -)")
	}
	return nil
}

// ValidatePassword enforces a minimum length. The maximum avoids pathological
// PBKDF2 inputs.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return errors.New("password must be at most 128 characters")
	}
	return nil
}

// HashPassword returns a self-describing PBKDF2-SHA256 hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLength)
	if err != nil {
		return "", fmt.Errorf("password hash: %w", err)
	}
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s",
		pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks password against an encoded hash in constant time.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(want, got) == 1
}

// NewSessionToken returns a random cookie token and its storage hash.
func NewSessionToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("session token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken hashes a cookie token before it is persisted.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
