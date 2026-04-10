# Analytics & Funnels (RFC §3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align analytics ingestion with a single JSON envelope across bot and worker, add funnel step events, expose cancellation / no-show proxy / referral / retention metrics, and let admins choose report period and optional specialty segment.

**Architecture:** Keep `transport -> service -> repository`. Bot and service continue to call `BookingService.LogAnalytics`; worker paths use the same envelope builder in package `service` (already holds `logAnalyticsWithEnvelope`). New read-only SQL aggregates live in `postgres.go` behind small repository methods; `AdminAnalyticsReport` gains parameters and formats fixed report sections for the admin callback.

**Tech Stack:** Go, PostgreSQL, existing `analytics_events` and `clinic_bookings` / `doctor_slots` / `user_profiles`, `go test`, Telegram callback `admin:*`.

---

## File Structure

- **Modify:** `internal/service/analytics.go` — keep envelope builder; add exported helper if worker/tests need a stable entry (optional: only if tests cannot stay in-package).
- **Modify:** `internal/service/outbox_worker.go` — route dead-letter analytics through `logAnalyticsWithEnvelope`.
- **Modify:** `internal/repository/repository.go` — new read methods for aggregates (cancellations, referrals, no-show proxy, funnel-friendly counts, optional specialty filter).
- **Modify:** `internal/repository/postgres.go` — SQL for those methods + `MemoryRepository` stubs in `internal/repository/repository.go` (or dedicated memory test file) returning zeros where not modeled.
- **Modify:** `internal/service/platform.go` — extend `AdminAnalyticsReport` signature (days, optional `specialtyID *int64`) and compose multi-section text.
- **Modify:** `internal/bot/handlers.go` — funnel `LogAnalytics` calls on `book:spec`, `book:doc`, `book:slot` / slot list views as agreed below; admin buttons `admin:analytics:7` / `admin:analytics:30` and optional `admin:analyticsseg:<specID>` or a two-step flow.
- **Modify:** `internal/i18n/i18n.go` (only if new user-visible strings are required for admin).
- **Test:** `internal/service/outbox_worker_test.go` — assert dead-letter row uses envelope with `"source":"worker"`.
- **Test:** `internal/service/platform_test.go` — extend or add tests for new report sections and period behavior.
- **Test:** `internal/repository/postgres` integration tests if the repo already uses them for analytics; otherwise `level3_memory_test.go` + postgres test file pattern used elsewhere.

Responsibilities:

- Envelope parity: every row in `analytics_events` has `payload_json` matching RFC shape: `event_name`, `user_id`, `context`, `ts`, `source` (already produced for bot via `buildAnalyticsEnvelopePayload`; worker must match).
- Funnel: distinct `event_type` values per step so `CountAnalyticsByEventSince` can compute step counts and rough conversion (ratio of counts, not strict user-level cohort without extra work — document limitation in report footer).
- Reports: SQL from truth tables (`clinic_bookings`, `user_profiles`, `doctor_slots`) for cancellations, no-show proxy, referral signups; retention as “users with `cmd_start` in window W who also have `booking_confirmed` after start within N days” can be a later task or simplified v1 (see Task 5).

---

### Task 1: Worker analytics uses the same envelope as the bot

**Files:**

- Modify: `internal/service/outbox_worker.go:69-72`
- Modify: `internal/service/outbox_worker_test.go`
- Test: `internal/service/outbox_worker_test.go`

- [ ] **Step 1: Write failing test — dead-letter payload contains envelope fields**

```go
func TestOutboxWorker_DeadLetterAnalytics_UsesEnvelope(t *testing.T) {
    // Memory repo with ClaimDueOutboxEvents returning one item, handler always err,
    // maxAttempts 1 so first failure marks dead.
    // After Tick, CountAnalyticsByEventSince(now-1m) must include outbox_event_dead
    // and the stored payload_json ( unmarshalled ) must have keys: event_name, source, ts, context
    // with source == "worker".
}
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./internal/service -run OutboxWorker_DeadLetterAnalytics_UsesEnvelope -v`  
Expected: FAIL (payload still raw JSON without envelope).

- [ ] **Step 3: Replace direct `LogAnalyticsEvent` with envelope helper**

```go
// In outbox_worker.go when marking dead:
ctxJSON := fmt.Sprintf(`{"event_id":%d,"event_type":%q,"attempts":%d}`, item.ID, item.EventType, item.Attempts)
_ = logAnalyticsWithEnvelope(ctx, w.repo, nil, "outbox_event_dead", ctxJSON, "worker")
```

