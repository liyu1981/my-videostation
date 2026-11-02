package redact

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			`Get "https://api.themoviedb.org/3/search/movie?api_key=abc123&language=en-US": context deadline exceeded`,
			`Get "https://api.themoviedb.org/3/search/movie?api_key=[redacted]&language=en-US": context deadline exceeded`,
		},
		{`https://example.test/?access_token=xyz&x=1`, `https://example.test/?access_token=[redacted]&x=1`},
		{`Authorization: Bearer secret-token`, `Authorization: Bearer [redacted]`},
		{`plain text`, `plain text`},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestErrorUnwrap(t *testing.T) {
	base := errors.New("request failed: api_key=abc")
	err := Error(base)
	if strings.Contains(err.Error(), "abc") {
		t.Fatalf("unredacted error: %v", err)
	}
	if !errors.Is(err, base) {
		t.Fatal("Error should preserve Unwrap")
	}
}

func TestHandlerRedactsAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))
	log.Error("request failed api_key=message-secret", "err", errors.New("url?api_key=attr-secret"), "url", "https://x.test/?api_key=string-secret")
	out := buf.String()
	for _, secret := range []string{"message-secret", "attr-secret", "string-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log contains %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("log was not redacted: %s", out)
	}
}
