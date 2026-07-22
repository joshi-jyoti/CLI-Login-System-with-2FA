package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// Session mirrors the `sessions` table.
type Session struct {
	Token     string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionRepo provides access to the sessions table.
type SessionRepo struct {
	db *sql.DB
}

// NewSessionRepo constructs a SessionRepo bound to the given database.
func NewSessionRepo(db *sql.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

// Create generates a new cryptographically random session token for userID
// that expires after ttl, persists it, and returns the resulting Session.
func (r *SessionRepo) Create(userID int64, ttl time.Duration) (*Session, error) {
	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	_, err = r.db.Exec(
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	return &Session{Token: token, UserID: userID, CreatedAt: now, ExpiresAt: expiresAt}, nil
}

// Get fetches a session by token. Returns ErrNotFound if it doesn't exist
// OR has already expired (expired sessions are treated as gone).
func (r *SessionRepo) Get(token string) (*Session, error) {
	var s Session
	err := r.db.QueryRow(
		`SELECT token, user_id, created_at, expires_at FROM sessions WHERE token = ?`,
		token,
	).Scan(&s.Token, &s.UserID, &s.CreatedAt, &s.ExpiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(s.ExpiresAt) {
		_ = r.Delete(token)
		return nil, ErrNotFound
	}
	return &s, nil
}

// Delete removes a session (used for logout).
func (r *SessionRepo) Delete(token string) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// DeleteExpired purges all expired sessions. Called periodically to keep
// the table small; not strictly required for correctness since Get()
// already treats expired sessions as absent.
func (r *SessionRepo) DeleteExpired() error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC())
	return err
}

func generateToken(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
