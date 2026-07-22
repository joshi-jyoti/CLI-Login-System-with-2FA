// Package service contains the application's business logic, independent
// of any presentation layer (CLI, web, etc). The CLI package calls into
// here; this package never touches os.Stdin/Stdout directly.
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cli-login-system/internal/auth"
	"cli-login-system/internal/config"
	"cli-login-system/internal/models"
)

// Sentinel errors returned to callers (the CLI) so it can present friendly,
// specific messages without string-matching.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountLocked      = errors.New("account is locked, try again later")
	ErrTOTPRequired       = errors.New("2FA code required")
	ErrInvalidTOTPCode    = errors.New("invalid 2FA code")
	ErrTOTPAlreadyEnabled = errors.New("2FA is already enabled")
	ErrTOTPNotEnabled     = errors.New("2FA is not enabled")
	ErrNotAuthenticated   = errors.New("not authenticated")
)

// Service wires together the repositories and config needed to implement
// registration, login, 2FA, and session management.
type Service struct {
	users    *models.UserRepo
	sessions *models.SessionRepo
	cfg      config.Config
}

// New constructs a Service.
func New(users *models.UserRepo, sessions *models.SessionRepo, cfg config.Config) *Service {
	return &Service{users: users, sessions: sessions, cfg: cfg}
}

// UserView is a read-only projection of a User safe to display to the
// person at the terminal (never includes password hash or TOTP secret).
type UserView struct {
	Username       string
	CreatedAt      time.Time
	TOTPEnabled    bool
	LastLogin      *time.Time
	SessionExpires time.Time
}

// Register creates a new user account with a bcrypt-hashed password.
func (s *Service) Register(username, password string) error {
	username = normalizeUsername(username)
	if username == "" {
		return errors.New("username cannot be empty")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err // e.g. auth.ErrPasswordTooShort
	}

	_, err = s.users.Create(username, hash)
	if errors.Is(err, models.ErrDuplicateUsername) {
		return models.ErrDuplicateUsername
	}
	return err
}

// LoginResult is returned by Login. If TOTPRequired is true, the caller
// must prompt for a 2FA code and call CompleteLoginWithTOTP.
type LoginResult struct {
	TOTPRequired bool
	UserID       int64
	Token        string
}

// Login validates username+password. If the account has 2FA enabled, it
// returns TOTPRequired=true without creating a session yet; the caller
// must then call CompleteLoginWithTOTP with the 6-digit code.
func (s *Service) Login(username, password string) (*LoginResult, error) {
	username = normalizeUsername(username)
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, models.ErrNotFound) {
		// Deliberately identical error to "wrong password" so we don't
		// leak which usernames exist.
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if locked, remaining := auth.IsLocked(u); locked {
		return nil, fmt.Errorf("%w (retry in %s)", ErrAccountLocked, remaining.Round(time.Second))
	}

	if !auth.CheckPassword(u.PasswordHash, password) {
		_ = s.users.RecordFailedAttempt(u.ID, s.cfg.MaxFailedAttempts, s.cfg.LockoutDuration)
		return nil, ErrInvalidCredentials
	}

	if u.TOTPEnabled {
		return &LoginResult{TOTPRequired: true, UserID: u.ID}, nil
	}

	return s.finishLogin(u.ID)
}

// CompleteLoginWithTOTP finishes a login for an account that has 2FA
// enabled, after Login returned TOTPRequired=true.
func (s *Service) CompleteLoginWithTOTP(userID int64, code string) (*LoginResult, error) {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return nil, err
	}

	if locked, remaining := auth.IsLocked(u); locked {
		return nil, fmt.Errorf("%w (retry in %s)", ErrAccountLocked, remaining.Round(time.Second))
	}

	if !u.TOTPSecret.Valid || !auth.ValidateTOTPCode(u.TOTPSecret.String, code) {
		_ = s.users.RecordFailedAttempt(u.ID, s.cfg.MaxFailedAttempts, s.cfg.LockoutDuration)
		return nil, ErrInvalidTOTPCode
	}

	return s.finishLogin(u.ID)
}

