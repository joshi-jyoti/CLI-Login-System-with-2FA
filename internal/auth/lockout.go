package auth

import (
	"time"

	"cli-login-system/internal/models"
)

// IsLocked reports whether the user's account is currently under a
// failed-attempt lockout, and if so, how much time remains.
func IsLocked(u *models.User) (locked bool, remaining time.Duration) {
	if !u.LockedUntil.Valid {
		return false, 0
	}
	remaining = time.Until(u.LockedUntil.Time)
	if remaining <= 0 {
		return false, 0
	}
	return true, remaining
}
