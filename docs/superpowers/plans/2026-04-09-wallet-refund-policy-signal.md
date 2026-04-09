# Wallet Refund Policy Signal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose a clear "refund blocked by policy" signal in cancellation flow without changing current money logic.

**Architecture:** Keep refund decision logic in repository; return explicit policy flag in cancellation result DTO; map this flag to a user-facing message in service layer. Preserve existing debit/refund/idempotency behavior.

**Tech Stack:** Go, repository pattern (memory + postgres), Telegram bot service layer tests.

---

### Task 1: Extend cancel result contract with policy signal

**Files:**
- Modify: `internal/repository/repository.go`
- Test: `internal/repository/level3_memory_test.go`

- [ ] **Step 1: Add result flag to DTO**

```go
type CancelClinicBookingResult struct {
    Booking                ClinicBookingView
    RefundedCents          int64
    BalanceAfter           int64
    RefundApplied          bool
    RefundBlockedByPolicy  bool
}
```

- [ ] **Step 2: Set flag in memory cancel flow**

```go
hadDebit := false
// detect related debit as before
if debitAmountFound {
    hadDebit = true
}
if hadDebit && !now.Before(slotStart) {
    result.RefundBlockedByPolicy = true
}
```

- [ ] **Step 3: Run focused repository tests**

Run: `go test ./internal/repository -run CancelClinicBooking -v`  
Expected: PASS

- [ ] **Step 4: Extend memory tests for policy flag**

```go
func TestMemoryRepositoryCancelClinicBookingAfterStartSetsPolicyFlag(t *testing.T) {
    // Arrange paid booking with slot in the past
    // Act cancel
    // Assert: RefundApplied == false, RefundBlockedByPolicy == true
}
```

- [ ] **Step 5: Re-run repository package**

Run: `go test ./internal/repository -v`  
Expected: PASS

### Task 2: Mirror policy signal in postgres cancel flow

**Files:**
- Modify: `internal/repository/postgres.go`
- Test: `internal/repository/level3_memory_test.go` (contract-level parity only in this PR)

- [ ] **Step 1: Track policy-block condition in postgres cancellation**

```go
// inside CancelClinicBooking
var refundBlockedByPolicy bool
// when debit exists but slot already started:
refundBlockedByPolicy = true
```

- [ ] **Step 2: Return flag in result**

```go
return CancelClinicBookingResult{
    Booking:               item,
    RefundedCents:         refunded,
    BalanceAfter:          balanceAfter,
    RefundApplied:         refundApplied,
    RefundBlockedByPolicy: refundBlockedByPolicy,
}, nil
```

- [ ] **Step 3: Run repository tests**

Run: `go test ./internal/repository -v`  
Expected: PASS

### Task 3: Add user-facing message in service cancel response

**Files:**
- Modify: `internal/service/booking.go`
- Modify: `internal/service/booking_test.go`

- [ ] **Step 1: Append policy line when refund is blocked**

```go
if result.RefundBlockedByPolicy {
    msg += "\nВозврат недоступен: время приема уже началось."
}
```

- [ ] **Step 2: Add service test for policy message**

```go
func TestBookingServiceCancelClinicBookingPolicyBlockedMessage(t *testing.T) {
    // Arrange booking in past with debit
    // Act cancel via service
    // Assert message contains "Возврат недоступен"
}
```

- [ ] **Step 3: Run focused service tests**

Run: `go test ./internal/service -run CancelClinicBooking -v`  
Expected: PASS

- [ ] **Step 4: Run full suite**

Run: `go test ./...`  
Expected: PASS

### Task 4: Validate runtime and ship PR

**Files:**
- Modify: none

- [ ] **Step 1: Run preprod checks**

Run: `make preprod`  
Expected: all checks pass

- [ ] **Step 2: Runtime smoke**

Run: `make docker-compose-up`  
Verify: app starts, no wallet-related startup errors

- [ ] **Step 3: Teardown**

Run: `make docker-compose-down`  
Expected: containers stopped and removed

- [ ] **Step 4: Commit and open PR**

```bash
git add internal/repository/repository.go internal/repository/postgres.go internal/repository/level3_memory_test.go internal/service/booking.go internal/service/booking_test.go docs/superpowers/plans/2026-04-09-wallet-refund-policy-signal.md
git commit -m "feat(wallet): surface refund policy blocked result"
git push -u origin HEAD
gh pr create ...
```
