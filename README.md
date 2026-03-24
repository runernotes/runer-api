# Runer API

A RESTful notes API built in Go. It implements authentication, note sync, trash/restore, and subscription-based quota enforcement.

---

## Stack

| Layer        | Library                  |
|--------------|--------------------------|
| HTTP         | Echo v5                  |
| ORM          | GORM + pgx (PostgreSQL)  |
| Auth tokens  | JWT (golang-jwt/jwt v5)  |
| Config       | Viper                    |
| Logging      | zerolog                  |
| Email        | Resend (falls back to console in dev) |

---

## Architecture

The project follows a clean layered architecture. Every domain (`auth`, `notes`, `users`, `subscription`) is structured identically:

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
  api/                    — shared DTOs (ErrorResponse, MessageResponse)
  auth/                   — magic link auth, JWT issuance, refresh tokens
  config/                 — Viper config loading, DB connection + auto-migrate
  email/                  — email sender interface, Resend sender, console sender (dev)
  middleware/             — request logger, JWT auth middleware, rate limiter
  notes/                  — note CRUD, sync (upsert, trash, restore, purge, tombstones)
  subscription/           — subscription query handler (plan + quota)
  users/                  — user model and repository
  utils/                  — JWT manager, SHA-256 token hashing
  validator/              — Echo validator setup
  routes.go               — central route registration (wires all dependencies)
```

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

| Method   | Path                             | Description                              |
|----------|----------------------------------|------------------------------------------|
| `GET`    | `/api/v1/notes`                  | Fetch all notes (supports delta sync)    |
| `GET`    | `/api/v1/notes/:note_id`         | Fetch a single note                      |
| `PUT`    | `/api/v1/notes/:note_id`         | Create or update a note (upsert)         |
| `DELETE` | `/api/v1/notes/:note_id`         | Trash a note (soft-delete)               |
| `POST`   | `/api/v1/notes/:note_id/restore` | Restore a trashed note                   |
| `DELETE` | `/api/v1/notes/:note_id/purge`   | Permanently delete a note (tombstone)    |

### Note model

Notes store an **opaque `encrypted_payload`** (binary). The server never sees plaintext content — encryption and decryption happens on the client.

### Delta sync

`GET /api/v1/notes` accepts a `since` query parameter (ISO 8601 timestamp). When provided, only notes updated after that timestamp are returned. Trashed notes are included in sync responses (with `trashed_at` populated) so all devices learn about trash and restore events.

### Trash / restore / purge

- **`DELETE /notes/:note_id`** — soft-delete (trash). Sets `trashed_at = now` and bumps `updated_at`. The note remains in sync responses so clients can update their local state.
- **`POST /notes/:note_id/restore`** — clears `trashed_at` and bumps `updated_at`.
- **`DELETE /notes/:note_id/purge`** — permanent hard-delete. Writes a `NoteTombstone` record and removes the note.

### Tombstone pattern (offline-first purges)

When a note is permanently deleted via purge, a `NoteTombstone` record is written with the `note_id`, `user_id`, and `deleted_at` timestamp. On the next sync, clients receive the tombstone list so purges propagate correctly to all devices — including those that were offline.

A background purge job (not yet wired) will remove tombstones older than 30 days.

### Conflict detection

`PUT /api/v1/notes/:note_id` accepts an `updated_at` field from the client. If the server's copy has a newer `updated_at`, the upsert is rejected with `409 Conflict`. This is an optimistic-locking approach suitable for an offline-first sync model.

---

## Subscription API

`GET /api/v1/subscription` _(auth required)_ — returns the user's current plan, note count, and note limit.

```json
{
  "plan": "free",
  "note_count": 12,
  "note_limit": 50
}
```

Free plan users are hard-capped at `FREE_NOTE_LIMIT` active notes (default 50). Pro plan users have `note_limit: null` (unlimited). Quota is enforced on `PUT /notes/:note_id` for new notes.

---

## Health check

`GET /health` — unauthenticated liveness probe.

```json
{ "status": "ok" }
```

---

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```env
PORT=:8080
ENV=development

JWT_SECRET=<generate with: openssl rand -hex 32>
JWT_TOKEN_DURATION=1h
JWT_REFRESH_TOKEN_DURATION=168h
MAGIC_LINK_TOKEN_DURATION=1h

DATABASE_URL=postgresql://postgres:postgres@localhost:5432/runer_notes
DATABASE_LOG_LEVEL=warn
DATABASE_MAX_IDLE_CONNS=10
DATABASE_MAX_OPEN_CONNS=100
DATABASE_CONN_MAX_LIFETIME=1h

APP_BASE_URL=http://localhost:8080
FREE_NOTE_LIMIT=50

# Email — if RESEND_API_KEY is unset the server logs magic links to stdout (dev mode)
RESEND_API_KEY=
EMAIL_FROM=noreply@example.com
```

All values can also be provided as environment variables — Viper reads from both. Config defaults are set before `.env` is loaded, so missing keys fall back to safe defaults.

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
| **Interface-based DI** | Every service depends on interfaces, not concrete types. This keeps layers decoupled and makes unit testing straightforward without a live database. |
| **Opaque refresh tokens** | Refresh tokens are random bytes, not JWTs. This means they can be revoked by hash lookup and a compromised token cannot be decoded to extract claims. |
| **Encrypted payload** | The server stores `bytea` blobs. Client-side encryption means a database breach exposes no plaintext note content. |
| **Trash before purge** | `DELETE` only trashes (soft-delete). A separate `purge` endpoint does the hard delete. This gives users a recoverable state and makes offline-first conflict resolution predictable. |
| **Tombstone on purge only** | Tombstones are only written on hard-delete (purge). Trashed notes propagate via `trashed_at` on the note itself, keeping the tombstone table small. |
| **Composite index `(user_id, updated_at)`** | Directly supports the delta-sync query: `WHERE user_id = ? AND updated_at > ?`. |
| **Centralized route registration** | `internal/routes.go` is the single place to audit the full API surface and dependency wiring. |
| **Console email fallback** | If `RESEND_API_KEY` is not set, magic links are printed to stdout. No additional config needed for local development. |

---

## Known gaps / roadmap

- [ ] Background job to purge tombstones older than 30 days
- [ ] JWT `Audience` claim and refresh token rotation
- [ ] Production JSON logger (currently always uses `ConsoleWriter`)
