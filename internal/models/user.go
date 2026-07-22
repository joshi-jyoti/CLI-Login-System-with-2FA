// Package models defines domain types and the data-access layer (repository)
// for users and sessions. Keeping SQL confined to this package means the
// rest of the app (auth, cli) never writes raw queries.
package models

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when a lookup finds no matching row.
var ErrNotFound = errors.New("not found")

// ErrDuplicateUsername is returned when registering a username that
// already exists.
var ErrDuplicateUsername = errors.New("username already exists")

// User mirrors the `users` table.
type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	TOTPSecret     sql.NullString
	TOTPEnabled    bool
	FailedAttempts int
	LockedUntil    sql.NullTime
	CreatedAt      time.Time
	LastLogin      sql.NullTime
}

// UserRepo provides CRUD-ish access to the users table.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo constructs a UserRepo bound to the given database.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create inserts a new user with the given username and pre-hashed password.
func (r *UserRepo) Create(username, passwordHash string) (*User, error) {
	res, err := r.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username, passwordHash,
	)
	if err != nil {
		// SQLite reports uniqueness violations with this substring.
		if isUniqueViolation(err) {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// GetByUsername fetches a user by username.
func (r *UserRepo) GetByUsername(username string) (*User, error) {
	row := r.db.QueryRow(
		`SELECT id, username, password_hash, totp_secret, totp_enabled,
		        failed_attempts, locked_until, created_at, last_login
		 FROM users WHERE username = ?`, username,
	)
	return scanUser(row)
}

// GetByID fetches a user by primary key.
func (r *UserRepo) GetByID(id int64) (*User, error) {
	row := r.db.QueryRow(
		`SELECT id, username, password_hash, totp_secret, totp_enabled,
		        failed_attempts, locked_until, created_at, last_login
		 FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

// RecordFailedAttempt increments the failed-attempt counter and, if the
// threshold is reached, sets locked_until to now+lockoutDuration.
func (r *UserRepo) RecordFailedAttempt(userID int64, threshold int, lockoutDuration time.Duration) error {
	_, err := r.db.Exec(`
		UPDATE users
		SET failed_attempts = failed_attempts + 1,
		    locked_until = CASE
		        WHEN failed_attempts + 1 >= ? THEN datetime('now', ?)
		        ELSE locked_until
		    END
		WHERE id = ?`,
		threshold, lockoutOffset(lockoutDuration), userID,
	)
	return err
}

// ResetFailedAttempts clears the failed-attempt counter and any lock,
// and stamps last_login. Called after a fully successful login.
func (r *UserRepo) ResetFailedAttempts(userID int64) error {
	_, err := r.db.Exec(`
		UPDATE users
		SET failed_attempts = 0, locked_until = NULL, last_login = CURRENT_TIMESTAMP
		WHERE id = ?`, userID,
	)
	return err
}

// SetTOTPSecret stores a (not-yet-confirmed) TOTP secret for a user.
// It does NOT enable 2FA; call EnableTOTP after the user verifies a code.
func (r *UserRepo) SetTOTPSecret(userID int64, secret string) error {
	_, err := r.db.Exec(`UPDATE users SET totp_secret = ? WHERE id = ?`, secret, userID)
	return err
}

// EnableTOTP flips the totp_enabled flag on.
func (r *UserRepo) EnableTOTP(userID int64) error {
	_, err := r.db.Exec(`UPDATE users SET totp_enabled = 1 WHERE id = ?`, userID)
	return err
}

// DisableTOTP flips totp_enabled off and clears the stored secret.
func (r *UserRepo) DisableTOTP(userID int64) error {
	_, err := r.db.Exec(`UPDATE users SET totp_enabled = 0, totp_secret = NULL WHERE id = ?`, userID)
	return err
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &u.TOTPEnabled,
		&u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.LastLogin,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// lockoutOffset renders a Go duration as a SQLite `datetime()` modifier
// string, e.g. "+15 minutes".
func lockoutOffset(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return "+" + strconv.Itoa(minutes) + " minutes"
}

// isUniqueViolation detects SQLite's uniqueness-constraint error so callers
// can translate it into the friendlier ErrDuplicateUsername.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
