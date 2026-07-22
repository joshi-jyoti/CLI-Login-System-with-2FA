// Package db manages the SQLite connection and schema migrations.
//
// We use modernc.org/sqlite, a pure-Go SQLite driver, specifically so the
// application has zero CGO dependency. That makes cross-compiling and
// building small Docker images painless.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/001_init_schema.sql
var initSchema string

// Open opens (and creates, if necessary) the SQLite database at path and
// applies all migrations. The returned *sql.DB is safe for concurrent use.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory: %w", err)
		}
	}

	// _foreign_keys/_journal_mode are mattn/go-sqlite3 DSN options: enable
	// foreign key enforcement and use WAL for better concurrent access.
	dsn := fmt.Sprintf("file:%s?_foreign_keys=1&_journal_mode=WAL", path)

	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite only really supports a single writer at a time; capping the
	// pool avoids "database is locked" errors under concurrent access.
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := migrate(database); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return database, nil
}

// migrate applies the embedded schema. CREATE TABLE/INDEX IF NOT EXISTS
// statements make this idempotent, so it is safe to run on every startup.
func migrate(database *sql.DB) error {
	_, err := database.Exec(initSchema)
	return err
}
