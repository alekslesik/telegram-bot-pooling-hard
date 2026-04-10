# Admin v2 & roles — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the gaps listed under RFC §4 in `docs/rfc-todo-status.md`: full role scope (including admin roster), batch slot operations, blackout/holiday rules, and a stronger audit trail for critical admin actions.

**Architecture:** Keep `transport → service → repository`. Telegram handlers stay thin; `BookingService` enforces `AdminCapabilities` and calls `BookingRepository`. New persistence lives in SQL migrations plus `PostgresRepository` / `MemoryRepository` parity. Prefer small, testable repository primitives composed in the service layer.

**Tech stack:** Go 1.26.x, PostgreSQL, existing `internal/bot`, `internal/service`, `internal/repository`, migrations under `migrations/`.

**Baseline (this repo today):**

- Roles: `owner` / `admin` / `operator` on `admins.role` (`migrations/013_admin_roles_v2.sql`). Capability matrix in `internal/service/booking.go` (`canManageCatalog`, `canManageDaySlots`, `canViewAnalytics`).
- Admin UI: inline keyboard in `internal/bot/handlers.go` (`adminKeyboard`), states in `booking.go` (`StateAdmin*`).
- Day tools: `CloseDoctorDay`, `OpenDoctorDay`, `ListDoctorSlotsForDay`, `GenerateDoctorSlots` in `internal/repository/postgres.go`.
- Audit: `LogAdminAction` → `admin_audit_logs`; used for catalog + day tools in `booking.go` (not for all critical paths).

---

## File map (expected touch points)

| Area | Create | Modify |
|------|--------|--------|
| Migrations | `migrations/0NN_admin_blackouts.sql`, optional `0NN_admin_audit_extend.sql` | — |
| Repository API | types for blackout rows, optional audit list DTO | `internal/repository/repository.go` (`BookingRepository`) |
| Postgres | new methods | `internal/repository/postgres.go` |
| Memory repo | same methods | `internal/repository/repository.go` (`MemoryRepository`) |
| Service | batch + blackout orchestration, owner-only admin CRUD | `internal/service/booking.go` |
| Capabilities | new flags if needed | `AdminCapabilities`, `canManage*` helpers |
| Bot transport | callbacks + text flows | `internal/bot/handlers.go` |
| Tests | new `_test.go` cases | `internal/repository/repository_test.go`, `internal/service/booking_test.go`, integration if present |

---

### Task 1: Batch doctor slot generation (date range)

**Files:**

- Modify: `internal/repository/repository.go` — add `GenerateDoctorSlotsDateRange(ctx, doctorID, specialtyID int64, fromDate, toDate time.Time, startMinute, endMinute, stepMinutes int) (int, error)` to `BookingRepository`.
- Modify: `internal/repository/postgres.go` — implement with **one transaction** looping UTC calendar days from `fromDate` through `toDate` (inclusive), reusing the same insert semantics as `GenerateDoctorSlots` (`ON CONFLICT (doctor_id, specialty_id, start_at) DO NOTHING`). Refactor shared insert into an unexported helper that accepts `*sql.Tx` if needed.
- Modify: `internal/repository/repository.go` (`MemoryRepository`) — loop days and call existing in-memory slot generation logic; match Postgres “insert count” semantics (only new rows).
- Modify: `internal/service/booking.go` — `handleAdminGenerateSlots` (or parallel handler) accepts range input, e.g. `doctor_id|specialty_id|YYYY-MM-DD|YYYY-MM-DD|startMin|endMin|step` **or** keep single-day format and add a separate admin action `admin:slotsrange` with its own state/message. Enforce `CanManageCatalog` (same as current generate slots).
- Modify: `internal/bot/handlers.go` — button + callback branch for the new flow.
- Test: `internal/repository/repository_test.go`

- [ ] **Step 1: Write the failing test**

Add `TestMemoryRepository_GenerateDoctorSlotsDateRange` that picks a known `doctorID`/`specialtyID` from seeded memory data, chooses two consecutive UTC dates with no pre-existing slots, calls `GenerateDoctorSlotsDateRange` with a short window (e.g. 10:00–11:00, step 30), asserts total inserted ≥ 2 and that `ListDoctorSlotsForDay` returns the expected count for each day.

