-- 001_init_schema.sql
-- Initial schema for the CLI login system.
-- Applied automatically at startup by internal/db.Migrate().

CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    totp_secret     TEXT,               -- NULL until 2FA is enabled
    totp_enabled    INTEGER NOT NULL DEFAULT 0,  -- 0 = false, 1 = true
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until    DATETIME,           -- NULL when not locked
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login      DATETIME            -- NULL until first successful login
);

CREATE TABLE IF NOT EXISTS sessions (
    token       TEXT PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
