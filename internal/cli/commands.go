package cli

import (
	"errors"
	"fmt"
	"os"

	"cli-login-system/internal/auth"
	"cli-login-system/internal/service"

	"github.com/mdp/qrterminal/v3"
)

// cmdRegister walks the user through creating a new account.
func (a *App) cmdRegister() {
	username, err := a.readLine("New username: ")
	if err != nil || username == "" {
		fmt.Println("Registration cancelled.")
		return
	}

	password, err := a.readSecret(fmt.Sprintf("Password (min %d chars): ", auth.MinPasswordLength))
	if err != nil {
		fmt.Println("Registration cancelled.")
		return
	}
	confirm, err := a.readSecret("Confirm password: ")
	if err != nil {
		fmt.Println("Registration cancelled.")
		return
	}
	if password != confirm {
		fmt.Println("✗ Passwords do not match.")
		return
	}

	if err := a.svc.Register(username, password); err != nil {
		switch {
		case errors.Is(err, auth.ErrPasswordTooShort):
			fmt.Printf("✗ %s\n", err)
		default:
			fmt.Printf("✗ Registration failed: %s\n", friendlyErr(err))
		}
		return
	}

	fmt.Printf("✓ Account %q created successfully. You can now log in.\n", username)
}

// cmdLogin walks the user through username/password (and, if enabled,
// TOTP) authentication.
func (a *App) cmdLogin() {
	username, err := a.readLine("Username: ")
	if err != nil || username == "" {
		fmt.Println("Login cancelled.")
		return
	}
	password, err := a.readSecret("Password: ")
	if err != nil {
		fmt.Println("Login cancelled.")
		return
	}

	result, err := a.svc.Login(username, password)
	if err != nil {
		fmt.Printf("✗ Login failed: %s\n", friendlyErr(err))
		return
	}

	if result.TOTPRequired {
		code, err := a.readLine("2FA code: ")
		if err != nil || code == "" {
			fmt.Println("Login cancelled.")
			return
		}
		result, err = a.svc.CompleteLoginWithTOTP(result.UserID, code)
		if err != nil {
			fmt.Printf("✗ Login failed: %s\n", friendlyErr(err))
			return
		}
	}

	a.sess = session{loggedIn: true, token: result.Token, username: username}
	a.refreshCompleter()
	fmt.Printf("✓ Welcome back, %s!\n\n", username)
	a.cmdWhoAmI()
}

// cmdWhoAmI prints the current user's profile, as required to
// auto-display after login and on demand via the `whoami` command.
func (a *App) cmdWhoAmI() {
	view, err := a.svc.WhoAmI(a.sess.token)
	if err != nil {
		fmt.Printf("✗ %s\n", friendlyErr(err))
		a.clearSessionIfExpired(err)
		return
	}

	mfaStatus := "disabled"
	if view.TOTPEnabled {
		mfaStatus = "enabled"
	}
	lastLogin := "never (this is your first login)"
	if view.LastLogin != nil {
		lastLogin = view.LastLogin.Local().Format("2006-01-02 15:04:05 MST")
	}

	fmt.Println("--- Account details ---")
	fmt.Printf("Username:           %s\n", view.Username)
	fmt.Printf("Registered on:      %s\n", view.CreatedAt.Local().Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("MFA status:         %s\n", mfaStatus)
	fmt.Printf("Session expires:    %s\n", view.SessionExpires.Local().Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Last login:         %s\n", lastLogin)
	fmt.Println("------------------------")
}

// cmdEnable2FA generates and displays a TOTP secret / QR-style URI, then
// asks the user to confirm by entering a code from their authenticator
// app before actually turning 2FA on.
func (a *App) cmdEnable2FA() {
	secret, otpauthURL, err := a.svc.BeginEnableTOTP(a.sess.token)
	if err != nil {
		fmt.Printf("✗ %s\n", friendlyErr(err))
		a.clearSessionIfExpired(err)
		return
	}

	fmt.Println("Scan this into Google Authenticator (or any TOTP app),")
	fmt.Println("or enter the secret manually:")
	fmt.Println()
	fmt.Printf("  Secret: %s\n", secret)
	fmt.Printf("  URI:    %s\n", otpauthURL)
	fmt.Println()

	qrterminal.GenerateHalfBlock(otpauthURL, qrterminal.L, os.Stdout)
	fmt.Println()

	code, err := a.readLine("Enter the 6-digit code from your app to confirm: ")
	if err != nil || code == "" {
		fmt.Println("2FA setup cancelled. Run enable-2fa again to retry.")
		return
	}

	if err := a.svc.ConfirmEnableTOTP(a.sess.token, code); err != nil {
		fmt.Printf("✗ %s\n", friendlyErr(err))
		return
	}
	fmt.Println("✓ Two-factor authentication is now enabled on your account.")
}

// cmdDisable2FA turns off 2FA, requiring a valid current code as
// re-authentication since this is a security-reducing action.
func (a *App) cmdDisable2FA() {
	code, err := a.readLine("Enter your current 6-digit 2FA code to confirm disabling: ")
	if err != nil || code == "" {
		fmt.Println("Cancelled.")
		return
	}
	if err := a.svc.DisableTOTP(a.sess.token, code); err != nil {
		fmt.Printf("✗ %s\n", friendlyErr(err))
		return
	}
	fmt.Println("✓ Two-factor authentication has been disabled.")
}

// cmdLogout ends the current session both client-side and server-side.
func (a *App) cmdLogout() {
	if err := a.svc.Logout(a.sess.token); err != nil {
		fmt.Printf("(warning: %s)\n", err)
	}
	a.sess = session{}
	a.refreshCompleter()
	fmt.Println("✓ Logged out.")
}

// friendlyErr maps known service-layer sentinel errors to clear,
// non-leaky user-facing messages.
func friendlyErr(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		return "invalid username or password."
	case errors.Is(err, service.ErrAccountLocked):
		return err.Error() + "."
	case errors.Is(err, service.ErrInvalidTOTPCode):
		return "invalid 2FA code."
	case errors.Is(err, service.ErrTOTPAlreadyEnabled):
		return "2FA is already enabled on this account."
	case errors.Is(err, service.ErrTOTPNotEnabled):
		return "2FA is not currently enabled."
	case errors.Is(err, service.ErrNotAuthenticated):
		return "your session has expired. Please log in again."
	default:
		return err.Error()
	}
}

// clearSessionIfExpired resets local client state when the server reports
// that our session token is no longer valid (e.g. it timed out), so the
// prompt and tab-completion correctly fall back to the logged-out state.
func (a *App) clearSessionIfExpired(err error) {
	if errors.Is(err, service.ErrNotAuthenticated) {
		a.sess = session{}
		a.refreshCompleter()
	}
}
