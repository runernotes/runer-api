# Runer API

A RESTful notes API built in Go. This is the core library — it implements authentication, note sync, and the tombstone-based delete pattern. It is designed to be embedded or extended by other binaries (e.g. a hosted version that adds subscription and billing logic).

---

## Stack

| Layer        | Library                  |
|--------------|--------------------------|
| HTTP         | Echo v5                  |
| ORM          | GORM + pgx (PostgreSQL)  |
| Auth tokens  | JWT (golang-jwt/jwt v5)  |
| Config       | Viper                    |
| Logging      | zerolog                  |

---

## Architecture

The project follows a clean layered architecture. Every domain (`auth`, `notes`, `users`) is structured identically:

```
Handler  →  Service  →  Repository  →  Database
```

- **Handler** — parses the HTTP request, calls the service, maps errors to HTTP status codes.
- **Service** — contains all business logic; depends only on interfaces (repository, email sender, etc.), not concrete types.
- **Repository** — owns all database queries via GORM.

Dependency injection is constructor-based. All cross-layer dependencies are expressed as Go interfaces, which means services and handlers can be unit-tested without a real database or email server.

### Package layout

```
cmd/
  server/main.go          — entry point: load config, connect DB, start server

internal/
  auth/                   — magic link auth, JWT issuance, refresh tokens
  notes/                  — note CRUD + sync (upsert, tombstone deletes)
  users/                  — user model and repository
  email/                  — email sender interface + console sender (dev)
  middleware/             — request logger, JWT auth middleware
  utils/                  — JWT manager, SHA-256 token hashing
  routes.go               — central route registration (wires all dependencies)

pkg/
  app/                    — Echo setup, middleware, graceful shutdown
  config/                 — Viper config loading, DB connection + auto-migrate
  logger/                 — zerolog initialisation
```

`internal/` is locked to this module. `pkg/` is intended to be importable by the extending binary.

---

## Authentication

Passwordless magic-link flow:

1. **Register** — `POST /api/v1/auth/register` — creates a user and sends a magic link.
2. **Request magic link** — `POST /api/v1/auth/magic-link` — sends a new magic link to a registered email.
3. **Verify** — `POST /api/v1/auth/verify` — exchanges the magic link token for an access token + refresh token.
4. **Verify redirect** — `GET /api/v1/auth/verify-redirect` — deep-link handler for email clients that auto-follow links; redirects to `runer://auth/verify?token=<token>`.
5. **Refresh** — `POST /api/v1/auth/refresh` — issues a new access token using the refresh token.
6. **Logout** — `POST /api/v1/auth/logout` _(auth required)_ — revokes the refresh token.

### Token security design

- Magic link tokens and refresh tokens are **32-byte cryptographically random** values (`crypto/rand`), base64url-encoded.
- Only the **SHA-256 hash** of the raw token is stored in the database. A database breach does not yield usable tokens.
- The magic link token is **single-use** — `MarkMagicLinkTokenAsUsed` uses `RowsAffected == 0` as a concurrency guard so two simultaneous requests with the same token cannot both succeed.
- Access tokens and refresh tokens carry a **type claim**; a refresh token cannot be used as an access token and vice versa.
- The `POST /api/v1/auth/magic-link` endpoint always returns HTTP 200, regardless of whether the email is registered, to prevent user enumeration.

---

## Notes API

All note endpoints are protected (`Authorization: Bearer <access_token>`).

| Method   | Path                        | Description                              |
|----------|-----------------------------|------------------------------------------|
| `GET`    | `/api/v1/notes`             | Fetch all notes (supports delta sync)    |
| `GET`    | `/api/v1/notes/:note_id`    | Fetch a single note                      |
| `PUT`    | `/api/v1/notes/:note_id`    | Create or update a note (upsert)         |
| `DELETE` | `/api/v1/notes/:note_id`    | Delete a note (tombstone)                |

### Note model

Notes store an **opaque `encrypted_payload`** (binary). The server never sees plaintext content — encryption and decryption happens on the client.

### Delta sync

`GET /api/v1/notes` accepts a `since` query parameter (ISO 8601 timestamp). When provided, only notes updated after that timestamp are returned. This enables efficient incremental sync — clients only pull what changed since their last sync.

### Tombstone pattern (offline-first deletes)