// finishLogin resets the failed-attempt counter, stamps last_login, and
// issues a fresh session token.
func (s *Service) finishLogin(userID int64) (*LoginResult, error) {
	if err := s.users.ResetFailedAttempts(userID); err != nil {
		return nil, err
	}
	sess, err := s.sessions.Create(userID, s.cfg.SessionTimeout)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Token: sess.Token, UserID: userID}, nil
}

// Logout invalidates a session token.
func (s *Service) Logout(token string) error {
	return s.sessions.Delete(token)
}

// WhoAmI resolves a session token to a displayable user profile. Returns
// ErrNotAuthenticated if the token is missing/expired.
func (s *Service) WhoAmI(token string) (*UserView, error) {
	sess, err := s.sessions.Get(token)
	if errors.Is(err, models.ErrNotFound) {
		return nil, ErrNotAuthenticated
	}
	if err != nil {
		return nil, err
	}

	u, err := s.users.GetByID(sess.UserID)
	if err != nil {
		return nil, err
	}

	view := &UserView{
		Username:       u.Username,
		CreatedAt:      u.CreatedAt,
		TOTPEnabled:    u.TOTPEnabled,
		SessionExpires: sess.ExpiresAt,
	}
	if u.LastLogin.Valid {
		t := u.LastLogin.Time
		view.LastLogin = &t
	}
	return view, nil
}

// BeginEnableTOTP generates a new (unconfirmed) TOTP secret for the user
// behind token and returns the secret plus otpauth:// enrolment URL, which
// the CLI should show as text (and optionally render as a QR code).
// The secret is not active until ConfirmEnableTOTP is called with a valid
// code generated from it.
func (s *Service) BeginEnableTOTP(token string) (secret, otpauthURL string, err error) {
	userID, err := s.requireUserID(token)
	if err != nil {
		return "", "", err
	}

	u, err := s.users.GetByID(userID)
	if err != nil {
		return "", "", err
	}
	if u.TOTPEnabled {
		return "", "", ErrTOTPAlreadyEnabled
	}

	secret, url, err := auth.GenerateTOTPSecret(s.cfg.TOTPIssuer, u.Username)
	if err != nil {
		return "", "", err
	}
	if err := s.users.SetTOTPSecret(userID, secret); err != nil {
		return "", "", err
	}
	return secret, url, nil
}

// ConfirmEnableTOTP verifies a code against the pending secret and, if
// correct, flips 2FA on.
func (s *Service) ConfirmEnableTOTP(token, code string) error {
	userID, err := s.requireUserID(token)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(userID)
	if err != nil {
		return err
	}
	if !u.TOTPSecret.Valid || !auth.ValidateTOTPCode(u.TOTPSecret.String, code) {
		return ErrInvalidTOTPCode
	}
	return s.users.EnableTOTP(userID)
}

// DisableTOTP turns off 2FA for the authenticated user (requires a valid
// current TOTP code as re-authentication, since this is a sensitive
// security-reducing action).
func (s *Service) DisableTOTP(token, code string) error {
	userID, err := s.requireUserID(token)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(userID)
	if err != nil {
		return err
	}
	if !u.TOTPEnabled {
		return ErrTOTPNotEnabled
	}
	if !auth.ValidateTOTPCode(u.TOTPSecret.String, code) {
		return ErrInvalidTOTPCode
	}
	return s.users.DisableTOTP(userID)
}

func (s *Service) requireUserID(token string) (int64, error) {
	sess, err := s.sessions.Get(token)
	if errors.Is(err, models.ErrNotFound) {
		return 0, ErrNotAuthenticated
	}
	if err != nil {
		return 0, err
	}
	return sess.UserID, nil
}

func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}
