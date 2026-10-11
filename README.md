# home-ctl

🏠 *A self-hosted, multi-user Go Telegram bot for household management (Shopping, Chores, Reminders) backed by SQLite as the authoritative source of truth.*

---

## Overview

`home-ctl` manages daily household operations through Telegram:
- **Shopping**: Shared shopping list with an item catalog and tap-to-purchase buttons in the Shopping topic.
- **Chores**: Assigned household chores with due dates, recurrence (scheduled or completion anchors), early completions, and postponements.
- **Reminders**: One-off and recurring personal reminders with configurable repeated notifications until acknowledged.
- **Forum Topics & Private Chats**: Integrates with a single Telegram supergroup forum with configured Topics (`Shopping`, `Chores`, `Reminders`) and private direct messages for personal lists and notifications.
- **Authoritative SQLite**: SQLite is the single source of truth using pure-Go driver (`modernc.org/sqlite`, CGO-free), WAL mode, and strict versioned migrations.
- **Zero Host Tooling Needed**: All build, test, and lint commands run inside ephemeral Docker containers.

---

## Build, Run, and Verify

### 1. Verification Gate
To run formatting checks, `go vet`, `golangci-lint`, race-detector unit tests, and production code coverage:
```bash
./scripts/verify.sh
```

### 2. Build Binary via Docker
To compile the Go binary inside a clean container:
```bash
docker run --rm -v "$PWD":/app -w /app golang:1.27.2 go build ./...
```

### 3. Build Container Image
To build the minimal runtime container:
```bash
docker build -t homectl:latest -f build/package/Dockerfile .
```

### 4. Running
To run with Docker Compose, create the (unversioned) env and config files first:
```bash
cp deployments/.env.example deployments/.env
for f in app telegram users; do cp config/$f.example.yaml config/$f.yaml; done
# edit them, then:
docker compose -f deployments/docker-compose.yml up -d
```
Or run directly:
```bash
docker run --rm \
  -e TELEGRAM_BOT_TOKEN="your_bot_token" \
  -e APP_TIMEZONE="America/Guayaquil" \
  -v "$PWD/config":/app/config \
  -v "$PWD/locales":/app/locales \
  -v "$PWD/data":/app/data \
  homectl:latest
```

---

## Project Structure

- `cmd/homectl/`: Application entrypoint and dependency wiring.
- `internal/`: Core domain logic, adapters, SQLite persistence, and services.
- `config/`: Configuration files (`users.yaml`, `telegram.yaml`, `app.yaml`).
- `locales/`: Localization bundles (`en.yaml`, `es.yaml`).
- `build/package/`: Multi-stage Dockerfile.
- `deployments/`: Docker Compose definitions.
- `docs/adr/`: Architecture Decision Records.
- `scripts/`: Validation and verification scripts.