- [ ] **Step 4: Run test — expect PASS**

Run: `go test ./internal/service -run OutboxWorker_DeadLetterAnalytics_UsesEnvelope -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/outbox_worker.go internal/service/outbox_worker_test.go
git commit -m "fix(analytics): use unified envelope for worker dead-letter events"
```

---

### Task 2: Funnel step events from the booking callback flow

**Files:**

- Modify: `internal/bot/handlers.go` (`handleBookingCallback` branches `spec`, `doc`, `slot` before confirm, and optionally `specp`/`docp`/`slotp` if you want “impression” counts — v1: log only on committed navigation `spec`, `doc`, `slot` selection before payment)
- Test: `internal/bot/handlers_test.go` (table-driven or existing callback test harness with mock `Booking` recording `LogAnalytics` calls)

Event names (stable, lowercase, snake_case):

- `funnel_book_specialty_selected` — context: `{"specialty_id":<id>}`
- `funnel_book_doctor_selected` — context: `{"specialty_id":<id>,"doctor_id":<id>}`
- `funnel_book_slot_selected` — context: `{"specialty_id":<id>,"doctor_id":<id>,"slot_id":<id>}` (fire when user taps a slot, immediately before `ConfirmClinicBooking`; keep distinct from `booking_confirmed` which already fires on success)

- [ ] **Step 1: Add test — mock expects `funnel_book_specialty_selected` when `book:spec:` handled**

Use the same mock pattern as existing handler tests; assert `LogAnalytics` called once with matching `eventType` and JSON context containing `specialty_id`.

- [ ] **Step 2: Run test — FAIL**

Run: `go test ./internal/bot -run FunnelBookSpecialty -v`

- [ ] **Step 3: Implement `LogAnalytics` in `case "spec":` after validating IDs**

```go
uidp := userID // from callback From.ID
payload, _ := json.Marshal(map[string]int64{"specialty_id": specID})
_ = h.Booking.LogAnalytics(context.Background(), &uidp, "funnel_book_specialty_selected", string(payload))
```

Mirror for `doc` and `slot` cases with the maps above.

- [ ] **Step 4: Run bot tests — PASS**

Run: `go test ./internal/bot -v`

- [ ] **Step 5: Commit**

```bash
git add internal/bot/handlers.go internal/bot/handlers_test.go
git commit -m "feat(analytics): log booking funnel step events"
```

---

### Task 3: Repository aggregates for operations reports

**Files:**

- Modify: `internal/repository/repository.go`
- Modify: `internal/repository/postgres.go`
- Modify: `internal/repository/repository.go` (`MemoryRepository` methods return zero / in-memory counts where feasible)
- Test: new or existing postgres integration test file

Add methods (exact names can match house style):

```go
// CountClinicBookingsCancelledSince counts rows with status = 'cancelled' and cancelled_at >= since.
CountClinicBookingsCancelledSince(ctx context.Context, since time.Time) (int64, error)

// CountNoShowProxySince: confirmed bookings whose slot start_at < now and start_at >= since (proxy only — no check-in).
CountNoShowProxySince(ctx context.Context, since time.Time) (int64, error)

// CountReferralRewardsGrantedSince: user_profiles.referral_reward_granted = true AND updated_at >= since (or created_at if you only set flag once — align with ApplyReferral implementation).
CountReferralRewardsGrantedSince(ctx context.Context, since time.Time) (int64, error)

// CountBookingsConfirmedSinceWithOptionalSpecialty: specialtyID nil = all.
CountBookingsConfirmedSinceWithOptionalSpecialty(ctx context.Context, since time.Time, specialtyID *int64) (int64, error)
```

- [ ] **Step 1: Write postgres test inserting known rows, call each counter, assert numbers**

- [ ] **Step 2: Implement SQL in `PostgresRepository`**

Example no-show proxy query shape:

```sql
SELECT COUNT(*)
FROM clinic_bookings cb
INNER JOIN doctor_slots ds ON ds.id = cb.doctor_slot_id
WHERE cb.status = 'confirmed'
  AND ds.start_at < NOW()
  AND ds.start_at >= $1
```

- [ ] **Step 3: Wire memory repo**

- [ ] **Step 4: `go test ./...` for affected packages**

- [ ] **Step 5: Commit**

```bash
git add internal/repository/
git commit -m "feat(analytics): add repository aggregates for admin reports"
```

---

