package service_test

import (
	"os"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"cli-login-system/internal/config"
	"cli-login-system/internal/db"
	"cli-login-system/internal/models"
	"cli-login-system/internal/service"
)

// newTestService spins up a throwaway SQLite file for isolated testing and
// returns a ready-to-use Service plus a cleanup func.
func newTestService(t *testing.T, cfg config.Config) (*service.Service, func()) {
	t.Helper()

	f, err := os.CreateTemp("", "cli-login-test-*.db")
	if err != nil {
		t.Fatalf("creating temp db file: %v", err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // let db.Open create it fresh

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	users := models.NewUserRepo(database)
	sessions := models.NewSessionRepo(database)
	svc := service.New(users, sessions, cfg)

	cleanup := func() {
		database.Close()
		os.Remove(path)
	}
	return svc, cleanup
}

func testConfig() config.Config {
	return config.Config{
		SessionTimeout:    30 * time.Minute,
		MaxFailedAttempts: 3,
		LockoutDuration:   15 * time.Minute,
		TOTPIssuer:        "TestIssuer",
	}
}

func TestRegisterAndLogin(t *testing.T) {
	svc, cleanup := newTestService(t, testConfig())
	defer cleanup()

	if err := svc.Register("alice", "password123"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Duplicate registration must fail.
	if err := svc.Register("alice", "password123"); err == nil {
		t.Fatal("expected error registering duplicate username, got nil")
	}

	result, err := svc.Login("alice", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.TOTPRequired {
		t.Fatal("expected TOTPRequired = false for account without 2FA")
	}
	if result.Token == "" {
		t.Fatal("expected non-empty session token")
	}

	// Wrong password must fail with a generic error.
	if _, err := svc.Login("alice", "wrongpassword"); err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestLoginLockout(t *testing.T) {
	cfg := testConfig()
	cfg.MaxFailedAttempts = 3
	svc, cleanup := newTestService(t, cfg)
	defer cleanup()

	if err := svc.Register("bob", "password123"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Fail enough times to trip the lockout.
	for i := 0; i < cfg.MaxFailedAttempts; i++ {
		if _, err := svc.Login("bob", "wrongpassword"); err == nil {
			t.Fatalf("attempt %d: expected error for wrong password", i)
		}
	}

	// Even the CORRECT password should now be rejected because the
	// account is locked.
	if _, err := svc.Login("bob", "password123"); err == nil {
		t.Fatal("expected account-locked error, got nil")
	}
}

func TestWhoAmIAndLogout(t *testing.T) {
	svc, cleanup := newTestService(t, testConfig())
	defer cleanup()

	if err := svc.Register("carol", "password123"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	result, err := svc.Login("carol", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	view, err := svc.WhoAmI(result.Token)
	if err != nil {
		t.Fatalf("WhoAmI() error = %v", err)
	}
	if view.Username != "carol" {
		t.Fatalf("WhoAmI() username = %q, want carol", view.Username)
	}
	if view.TOTPEnabled {
		t.Fatal("expected TOTPEnabled = false for a fresh account")
	}

	if err := svc.Logout(result.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, err := svc.WhoAmI(result.Token); err != service.ErrNotAuthenticated {
		t.Fatalf("WhoAmI() after logout error = %v, want ErrNotAuthenticated", err)
	}
}

func TestEnableAndLoginWithTOTP(t *testing.T) {
	svc, cleanup := newTestService(t, testConfig())
	defer cleanup()

	if err := svc.Register("dave", "password123"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	result, err := svc.Login("dave", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	secret, url, err := svc.BeginEnableTOTP(result.Token)
	if err != nil {
		t.Fatalf("BeginEnableTOTP() error = %v", err)
	}
	if secret == "" || url == "" {
		t.Fatal("expected non-empty secret and otpauth URL")
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generating test TOTP code: %v", err)
	}

	if err := svc.ConfirmEnableTOTP(result.Token, code); err != nil {
		t.Fatalf("ConfirmEnableTOTP() error = %v", err)
	}

	// Plain login should now require a second factor.
	loginResult, err := svc.Login("dave", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !loginResult.TOTPRequired {
		t.Fatal("expected TOTPRequired = true after enabling 2FA")
	}

	code2, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generating second test TOTP code: %v", err)
	}
	final, err := svc.CompleteLoginWithTOTP(loginResult.UserID, code2)
	if err != nil {
		t.Fatalf("CompleteLoginWithTOTP() error = %v", err)
	}
	if final.Token == "" {
		t.Fatal("expected a session token after completing 2FA login")
	}

	// Wrong code must be rejected.
	if _, err := svc.CompleteLoginWithTOTP(loginResult.UserID, "000000"); err == nil {
		t.Fatal("expected error for invalid 2FA code, got nil")
	}

	// Disabling requires a valid current code.
	code3, _ := totp.GenerateCode(secret, time.Now())
	if err := svc.DisableTOTP(final.Token, code3); err != nil {
		t.Fatalf("DisableTOTP() error = %v", err)
	}

	view, err := svc.WhoAmI(final.Token)
	if err != nil {
		t.Fatalf("WhoAmI() error = %v", err)
	}
	if view.TOTPEnabled {
		t.Fatal("expected TOTPEnabled = false after disabling 2FA")
	}
}
