package store

import (
	"context"
	"fmt"
	"strings"
)

type userRoleKey struct{}
type userRatingKey struct{}
type userLibraryRestrictedKey struct{}

// WithUserRole stores the authenticated role in a context.
func WithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, userRoleKey{}, role)
}

// WithUserLibraryRestricted stores whether the user has an explicit library allow-list.
func WithUserLibraryRestricted(ctx context.Context, restricted bool) context.Context {
	return context.WithValue(ctx, userLibraryRestrictedKey{}, restricted)
}

// WithUserRating stores the user's content-rating ceiling in a context.
func WithUserRating(ctx context.Context, ceiling string) context.Context {
	return context.WithValue(ctx, userRatingKey{}, strings.TrimSpace(ceiling))
}

// IsAdmin reports whether the context is allowed to bypass library/rating
// restrictions. Background jobs and tests have no role and are treated as admin.
func IsAdmin(ctx context.Context) bool {
	role, ok := ctx.Value(userRoleKey{}).(string)
	if !ok || role == "" {
		return true
	}
	return role == RoleAdmin
}

// UserRating returns the context user's content-rating ceiling.
func UserRating(ctx context.Context) string {
	v, _ := ctx.Value(userRatingKey{}).(string)
	return v
}

// accessScope returns an extra SQL predicate and args for a media_items alias.
// Admins, background jobs and unrestricted users get no predicate.
func accessScope(ctx context.Context, alias string) (string, []any) {
	if IsAdmin(ctx) {
		return "", nil
	}
	var clauses []string
	var args []any
	if restricted, _ := ctx.Value(userLibraryRestrictedKey{}).(bool); restricted {
		clauses = append(clauses, fmt.Sprintf(`EXISTS (SELECT 1 FROM library_permissions lp
			WHERE lp.user_id = ? AND lp.library_id = %s.library_id AND lp.can_view = 1)`, alias))
		args = append(args, UserID(ctx))
	}
	if ceiling := UserRating(ctx); ceiling != "" {
		rank := ratingRank(ceiling)
		if rank > 0 && rank < 99 {
			clauses = append(clauses, fmt.Sprintf(`(
				COALESCE(%s.content_rating,'') = '' OR
				CASE UPPER(TRIM(%s.content_rating))
					WHEN 'G' THEN 1 WHEN 'TV-Y' THEN 1 WHEN 'TV-Y7' THEN 1 WHEN 'TV-G' THEN 1
					WHEN 'PG' THEN 2 WHEN 'TV-PG' THEN 2
					WHEN 'PG-13' THEN 3 WHEN 'TV-14' THEN 3
					WHEN 'R' THEN 4 WHEN 'TV-MA' THEN 4
					WHEN 'NC-17' THEN 5
					ELSE 9
				END <= ?
			)`, alias, alias))
			args = append(args, rank)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// ratingRank maps common MPAA/TV ratings to an increasing order.
func ratingRank(rating string) int {
	switch strings.ToUpper(strings.TrimSpace(rating)) {
	case "G", "TV-Y", "TV-Y7", "TV-G":
		return 1
	case "PG", "TV-PG":
		return 2
	case "PG-13", "TV-14":
		return 3
	case "R", "TV-MA":
		return 4
	case "NC-17":
		return 5
	case "":
		return 99
	default:
		return 9
	}
}

// libraryScope returns an extra SQL predicate for a libraries alias.
func libraryScope(ctx context.Context, alias string) (string, []any) {
	if IsAdmin(ctx) {
		return "", nil
	}
	if restricted, _ := ctx.Value(userLibraryRestrictedKey{}).(bool); restricted {
		return fmt.Sprintf(` AND EXISTS (SELECT 1 FROM library_permissions lp
			WHERE lp.user_id = ? AND lp.library_id = %s.id AND lp.can_view = 1)`, alias), []any{UserID(ctx)}
	}
	return "", nil
}
