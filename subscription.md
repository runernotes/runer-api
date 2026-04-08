# Subscription & Quota — Implementation Plan

This plan covers what's needed to enforce the free-plan note quota and expose a `GET /subscription` endpoint — **without** billing (no Stripe, no IAP, no webhooks). Plan changes are done directly in the database.

---

## Deviations from spec

- The spec stores billing state in a separate `subscriptions` table. For now, the `Plan` field lives directly on the `User` model. This is simpler and sufficient until billing is wired in.
- The spec and codebase both use `pro` as the plan value (not `paid`).
- No `POST /subscription/checkout` or webhook endpoints — those are blocked until billing is implemented.

---

## Implementation

### 1. Config: add `FreeNoteLimit`

**File:** `internal/config/config.go`

Add a new field to `Config`:

```go
FreeNoteLimit int `mapstructure:"FREE_NOTE_LIMIT"`
```

Default: `50` (set in `setDefaults()`).

This makes the cap configurable via `FREE_NOTE_LIMIT` env var while falling back to 50.

---

### 2. Fix `CountLiveNotes` — exclude trashed notes

**File:** `internal/notes/notesRepository.go`

The existing `CountLiveNotes` counts **all** notes including trashed ones. Quota should only count live (non-trashed) notes.

Change:
```go
Where("user_id = ?", userID)
```
To:
```go
Where("user_id = ? AND trashed_at IS NULL", userID)
```

---

### 3. Add quota check to `UpsertNote`

**File:** `internal/notes/notesService.go`

The service needs to know the user's plan and the note limit. Two options:

