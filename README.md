# Containerized CLI Login System with Optional 2FA

A secure, interactive command-line login system written in Go. It supports user registration, username/password authentication, optional TOTP-based two-factor authentication (compatible with Google Authenticator, Authy, etc.), account lockout after repeated failed attempts, and configurable session timeouts — all backed by a SQLite database that persists across Docker container restarts.

This project fulfills all the objectives for the "Containerized CLI Login System" assignment.

## Features

1. **Authentication System**
   - User registration and login.
   - **Optional TOTP 2FA**: Generates a QR code directly in the terminal for easy scanning with any standard authenticator app.
   - **Secure password storage**: Hashed securely using `bcrypt`.
   - **Account lockout**: Locks out accounts after a configurable number of failed attempts (default: 5) for a set duration (default: 15 minutes).
   - **Session management**: Uses server-side sessions with configurable timeouts (default: 30 minutes) stored in the database.

2. **Database Integration**
   - Uses **SQLite** for simplicity and portability.
   - The database runs seamlessly within the container and persists data across restarts via a named Docker volume (`cli-login-data`).

3. **Command-Line Interface**
   - Interactive REPL prompt with command history and tab-completion.
   - Context-aware `help` command.
   - Clear and friendly success/error messages.
   - Secure input for passwords (input is not echoed to the terminal).

## Setup Instructions

### Prerequisites
- **Docker & Docker Compose** (Highly Recommended)
- Alternatively, Go 1.22+ and a C compiler (gcc) for running natively.

### Running with Docker (Recommended)

1. **Build the container image**:
   ```bash
   docker compose build
   ```

2. **Run the interactive CLI**:
   ```bash
   docker compose run --rm cli-login-system
   ```
   *Note: Because this is an interactive terminal program, you must use `docker compose run` rather than `up -d` so that your terminal stays attached to the container's standard input.*

### Inspecting the Database
The database persists in a Docker volume named `cli-login-data`. If you'd like to inspect the database, you can start a temporary SQLite session inside the container environment:
```bash
docker compose run --rm --entrypoint sqlite3 cli-login-system /data/app.db
```

To wipe all data and start fresh, remove the volume:
```bash
docker compose down -v
```

## Usage Guide

On startup, you will be greeted by the CLI prompt. Available commands depend on whether you are logged in.

### Commands Before Login
| Command    | Description                                    |
|------------|-------------------------------------------------|
| `register` | Create a new user account (prompts for username and password) |
| `login`    | Log in with username and password (plus TOTP if enabled) |
| `help`     | Show available commands                         |
| `exit`     | Quit the program                                |

### Commands After Login
| Command       | Description                                            |
|---------------|---------------------------------------------------------|
| `whoami`      | Show current user details (auto-displayed after login)  |
| `enable-2fa`  | Enable TOTP-based MFA. Displays a QR code and requires confirmation. |
| `disable-2fa` | Disable MFA (requires a valid current 2FA code to confirm) |
| `logout`      | End your session                                        |
| `help`        | Show available commands                                 |
| `exit`        | Quit the program                                        |

### User Details
Upon successful login (and via the `whoami` command), the system automatically displays the following user details:
- Username
- Registration date
- MFA status (enabled/disabled)
- Session expiration time
- Last login time

## Architecture & Code Structure

```
.
├── main.go                     # Entrypoint: wires config, db, repos, service, CLI
├── internal/
│   ├── config/                 # Env-var configuration with defaults
│   ├── db/                     # SQLite connection + embedded schema migration
│   ├── models/                 # User & Session structs + SQL layer
│   ├── auth/                   # bcrypt hashing, TOTP generation, lockout policy
│   ├── service/                # Business logic (UI-agnostic)
│   └── cli/                    # Interactive REPL: prompt, tab-completion, formatting
├── migrations/                 # Schema definitions (automatically applied on startup)
├── Dockerfile                  # Container build instructions
├── docker-compose.yml          # Compose file configuring volumes and env variables
└── go.mod / go.sum             # Go module dependencies
```

## Security Best Practices Implemented

- **Password Hashing**: Passwords are never stored in plaintext. Hashed with bcrypt.
- **Session Tokens**: Uses cryptographically secure random session tokens (256-bit).
- **Generic Error Messages**: "Invalid username or password" is used to prevent user enumeration attacks.
- **MFA Security**: Disabling 2FA requires verifying a valid code, protecting the account even if a session is left unlocked.

## Running Unit Tests

Unit tests cover registration, login, lockout, and the full 2FA flow. To run them (requires local Go installation):
```bash
CGO_ENABLED=1 go test ./...
```
