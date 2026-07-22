// Command cli-login-system is a containerized, interactive CLI that
// implements user registration, username/password login, optional
// TOTP-based two-factor authentication, account lockout, and session
// management, backed by a SQLite database.
package main

import (
	"fmt"
	"log"
	"os"

	"cli-login-system/internal/cli"
	"cli-login-system/internal/config"
	"cli-login-system/internal/db"
	"cli-login-system/internal/models"
	"cli-login-system/internal/service"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	userRepo := models.NewUserRepo(database)
	sessionRepo := models.NewSessionRepo(database)
	svc := service.New(userRepo, sessionRepo, cfg)

	app, err := cli.New(svc)
	if err != nil {
		log.Fatalf("failed to start CLI: %v", err)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
