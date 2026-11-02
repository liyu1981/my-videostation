package store

import "context"

type userIDKey struct{}

// WithUserID stores the authenticated user in a context. Background jobs and
// tests without a user fall back to DefaultUserID.
func WithUserID(ctx context.Context, id int64) context.Context {
	if id <= 0 {
		return ctx
	}
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the authenticated user for per-user state (progress, collections).
func UserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(userIDKey{}).(int64); ok && id > 0 {
		return id
	}
	return DefaultUserID
}