```go
func TestMemoryRepository_GenerateDoctorSlotsDateRange(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	from := time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 5, 2, 0, 0, 0, 0, time.UTC)
	n, err := repo.GenerateDoctorSlotsDateRange(ctx, 1, 1, from, to, 10*60, 11*60, 30)
	if err != nil {
		t.Fatalf("range generate: %v", err)
	}
	if n < 4 {
		t.Fatalf("expected at least 4 inserts, got %d", n)
	}
	day1, _ := repo.ListDoctorSlotsForDay(ctx, 1, 1, from)
	day2, _ := repo.ListDoctorSlotsForDay(ctx, 1, 1, to)
	if len(day1) < 2 || len(day2) < 2 {
		t.Fatalf("unexpected per-day counts: %d, %d", len(day1), len(day2))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/... -run TestMemoryRepository_GenerateDoctorSlotsDateRange -v`

Expected: FAIL (undefined method or compile error).

- [ ] **Step 3: Implement interface + MemoryRepository + PostgresRepository**

Add the method to `BookingRepository`, implement memory + postgres until the test passes.

- [ ] **Step 4: Run repository tests**

Run: `go test ./internal/repository/... -count=1`

Expected: PASS.

- [ ] **Step 5: Service + bot wiring + booking_test**

Add a focused test in `internal/service/booking_test.go` that stubs or uses memory repo: operator must be denied, admin allowed, successful path returns non-empty summary. Wire handler callback in `handlers.go`.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/repository.go internal/repository/postgres.go internal/service/booking.go internal/bot/handlers.go internal/repository/repository_test.go internal/service/booking_test.go
git commit -m "feat(admin): batch generate doctor slots for date range"
```

---

### Task 2: Batch close / open multiple days

**Files:**

- Modify: `internal/repository/repository.go` — `CloseDoctorDaysRange`, `OpenDoctorDaysRange` (or a single `ForEachDayInRange` helper used by service). Same transaction semantics as Task 1: all days succeed or rollback.
- Modify: `internal/repository/postgres.go`, `MemoryRepository`, `internal/service/booking.go`, `internal/bot/handlers.go`, tests.

- [ ] **Step 1: Write failing test** — extend `TestMemoryRepository_AdminDayTools_CloseOpenAndView` pattern: close range of 2 days, assert all slots unavailable; open range, assert non-booked slots available.

- [ ] **Step 2: Implement repository methods** (delegate to existing `CloseDoctorDay` / `OpenDoctorDay` in a transaction on Postgres).

- [ ] **Step 3: Service + bot** — only for roles with `CanManageDaySlots`; log via `LogAdminAction` with action names `close_doctor_days` / `open_doctor_days` and details including date range.

- [ ] **Step 4: `go test ./...` and commit** — `feat(admin): batch close and open doctor days`

---

### Task 3: Blackout / holiday rules (block new slots + optional booking guard)

**Files:**

- Create: `migrations/014_schedule_blackouts.sql` (number may shift; use next free serial in repo).

```sql
CREATE TABLE IF NOT EXISTS schedule_blackouts (
    id BIGSERIAL PRIMARY KEY,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    doctor_id BIGINT REFERENCES doctors(id) ON DELETE CASCADE,
    specialty_id BIGINT REFERENCES specialties(id) ON DELETE CASCADE,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT schedule_blackouts_range_check CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_schedule_blackouts_window
    ON schedule_blackouts USING gist (tstzrange(starts_at, ends_at, '[)'));

CREATE INDEX IF NOT EXISTS idx_schedule_blackouts_doctor
    ON schedule_blackouts (doctor_id, starts_at);
```

(If `gist` extension is undesirable in your environment, replace with btree on `(starts_at, ends_at)` and document overlap queries in repository comments.)

- Modify: `BookingRepository` — `InsertScheduleBlackout(...)`, `ListScheduleBlackoutsOverlapping(ctx, doctorID, specialtyID *int64, from, to time.Time) ([]ScheduleBlackout, error)` (nullable IDs = clinic-wide blackout).
- Modify: `GenerateDoctorSlots` / `GenerateDoctorSlotsDateRange` implementations to **skip** or **no-op** intervals overlapping blackouts (simplest: after insert, delete conflicting rows in same transaction; or pre-check per slot window — choose one approach and test).
- Modify: `ListAvailableDoctorSlots` (or confirm path) to exclude slots overlapping an active blackout if product requires “no booking during holiday”.
- Service: owner/admin only `CanManageBlackouts` new capability; operator read-only none.
- Bot: minimal “add blackout” text flow + list/delete later phase (split Task 3b if needed).

- [ ] **Step 1: Migration applied in dev** — verify with local Postgres.

- [ ] **Step 2: Repository tests** — insert blackout overlapping generated slots; assert slots not bookable or not listed.

- [ ] **Step 3: Service + bot + commit** — `feat(schedule): add blackout windows for doctor slots`

---

### Task 4: Owner-only admin roster (full role scope)

**Gap:** Admins are inserted via SQL seeds; there is no `BookingRepository` method to add/remove admins. RFC “full scope” implies **owner** can grant/revoke roles.

**Files:**

- Modify: `internal/repository/repository.go` — `UpsertAdmin(ctx, telegramUserID int64, role AdminRole, active bool) error`, `ListAdmins(ctx) ([]AdminRecord, error)` (new struct with `TelegramUserID`, `Role`, `IsActive`).
- Modify: `internal/repository/postgres.go` — `INSERT ... ON CONFLICT (telegram_user_id) DO UPDATE`.
- Modify: `internal/service/booking.go` — `canManageAdmins(role) bool` only `owner`; new handlers for text flow or callbacks.
- Modify: `internal/bot/handlers.go` — expose only when `caps.CanManageAdmins`.
- Extend: `AdminCapabilities` with `CanManageAdmins bool`.

- [ ] **Step 1: Test** — `TestBookingService_AdminCapabilities_OwnerGetsManageAdmins` in `booking_test.go`.

- [ ] **Step 2: Implement repo + service + bot.**

- [ ] **Step 3: Commit** — `feat(admin): owner can upsert admin roster`

---

### Task 5: Extended audit trail

**Goals:**

1. Use structured JSON in `details` for new actions (and optionally migrate log lines incrementally) so reports can parse payloads.
2. Log **critical** operations missing today (at minimum: any new admin roster changes, blackout changes, batch slot mutations; consider `ConfirmPaidClinicBooking` cancel path if treated as admin — only if product agrees).
3. Optional: `ListAdminAuditLogs(ctx, adminUserID int64, limit int) ([]AdminAuditLog, error)` restricted to owner in service layer.

- [ ] **Step 1: Define small JSON helpers** in `internal/service` or `internal/repository` — e.g. `auditDetails(map[string]any) string` using `encoding/json`.

- [ ] **Step 2: Replace string `details` in new code paths** with JSON; add `ListAdminAuditLogs` + postgres query ordered by `created_at DESC`.

- [ ] **Step 3: Bot read-only “audit tail” for owner** (last N lines) or defer to Task 5b.

- [ ] **Step 4: Tests** — memory repo stores logs; assert JSON parses.

- [ ] **Step 5: Commit** — `feat(admin): structured audit details and owner audit listing`

---

## Self-review

| RFC §4 row | Covered by |
|------------|------------|
| Roles to full scope | Task 4 (`CanManageAdmins`), Task 3 (`CanManageBlackouts`); verify matrix in tests |
| Batch slot operations | Tasks 1–2 |
| Blackout / holiday rules | Task 3 |
| Extended audit trail | Task 5 |

**Placeholder scan:** No `TBD` steps; optional items are explicitly scoped (Task 3b, 5b).

**Type consistency:** New repository methods use existing `AdminRole`, `time.Time` in UTC matching `GenerateDoctorSlots`.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-10-admin-v2-roles-implementation.md`.

**1. Subagent-driven (recommended)** — fresh subagent per task, review between tasks.

**2. Inline execution** — run tasks in order in one session with checkpoints after each task.

**Which approach?**

Before opening a PR, run `make preprod` per repository workflow.