### Task 4: Extend `AdminAnalyticsReport` with period and specialty segment

**Files:**

- Modify: `internal/service/platform.go` — `AdminAnalyticsReport(ctx, adminUserID, days int, specialtyID *int64) (string, error)`
- Modify: `internal/bot/handlers.go` — pass `7` or `30` and optional segment from callback data
- Modify: `internal/service/platform_test.go`
- Modify: any compile breakages (grep `AdminAnalyticsReport`)

Report sections (plain text lines, stable prefixes for i18n splitting if needed):

1. `period: last N days` (header line)
2. `segment: all` or `segment: specialty_id=<id>`
3. Existing per-event counts from `CountAnalyticsByEventSince` (unchanged semantics)
4. `funnel_conversion_approx:` lines — e.g. ratio `booking_confirmed / funnel_book_specialty_selected` with a note “count-based, not unique users”
5. `bookings_confirmed_total:`, `cancellations:`, `no_show_proxy:`, `referral_rewards_granted:`
6. Existing outbox + wallet mismatch lines

- [ ] **Step 1: Update test `TestBookingServiceAdminAnalyticsReportIncludesWalletMismatches` to new signature** (`days: 7`, `specialty: nil`)

- [ ] **Step 2: Add test asserting new section keys appear when repo seeded**

- [ ] **Step 3: Implement composition in `AdminAnalyticsReport`**

- [ ] **Step 4: `go test ./internal/service -v`**

- [ ] **Step 5: Commit**

```bash
git add internal/service/platform.go internal/service/platform_test.go internal/bot/handlers.go
git commit -m "feat(admin): analytics report period and segment aggregates"
```

---

### Task 5: Retention v1 (optional minimal) + admin callback UX

**Retention v1 (YAGNI-friendly):** Either skip in first merge and leave a single report line `retention_v1: not_implemented`, **or** implement: count distinct `telegram_user_id` from `analytics_events` where `event_type = 'cmd_start'` and `created_at` in [since, since+window]) intersect users with `booking_confirmed` in [since, end]). That requires a new SQL method `CountReturningBookers(...)` — add only if product insists; otherwise document as phase 2 in RFC status.

**Admin UX:**

- Replace single button with two rows:

```go
tgbotapi.NewInlineKeyboardButtonData("📊 7 дней", "admin:analytics:7"),
tgbotapi.NewInlineKeyboardButtonData("📊 30 дней", "admin:analytics:30"),
```

- Parse `admin:analytics` and `admin:analytics:7` / `:30` in the `admin:` callback router.
- **Segmentation v1:** add follow-up message “Reply with specialty id or 0 for all” is heavy for Telegram; prefer third button “По специальности” opening a small flow with paginated specialty picker (reuse list patterns from booking), callback `admin:analyticspec:<days>:<specialty_id>`.

- [ ] **Step 1: Handler test for `admin:analytics:30` calls service with `days=30`**

- [ ] **Step 2: Implement parsing and keyboard**

- [ ] **Step 3: `go test ./internal/bot -v`**

- [ ] **Step 4: Commit**

```bash
git add internal/bot/handlers.go internal/bot/handlers_test.go
git commit -m "feat(admin): analytics period buttons and optional specialty segment"
```

---

## Plan author self-review

**Spec coverage (RFC.md §3 + `docs/rfc-todo-status.md` §3):**

| Requirement | Task |
|-------------|------|
| Unified schema (`event_name`, `user_id`, `context`, `ts`, `source`) | Task 1 (worker parity); bot already uses `buildAnalyticsEnvelopePayload` |
| Funnel metrics (steps, conversion) | Task 2 + section in Task 4 |
| Reports: cancellations / no-show proxy / retention / referral | Tasks 3–4 (retention optional Task 5) |
| Admin: period + segments | Tasks 4–5 |

**Placeholder scan:** No `TBD` in executable steps; retention explicitly optional with concrete SQL sketch if enabled.

**Type consistency:** `AdminAnalyticsReport` signature change must be applied everywhere in one commit (Task 4).

---

## Execution handoff

**Plan complete and saved to `docs/superpowers/plans/2026-04-10-analytics-funnels-implementation.md`. Two execution options:**

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline execution** — run tasks in this session using executing-plans with checkpoints.

**Which approach?**

If Subagent-Driven: **REQUIRED SUB-SKILL:** superpowers:subagent-driven-development.  
If Inline: **REQUIRED SUB-SKILL:** superpowers:executing-plans.
