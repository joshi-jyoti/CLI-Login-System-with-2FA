// Package config centralizes all environment-driven configuration for the
// application. Every value has a sane default so the app can run with zero
// configuration, but everything can be overridden via environment variables
// (which is how docker-compose.yml wires things up).
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime-tunable settings.
type Config struct {
	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// SessionTimeout controls how long a session stays valid after login
	// without activity-based renewal.
	SessionTimeout time.Duration

	// MaxFailedAttempts is the number of consecutive failed login attempts
	// allowed before an account is locked out.
	MaxFailedAttempts int

	// LockoutDuration is how long an account stays locked after exceeding
	// MaxFailedAttempts.
	LockoutDuration time.Duration

	// TOTPIssuer is the "issuer" name shown in authenticator apps
	// (e.g. Google Authenticator) when a user scans the QR/enrolment URI.
	TOTPIssuer string
}

// Load reads configuration from environment variables, falling back to
// defaults for anything not set.
func Load() Config {
	return Config{
		DBPath:            getEnv("DB_PATH", "/data/app.db"),
		SessionTimeout:    getEnvDuration("SESSION_TIMEOUT_MINUTES", 30) * time.Minute,
		MaxFailedAttempts: getEnvInt("MAX_FAILED_ATTEMPTS", 5),
		LockoutDuration:   getEnvDuration("LOCKOUT_DURATION_MINUTES", 15) * time.Minute,
		TOTPIssuer:        getEnv("TOTP_ISSUER", "CLI-Login-System"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallbackMinutes int) time.Duration {
	return time.Duration(getEnvInt(key, fallbackMinutes))
}
