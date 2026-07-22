// Package auth contains security-sensitive helpers: password hashing,
// TOTP (2FA) enrolment/verification, and account-lockout policy.
package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooShort is returned when a candidate password fails our
// minimum length policy.
var ErrPasswordTooShort = errors.New("password must be at least 8 characters")

// MinPasswordLength is the minimum accepted password length.
const MinPasswordLength = 8

// HashPassword hashes a plaintext password with bcrypt using a safe
// default cost factor.
func HashPassword(plaintext string) (string, error) {
	if len(plaintext) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether plaintext matches the given bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
