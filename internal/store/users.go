package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Roles supported by the account system.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User is an account. PasswordHash is never serialized to the API.
type User struct {
	ID                int64     `json:"id"`
	Username          string    `json:"username"`
	DisplayName       string    `json:"displayName"`
	Role              string    `json:"role"`
	Active            bool      `json:"active"`
	RatingCeiling     string    `json:"ratingCeiling,omitempty"`
	LibraryRestricted bool      `json:"-"`
	PasswordHash      string    `json:"-"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

const userColumns = `
	id, username, display_name, role, active, COALESCE(rating_ceiling,''), library_restricted,
	COALESCE(password_hash,''), created_at, updated_at`

const userColumnsQualified = `
	u.id, u.username, u.display_name, u.role, u.active, COALESCE(u.rating_ceiling,''), u.library_restricted,
	COALESCE(u.password_hash,''), u.created_at, u.updated_at`

func scanUser(row rowScanner, u *User) error {
	var active, restricted int
	var created, updated string
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &active, &u.RatingCeiling, &restricted, &u.PasswordHash, &created, &updated); err != nil {
		return err
	}
	u.Active = active != 0
	u.LibraryRestricted = restricted != 0
	u.CreatedAt, u.UpdatedAt = parseTime(created), parseTime(updated)
	return nil
}

// NeedsSetup reports whether no account has a password yet.
func (s *Store) NeedsSetup(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE password_hash <> ''`).Scan(&n); err != nil {
		return false, wrap("needs setup", err)
	}
	return n == 0, nil
}

// GetUserByID loads one user.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id), &u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, wrap("get user", err)
	}
	return &u, nil
}

// GetUserByUsername loads a user by case-insensitive username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = ? COLLATE NOCASE`, strings.TrimSpace(username)), &u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, wrap("get user", err)
	}
	return &u, nil
}

// ListUsers returns all accounts.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, wrap("list users", err)
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := scanUser(rows, &u); err != nil {
			return nil, wrap("scan user", err)
		}
		out = append(out, u)
	}
	return out, wrap("iterate users", rows.Err())
}

// SetupAdmin sets the password of the built-in admin account on first run.
func (s *Store) SetupAdmin(ctx context.Context, username, displayName, passwordHash string) (*User, error) {
	needs, err := s.NeedsSetup(ctx)
	if err != nil {
		return nil, err
	}
	if !needs {
		return nil, errInvalid("the admin account is already set up")
	}
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" {
		return nil, errInvalid("username is required")
	}
	if displayName == "" {
		displayName = username
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("begin setup", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE users SET username = ?, display_name = ?, password_hash = ?, role = ?, active = 1, updated_at = ?
		WHERE id = ?`, username, displayName, passwordHash, RoleAdmin, ts, DefaultUserID)
	if err != nil {
		return nil, wrap("setup admin", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, username, display_name, password_hash, role, active, created_at, updated_at)
			VALUES (?,?,?,?,?,1,?,?)`,
			DefaultUserID, username, displayName, passwordHash, RoleAdmin, ts, ts); err != nil {
			return nil, wrap("create admin", err)
		}
	}
	if err := ensureSystemCollections(ctx, tx, DefaultUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap("commit setup", err)
	}
	return s.GetUserByID(ctx, DefaultUserID)
}

// CreateUser adds an account and its Favorites/Watchlist collections.
func (s *Store) CreateUser(ctx context.Context, username, displayName, passwordHash, role string) (*User, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" {
		return nil, errInvalid("username is required")
	}
	if role != RoleAdmin && role != RoleUser {
		return nil, errInvalid("role must be admin or user")
	}
	if displayName == "" {
		displayName = username
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("begin create user", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO users (username, display_name, password_hash, role, active, created_at, updated_at)
		VALUES (?,?,?,?,1,?,?)`, username, displayName, passwordHash, role, ts, ts)
	if err != nil {
		if sqliteErrIsUnique(err) {
			return nil, errInvalid("that username is already taken")
		}
		return nil, wrap("create user", err)
	}
	id, _ := res.LastInsertId()
	if err := ensureSystemCollections(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap("commit create user", err)
	}
	return s.GetUserByID(ctx, id)
}

// UpdateUser changes an account. Nil fields are left untouched.
func (s *Store) UpdateUser(ctx context.Context, id int64, displayName, role *string, active *bool, passwordHash, ratingCeiling *string) (*User, error) {
	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	newDisplay := u.DisplayName
	if displayName != nil {
		newDisplay = strings.TrimSpace(*displayName)
		if newDisplay == "" {
			newDisplay = u.Username
		}
	}
	newRole := u.Role
	if role != nil {
		if *role != RoleAdmin && *role != RoleUser {
			return nil, errInvalid("role must be admin or user")
		}
		newRole = *role
	}
	newActive := u.Active
	if active != nil {
		newActive = *active
	}
	newRating := u.RatingCeiling
	if ratingCeiling != nil {
		newRating = strings.TrimSpace(*ratingCeiling)
	}
	if u.Active && u.Role == RoleAdmin && (!newActive || newRole != RoleAdmin) {
		n, err := s.CountActiveAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, errInvalid("cannot remove the last active admin")
		}
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("begin update user", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET display_name = ?, role = ?, active = ?, rating_ceiling = ?, updated_at = ? WHERE id = ?`,
		newDisplay, newRole, boolInt(newActive), newRating, ts, id); err != nil {
		return nil, wrap("update user", err)
	}
	if passwordHash != nil && *passwordHash != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, *passwordHash, ts, id); err != nil {
			return nil, wrap("update password", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return nil, wrap("drop user sessions", err)
		}
	}
	if !newActive {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return nil, wrap("drop user sessions", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap("commit update user", err)
	}
	return s.GetUserByID(ctx, id)
}

