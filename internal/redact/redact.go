// Package redact removes credentials from log messages, errors and job logs.
package redact

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
)

var (
	// api_key=..., access_token=..., token=..., password=..., secret=...
	paramRe = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|password|passwd|secret)=([^&\s"'#]+)`)
	// Authorization: Bearer ..., or just Bearer ...
	bearerRe = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]+`)
)

// String replaces credential values in s. It is safe to call on already-redacted
// strings.
func String(s string) string {
	s = paramRe.ReplaceAllString(s, `${1}=[redacted]`)
	s = bearerRe.ReplaceAllString(s, `${1}[redacted]`)
	return s
}

// Error wraps err so both Error() and any log of it are redacted. Unwrap keeps
// errors.Is/As working for callers.
func Error(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{err: err}
}

type redactedError struct{ err error }

func (e redactedError) Error() string { return String(e.err.Error()) }
func (e redactedError) Unwrap() error { return e.err }

// NewHandler wraps a slog handler so every message, attribute (including error
// values) and group is redacted before it reaches the output.
func NewHandler(next slog.Handler) slog.Handler { return &handler{next: next} }

type handler struct{ next slog.Handler }

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, String(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return &handler{next: h.next.WithAttrs(out)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{next: h.next.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(String(a.Value.String()))
	case slog.KindGroup:
		group := a.Value.Group()
		out := make([]slog.Attr, len(group))
		for i, sub := range group {
			out[i] = redactAttr(sub)
		}
		a.Value = slog.GroupValue(out...)
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			a.Value = slog.StringValue(String(v.Error()))
		case fmt.Stringer:
			a.Value = slog.StringValue(String(v.String()))
		}
	}
	return a
}