When a note is deleted, a `NoteTombstone` record is written with the `note_id`, `user_id`, and `deleted_at` timestamp. On the next sync, clients receive the tombstone list alongside live notes, so deletes propagate correctly to all devices — including those that were offline at the time of deletion.

A background purge job (not yet wired) will remove tombstones older than 30 days.

### Conflict detection

`PUT /api/v1/notes/:note_id` accepts an `updated_at` field from the client. If the server's copy has a newer `updated_at`, the upsert is rejected with `409 Conflict`. This is an optimistic-locking approach suitable for an offline-first sync model.

---

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```env
PORT=:8080
ENV=development

JWT_SECRET=<generate with: openssl rand -hex 32>
JWT_TOKEN_DURATION=15m
JWT_REFRESH_TOKEN_DURATION=720h
MAGIC_LINK_TOKEN_DURATION=15m

DATABASE_URL=postgresql://postgres:postgres@localhost:5432/runer_notes
DATABASE_LOG_LEVEL=warn
DATABASE_MAX_IDLE_CONNS=10
DATABASE_MAX_OPEN_CONNS=100
DATABASE_CONN_MAX_LIFETIME=1h

APP_BASE_URL=http://localhost:8080
```

All values can also be provided as environment variables — Viper reads from both. Config defaults are set before `.env` is loaded, so missing keys fall back to safe defaults.

---

## Using as a library

Add to your `go.mod`:

```bash
go get github.com/runernotes/runer-api@v0.1.0
```

Only packages under `pkg/` are intended for external use:

```go
import (
    "github.com/runernotes/runer-api/pkg/app"
    "github.com/runernotes/runer-api/pkg/config"
    "github.com/runernotes/runer-api/pkg/logger"
)
```

`internal/` packages are locked to this module and cannot be imported externally.

---

## Releases

Releases are driven by **git tags**. The Go module proxy picks up tags automatically — no compiled binaries are shipped.

To cut a new release:

```bash
git tag v0.2.0
git push origin v0.2.0
```

This triggers the release workflow which:
1. Verifies the module builds cleanly
2. Creates a GitHub Release with auto-generated notes from commit history

**Versioning follows [semver](https://semver.org/):**
- `v0.x` — API is still settling, breaking changes are possible
- `v1.0.0` — API stability guaranteed
- `v2+` — major breaking change; import path must change to `.../v2`

---

## Running locally

**Prerequisites:** Go 1.23+, PostgreSQL

```bash
# Install dependencies
go mod download

# Copy and edit config
cp .env.example .env

# Run
go run ./cmd/server
```

The server auto-migrates all tables on startup (GORM `AutoMigrate`).

## Building

```bash
go build -o runer-api ./cmd/server
./runer-api
```

For a minimal production binary:

```bash
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o runer-api ./cmd/server
```

---

## Key design decisions

| Decision | Rationale |
|---|---|
| **`internal/` vs `pkg/`** | Business logic lives in `internal/` and cannot be imported by other modules. Infrastructure (config, app setup, logger) lives in `pkg/` and is importable by the extending binary. |
| **Interface-based DI** | Every service depends on interfaces, not concrete types. This keeps layers decoupled and makes unit testing straightforward without a live database. |
| **Opaque refresh tokens** | Refresh tokens are random bytes, not JWTs. This means they can be revoked by hash lookup and a compromised token cannot be decoded to extract claims. |
| **Encrypted payload** | The server stores `bytea` blobs. Client-side encryption means a database breach exposes no plaintext note content. |
| **Tombstone deletes** | Hard-deleting a row would silently drop the record on the next sync. A tombstone ensures every client eventually learns about the deletion. |
| **Composite index `(user_id, updated_at)`** | Directly supports the delta-sync query: `WHERE user_id = ? AND updated_at > ?`. |
| **Centralized route registration** | `internal/routes.go` is the single place to audit the full API surface and dependency wiring. |

---

## Known gaps / roadmap


- [ ] Soft-delete / trash (spec requires `is_deleted` flag, not only tombstone)
- [ ] Background job to purge tombstones older than 30 days
- [ ] Replace `ConsoleSender` with mail provider for production email delivery
- [ ] Rate limiting on auth endpoints
- [ ] CORS configuration
- [ ] Production JSON logger (currently always uses `ConsoleWriter`)
- [ ] JWT `Audience` claim and refresh token rotation
