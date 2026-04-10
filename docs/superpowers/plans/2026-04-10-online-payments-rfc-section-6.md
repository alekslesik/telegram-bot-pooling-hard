# Online payments (RFC §6) implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Telegram Stars top-up as the first online payment path: users pay in Stars, the bot credits the existing wallet ledger in kopecks idempotently by Telegram’s payment charge id, with `pre_checkout` validation and Postgres integration tests for duplicate delivery and reconciliation queries.

**Architecture:** Keep `transport -> service -> repository`. Booking confirmation stays `ConfirmPaidClinicBooking` (wallet debit). Stars only **credit** `user_profiles.balance_cents` and append a `wallet_transactions` row with `tx_type = 'credit'` and `operation_id = tg_stars:<TelegramPaymentChargeID>`. Bot transport gains handlers for `PreCheckoutQuery` and `Message.SuccessfulPayment` (wired from `cmd/bot/main.go`). Classic Telegram Payments (fiat PSP) and non-Telegram PSPs are **out of scope for this plan** except a small `internal/payments.Provider` interface stub so a second provider can be added without rewriting the service.

**Tech Stack:** Go 1.26, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, PostgreSQL, existing migrations `007_wallet_ledger.sql` (no new migration if `credit` is already allowed in `tx_type` CHECK), `go test`, Docker Compose for integration tests.

---

## File structure

| Path | Responsibility |
|------|----------------|
| `internal/repository/repository.go` | `StarsTopUpResult`, `ApplyTelegramStarsTopUp(...)`, interface method on `Repository` |
| `internal/repository/postgres.go` | Transactional credit + ledger row + read-model upsert; idempotent return on existing `operation_id` |
| `internal/repository/repository.go` (`MemoryRepository`) | Same semantics for unit/service tests |
| `internal/repository/level3_memory_test.go` (or new `stars_topup_memory_test.go`) | Idempotency + balance tests |
| `internal/service/payments.go` | `PaymentService`: payload encode/decode, pre-checkout checks, call repository |
| `internal/service/payments_test.go` | Table-driven payload / validation tests |
| `internal/bot/handlers.go` | `HandlePreCheckout`, `HandleSuccessfulPayment`; optional top-up keyboard when balance insufficient |
| `internal/bot/handlers_test.go` | Golden-path / reject pre-checkout with mocked `TelegramClient` + service |
| `cmd/bot/main.go` | Route `update.PreCheckoutQuery` before messages; construct `PaymentService` from env |
| `internal/i18n/i18n.go` | User strings for top-up CTA and payment errors |
| `internal/repository/postgres_stars_topup_integration_test.go` | Duplicate charge id, happy path credit |
| `internal/payments/provider.go` | `Provider` interface + `NoopPSP` for future external reconciliation |

Env (document in `README` or existing env example if the repo has one — only if a task below touches it):

- `ONLINE_PAYMENT_MODE` — `off` \| `stars` (default `off`)
- `STARS_KOPEKS_PER_STAR` — int64, e.g. `100` means 1 Star credits 100 kopecks to wallet

**Invoice payload contract (must stay ≤ 128 bytes):** JSON `{"v":1,"u":<telegram_user_id>,"s":<stars>}` with no spaces. `s` is the number of Stars in the invoice (matches `TotalAmount` for currency `XTR`).

---

### Task 1: Ledger credit API (repository)

**Files:**

- Modify: `internal/repository/repository.go`
- Modify: `internal/repository/postgres.go`
- Modify: `internal/repository/repository.go` (`MemoryRepository` implementation block)
- Test: new tests next to memory repo tests

- [ ] **Step 1: Write the failing memory test for idempotent Stars top-up**

