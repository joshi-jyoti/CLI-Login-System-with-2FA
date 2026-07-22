package auth

import (
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// timeNow is indirected through a var so tests can freeze time if needed.
var timeNow = time.Now

// GenerateTOTPSecret creates a brand-new TOTP secret for username, scoped
// under issuer, and returns both the secret (to store) and the otpauth://
// provisioning URI (to render as a QR code or show as plain text so the
// user can add it to Google Authenticator / Authy / etc.).
func GenerateTOTPSecret(issuer, username string) (secret string, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTPCode reports whether code is a currently-valid TOTP code for
// the given secret, allowing a small clock-skew window.
func ValidateTOTPCode(secret, code string) bool {
	valid, _ := totp.ValidateCustom(code, secret, timeNow(), totp.ValidateOpts{
		Period:    30,
		Skew:      1, // allow ±1 time-step (30s) of clock drift
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}