**Option A — Pass dependencies into service:**
- `NotesService` gets a `usersRepository` (to look up user plan) and `freeNoteLimit int`.
- Before inserting a **new** note (no `baseVersion` and note doesn't already exist), check:
  1. Fetch user plan from `usersRepository.FindByID()`
  2. If plan is `free`, call `CountLiveNotes()`
  3. If count >= `freeNoteLimit`, return a new `QuotaExceededError`

**Option B — Quota middleware:** More complex and less precise (can't distinguish create vs update). Option A is simpler.

Go with **Option A**.

New error type in `internal/notes/errors.go`:
```go
var ErrQuotaExceeded = errors.New("quota exceeded")
```

The handler maps `ErrQuotaExceeded` → `403 { "error": "Quota exceeded", "code": "QUOTA_EXCEEDED" }`.

**Important:** Only check quota on **create** (new note), not on **update** (note already exists). An update with `baseVersion` to an existing note should always be allowed regardless of quota — the user isn't adding a new note.

Logic:
1. If `baseVersion != nil` → this is an update, skip quota check.
2. If `baseVersion == nil` → check if note already exists for this user.
   - If it exists → this is an update (re-push), skip quota check.
   - If it doesn't exist → this is a create, enforce quota.

---

### 4. Wire dependencies

**File:** `internal/routes.go`

- Pass `usersRepository` and `cfg.FreeNoteLimit` into `NewNotesService()`.
- Update `NewNotesService` signature accordingly.

---

### 5. Lock down `Plan` — exclude from user update DTO

**File:** `internal/users/usersHandler.go`

The `UpdateUser` handler must use a request DTO that excludes `Plan`. Only fields like `Name` and `Email` should be settable through the API. The handler should never bind `Plan` from the request body onto the `User` model.

Example DTO:
```go
type UpdateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}
```

The `Plan` field is only changed directly in the database (by an admin or future billing webhooks), never through a user-facing endpoint.

---

### 6. Add `GET /api/v1/subscription` endpoint

**Files:**
- `internal/subscription/handler.go` — new handler
- `internal/subscription/dto.go` — response DTO
- `internal/routes.go` — register the route

Handler logic:
1. Extract `userID` from JWT context.
2. Look up user plan from `usersRepository.FindByID()`.
3. Count live notes via `notesRepository.CountLiveNotes()`.
4. Return:

```json
{
  "plan": "free",
  "note_count": 12,
  "note_limit": 50
}
```

If plan is `pro`: `note_limit` is `null`.

The endpoint requires auth middleware.

---

### 6. Handler: map `ErrQuotaExceeded` to 403

**File:** `internal/notes/notesHandler.go`

In the `Upsert` handler, add a check for the new error:

```go
if errors.Is(err, ErrQuotaExceeded) {
    return c.JSON(http.StatusForbidden, api.ErrorResponse{
        Error: "Quota exceeded",
        Code:  "QUOTA_EXCEEDED",
    })
}
```

---

## E2E Tests

All tests change the user's plan directly via the `*gorm.DB` handle available in the test setup — no admin endpoint needed.

Helper for tests:
```go
func setUserPlan(t *testing.T, db *gorm.DB, userID uuid.UUID, plan string) {
    t.Helper()
    err := db.Model(&users.User{}).Where("id = ?", userID).Update("plan", plan).Error
    require.NoError(t, err)
}
```

**File:** `e2e/subscription_test.go`

### Subscription endpoint

- **Get subscription for new user returns free plan** — register a user, `GET /subscription`; expect `{ plan: "free", note_count: 0, note_limit: 50 }`.
- **Get subscription requires auth** — `GET /subscription` without token returns 401. Baseline: same endpoint with token returns 200.
- **Pro user has unlimited note_limit** — register a user, set plan to `pro` via DB, `GET /subscription`; expect `{ plan: "pro", note_count: 0, note_limit: null }`.
- **Note count reflects actual live notes** — create 3 notes, `GET /subscription`; expect `note_count: 3`. Trash one, expect `note_count: 2`.

### Quota enforcement

**File:** `e2e/notes_quota_test.go`

- **Free user creating note beyond limit returns 403 `QUOTA_EXCEEDED`** — use a low `FreeNoteLimit` (e.g. 3) in test config to avoid creating 50 notes. Create 3 notes, then attempt a 4th; expect 403 with `QUOTA_EXCEEDED`. Baseline: the 3rd note must return 200.
- **Trashed notes do not count toward the free quota** — create 3 notes (at limit), trash one, create another; expect 200 (2 live notes < limit).
- **Pro user is not subject to the note quota** — register user, set plan to `pro` via DB, create notes beyond the limit; all succeed.
- **Updating an existing note does not trigger quota check** — create notes up to the limit, then update one of them (PUT with `base_version`); expect 200.
- **Re-pushing a note (PUT without base_version to existing note_id) does not trigger quota** — create notes up to the limit, re-push one; expect 200.

### Test config note

The e2e test server should use a small `FreeNoteLimit` (e.g. 3) to keep tests fast. Set this in `newTestServer()`:

```go
cfg := &config.Config{
    // ...existing fields...
    FreeNoteLimit: 3,
}
```

---

## Files changed (summary)

| File | Change |
|---|---|
| `internal/config/config.go` | Add `FreeNoteLimit` field + default |
| `internal/notes/notesRepository.go` | Fix `CountLiveNotes` to exclude trashed |
| `internal/notes/notesService.go` | Add users repo + quota check to `UpsertNote` |
| `internal/notes/errors.go` | Add `ErrQuotaExceeded` |
| `internal/notes/notesHandler.go` | Map `ErrQuotaExceeded` → 403 |
| `internal/routes.go` | Wire new deps, register subscription route |
| `internal/subscription/handler.go` | New — `GET /subscription` handler |
| `internal/subscription/dto.go` | New — response DTO |
| `e2e/server_test.go` | Set `FreeNoteLimit` in test config |
| `e2e/subscription_test.go` | New — subscription endpoint tests |
| `e2e/notes_quota_test.go` | New — quota enforcement tests |

---

## Out of scope

- Stripe checkout / webhooks
- IAP webhooks
- `POST /subscription/checkout` endpoint
- Admin endpoints for plan changes
- JWT `plan` claim embedding
- Separate `subscriptions` table (deferred until billing)