```go
func TestMemoryRepository_ApplyTelegramStarsTopUp_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	userID := int64(9001)
	_, _ = repo.EnsureUserProfile(ctx, userID)

	chargeID := "tg-charge-test-1"
	stars := int64(10)
	kopeksPerStar := int64(100)
	first, err := repo.ApplyTelegramStarsTopUp(ctx, userID, stars, kopeksPerStar, chargeID, `{"provider":"telegram_stars"}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ApplyTelegramStarsTopUp(ctx, userID, stars, kopeksPerStar, chargeID, `{"provider":"telegram_stars"}`)
	if err != nil {
		t.Fatal(err)
	}
	if first.BalanceAfter != second.BalanceAfter {
		t.Fatalf("balance drift on replay: first=%d second=%d", first.BalanceAfter, second.BalanceAfter)
	}
	if !second.AlreadyApplied {
		t.Fatal("expected AlreadyApplied on second call")
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./internal/repository -run ApplyTelegramStarsTopUp_Idempotent -v`  
Expected: FAIL (undefined `ApplyTelegramStarsTopUp` or missing method).

- [ ] **Step 3: Add types and interface method**

In `repository.go` (near other wallet types):

```go
type StarsTopUpResult struct {
	BalanceAfter   int64
	CreditedCents  int64
	AlreadyApplied bool
}
```

Add to `Repository` interface:

```go
ApplyTelegramStarsTopUp(ctx context.Context, userID, starsCount, kopeksPerStar int64, telegramPaymentChargeID, metadataJSON string) (StarsTopUpResult, error)
```

- [ ] **Step 4: Implement Postgres in one transaction**

Pattern: `operation_id := "tg_stars:" + telegramPaymentChargeID`. If a row exists with that `operation_id`, load user balance and return `AlreadyApplied: true` with `CreditedCents` from stored row. Else `FOR UPDATE` profile, compute `credited := starsCount * kopeksPerStar`, update balance, `INSERT` `wallet_transactions` with `tx_type = 'credit'`, `amount_cents = credited`, `related_booking_id = NULL`, `metadata_json = metadataJSON`, then `upsertWalletBalanceReadModelTx` like refund path.

- [ ] **Step 5: Implement MemoryRepository** mirroring the same rules (map by `operation_id`).

- [ ] **Step 6: Run tests**

Run: `go test ./internal/repository -run ApplyTelegramStarsTopUp -v`  
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/
git commit -m "feat(wallet): idempotent Telegram Stars top-up credit"
```

---

### Task 2: Payment service (payload + validation)

**Files:**

- Create: `internal/service/payments.go`
- Create: `internal/service/payments_test.go`
- Modify: `cmd/bot/main.go` (constructor only in Task 4; here no main change if you inject via tests)

- [ ] **Step 1: Write failing unit test for payload round-trip**

```go
func TestPaymentPayloadV1_RoundTrip(t *testing.T) {
	p := PaymentPayloadV1{Version: 1, UserID: 42, Stars: 25}
	s, err := EncodePaymentPayloadV1(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePaymentPayloadV1(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("got %+v want %+v", got, p)
	}
	if len(s) > 128 {
		t.Fatalf("payload too long: %d", len(s))
	}
}
```

- [ ] **Step 2: Run test — FAIL**

Run: `go test ./internal/service -run PaymentPayloadV1_RoundTrip -v`

- [ ] **Step 3: Implement `PaymentService`** with dependencies `repository.Repository`, `kopeksPerStar int64`, and methods:

  - `BuildTopUpInvoicePayload(userID, stars int64) (string, error)` — delegates to `EncodePaymentPayloadV1`
  - `ValidatePreCheckout(fromUserID int64, currency string, totalStars int64, payload string) error` — reject wrong currency (`XTR` only), decode payload, match `fromUserID` and `totalStars`
  - `ApplySuccessfulPayment(fromUserID int64, sp *tgbotapi.SuccessfulPayment) (repository.StarsTopUpResult, error)` — use `sp.TelegramPaymentChargeID`, `sp.TotalAmount`, validate payload again, then `ApplyTelegramStarsTopUp`

- [ ] **Step 4: Run service tests**

Run: `go test ./internal/service -run Payment -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/payments.go internal/service/payments_test.go
git commit -m "feat(bot): payment payload and Stars validation service"
```

---

### Task 3: Bot handlers + i18n

**Files:**

- Modify: `internal/bot/handlers.go`
- Modify: `internal/bot/handlers_test.go`
- Modify: `internal/i18n/i18n.go`

- [ ] **Step 1: Write failing handler test for pre-checkout rejection (wrong user)**

Build a `Handlers` with mocked `PaymentService` interface **or** real service with memory repo; call `HandlePreCheckout` with `PreCheckoutQuery` where payload encodes another user id; assert bot receives `AnswerPreCheckoutQuery` with `OK: false`.

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/bot -run PreCheckout -v`

- [ ] **Step 3: Implement `HandlePreCheckout`**

Use `h.Bot.Request(tgbotapi.PreCheckoutConfig{...})` with `OK: true/false` and `ErrorMessage` on failure (per telegram-bot-api v5).

- [ ] **Step 4: Implement `HandleSuccessfulPayment`**

On success, send user a short confirmation including new balance; log errors; call `LogAnalytics` with `stars_topup` / payload JSON if you already use `LogAnalytics` elsewhere.

- [ ] **Step 5: Extend insufficient-funds path** in `handleBookingCallback` `case "slot":`: when `ONLINE_PAYMENT_MODE=stars` and `PaymentService` non-nil, append inline button “⭐ Top up (Stars)” that triggers a **new** callback `pay:stars:pack:<n>` which sends invoice via `Send(tgbotapi.NewInvoice(...))` with `Currency: "XTR"`, prices `[]tgbotapi.LabeledPrice{{"Top up", stars}}`, `Payload` from `BuildTopUpInvoicePayload`.

- [ ] **Step 6: Add i18n helpers** e.g. `StarsTopUpPrompt`, `StarsTopUpDone`, `PaymentFailed`.

- [ ] **Step 7: Run bot tests**

Run: `go test ./internal/bot -v`

- [ ] **Step 8: Commit**

```bash
git add internal/bot/ internal/i18n/i18n.go
git commit -m "feat(bot): Stars invoice, pre_checkout, successful_payment"
```

---

### Task 4: Wire updates in `main`

**Files:**

- Modify: `cmd/bot/main.go`

- [ ] **Step 1: Extend `applyTelegramUpdate`**

```go
func applyTelegramUpdate(h *bot.Handlers, u tgbotapi.Update) {
	if u.PreCheckoutQuery != nil {
		h.HandlePreCheckout(u.PreCheckoutQuery)
		return
	}
	// existing callback / message routing
}
```

- [ ] **Step 2: On `Message.SuccessfulPayment != nil`**, route to `HandleSuccessfulPayment` before generic text handling (in `HandleMessage` or in `applyTelegramUpdate`).

- [ ] **Step 3: Build `PaymentService` from env** when mode is `stars`; pass into `Handlers` (add field `Payments *service.PaymentService`).

- [ ] **Step 4: Run**

Run: `go test ./...`

- [ ] **Step 5: Commit**

```bash
git add cmd/bot/main.go
git commit -m "feat(bot): route pre_checkout and successful_payment"
```

---

### Task 5: Postgres integration tests (resilience)

**Files:**

- Create: `internal/repository/postgres_stars_topup_integration_test.go` (same build tag pattern as existing `postgres_wallet_reconciliation_integration_test.go` if any)

- [ ] **Step 1: Test happy path** — credit balance and one `wallet_transactions` row with `tx_type = credit`.

- [ ] **Step 2: Test duplicate** — call `ApplyTelegramStarsTopUp` twice with same charge id; assert single row and stable balance.

- [ ] **Step 3: Run**

Run: `go test ./internal/repository -tags integration -run StarsTopUp -v` (adjust tag to match repo convention).

- [ ] **Step 4: Commit**

```bash
git add internal/repository/postgres_stars_topup_integration_test.go
git commit -m "test(wallet): Stars top-up idempotency integration"
```

---

### Task 6: Reconciliation (edge cases)

**Files:**

- Create: `internal/payments/provider.go` — `type Provider interface { Name() string }` and `NoopPSP` (document that Stripe/YooKassa implement later).
- Modify: `internal/repository/repository.go` + `postgres.go` — read-only query method optional: `ListWalletCreditsByProviderSince(ctx, providerPrefix string, since time.Time)`.

- [ ] **Step 1: Add SQL-backed listing** for ops starting with `tg_stars:` (or filter `metadata_json->>'provider' = 'telegram_stars'`) for ops review.

- [ ] **Step 2: Document in plan operations checklist** (runbook-style, no new doc file unless the repo already documents ops):

  1. **User charged, no credit:** search `wallet_transactions` by `operation_id = 'tg_stars:' || :charge_id`; if missing, call `getStarTransactions` (Bot API) or support export; manually insert credit only with a **new** `operation_id` suffix `manual:` after incident id (or replay `successful_payment` in test chat).
  2. **Double delivery:** idempotent `operation_id` must show `AlreadyApplied`; no second insert.
  3. **Wrong amount vs payload:** `pre_checkout` should have rejected; if passed due to bug, add compensating **manual** adjustment with audit log entry via existing admin tools if available.

- [ ] **Step 3: Add unit test** for list method or a simple `COUNT(*)` query test in integration file.

- [ ] **Step 4: Commit**

```bash
git add internal/payments/ internal/repository/
git commit -m "feat(payments): provider stub and Stars reconciliation query"
```

---

## Self-review

**Spec coverage (RFC §6):**

| Requirement | Task |
|-------------|------|
| Telegram Stars | Tasks 1–5 |
| Telegram Payments / external PSP | Stub only in Task 6; fiat PSP needs provider token + `currency` ≠ `XTR` — follow-up plan |
| Reconciliation edge cases | Task 6 |
| Resilience / live scenarios | Tasks 1, 5 (duplicate charge), 3 (pre-checkout reject) |

**Placeholder scan:** No TBD steps; amounts are parameterized via env.

**Type consistency:** `StarsTopUpResult` and `operation_id` prefix `tg_stars:` used consistently in repo + service.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-10-online-payments-rfc-section-6.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — run tasks in this session using executing-plans with checkpoints.

Which approach do you want?