// DeleteUser removes an account and its sessions/collections/progress.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if u.Active && u.Role == RoleAdmin {
		n, err := s.CountActiveAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return errInvalid("cannot delete the last active admin")
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
		return wrap("delete user", err)
	}
	return nil
}

// CountActiveAdmins counts enabled admin accounts.
func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = ? AND active = 1`, RoleAdmin).Scan(&n); err != nil {
		return 0, wrap("count admins", err)
	}
	return n, nil
}

// CreateSession stores a hashed session token.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash, userAgent, ip string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, user_agent, ip, created_at, expires_at)
		VALUES (?,?,?,?,?,?)`, tokenHash, userID, userAgent, ip, now(), expires.UTC().Format(time.RFC3339))
	return wrap("create session", err)
}

// SessionUser resolves a session token to an active user.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	var u User
	err := scanUser(s.db.QueryRowContext(ctx, `
		SELECT `+userColumnsQualified+` FROM users u
		JOIN sessions s ON s.user_id = u.id
		WHERE s.token_hash = ? AND s.expires_at > ? AND u.active = 1`,
		tokenHash, now()), &u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, wrap("session user", err)
	}
	return &u, nil
}

// DeleteSession removes one session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return wrap("delete session", err)
}

// DeleteUserSessions invalidates all sessions for a user.
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return wrap("delete user sessions", err)
}

// CleanupSessions removes expired sessions.
func (s *Store) CleanupSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
	return wrap("cleanup sessions", err)
}

func ensureSystemCollections(ctx context.Context, tx *sql.Tx, userID int64) error {
	ts := now()
	for key, name := range map[string]string{SystemFavorites: "Favorites", SystemWatchlist: "Watchlist"} {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO collections (user_id, name, system_key, created_at, updated_at)
			VALUES (?,?,?,?,?)`, userID, name, key, ts, ts); err != nil {
			return wrap("ensure system collections", err)
		}
	}
	return nil
}

// UpdatePassword changes the password hash without dropping the current session.
func (s *Store) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, passwordHash, now(), id)
	return wrap("update password", err)
}

// DeleteOtherSessions signs out every session for a user except keepHash.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID int64, keepHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepHash)
	return wrap("delete other sessions", err)
}

// LibraryAccess is one library with a user's view permission.
type LibraryAccess struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Allowed bool   `json:"allowed"`
}

// UserLibraryAccess lists every library with the user's permission. No rows
// means "all libraries" for backwards compatibility.
func (s *Store) UserLibraryAccess(ctx context.Context, userID int64) ([]LibraryAccess, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM libraries ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap("library access", err)
	}
	var out []LibraryAccess
	for rows.Next() {
		var a LibraryAccess
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			rows.Close()
			return nil, wrap("scan library access", err)
		}
		a.Allowed = true
		out = append(out, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, wrap("iterate library access", err)
	}
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.LibraryRestricted {
		perms, err := s.db.QueryContext(ctx, `SELECT library_id, can_view FROM library_permissions WHERE user_id = ?`, userID)
		if err != nil {
			return nil, wrap("user library permissions", err)
		}
		defer perms.Close()
		allowed := map[int64]bool{}
		for perms.Next() {
			var id int64
			var can int
			if err := perms.Scan(&id, &can); err != nil {
				return nil, wrap("scan library permission", err)
			}
			allowed[id] = can != 0
		}
		for i := range out {
			out[i].Allowed = allowed[out[i].ID]
		}
	}
	if out == nil {
		out = []LibraryAccess{}
	}
	return out, nil
}

// SetUserLibraryPermissions replaces a user's library allow/deny matrix.
func (s *Store) SetUserLibraryPermissions(ctx context.Context, userID int64, allowedIDs []int64) error {
	allowed := map[int64]bool{}
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM libraries ORDER BY id`)
	if err != nil {
		return wrap("list library ids", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return wrap("scan library id", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return wrap("iterate library ids", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin library permissions", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_permissions WHERE user_id = ?`, userID); err != nil {
		return wrap("clear library permissions", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET library_restricted = 1, updated_at = ? WHERE id = ?`, now(), userID); err != nil {
		return wrap("restrict user libraries", err)
	}
	for _, id := range ids {
		if !allowed[id] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO library_permissions (user_id, library_id, can_view) VALUES (?,?,1)`, userID, id); err != nil {
			return wrap("set library permission", err)
		}
	}
	return wrap("commit library permissions", tx.Commit())
}

// SetUserLibraryUnrestricted removes an explicit library allow-list.
func (s *Store) SetUserLibraryUnrestricted(ctx context.Context, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("begin unrestricted libraries", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_permissions WHERE user_id = ?`, userID); err != nil {
		return wrap("clear library permissions", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET library_restricted = 0, updated_at = ? WHERE id = ?`, now(), userID); err != nil {
		return wrap("unrestrict libraries", err)
	}
	return wrap("commit unrestricted libraries", tx.Commit())
}
