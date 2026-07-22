# ---- Build stage ----
FROM golang:1.22-bookworm AS builder

WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is required by github.com/mattn/go-sqlite3.
ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o /out/cli-login-system .

# ---- Runtime stage ----
FROM debian:bookworm-slim

# ca-certificates: harmless/standard hygiene for a Go binary.
# sqlite3: not required by the app itself (the driver is statically
# compiled in), but handy if you want to `docker exec` in and inspect
# the database file with the sqlite3 CLI.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates sqlite3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/cli-login-system /app/cli-login-system

# Default database location; mount a volume here to persist data across
# container restarts (see docker-compose.yml).
ENV DB_PATH=/data/app.db
VOLUME ["/data"]

# The CLI is interactive, so this image is meant to be run with
# `docker compose run` / `docker run -it`, not as a detached daemon.
ENTRYPOINT ["/app/cli-login-system"]
