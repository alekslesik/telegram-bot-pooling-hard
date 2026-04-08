# Outbox Schema Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make outbox delivery resilient on fresh databases by auto-bootstrapping `outbox_events` schema when missing, so worker and booking flows keep working without manual migration ordering issues.

**Architecture:** Keep `transport -> service -> repository`. Add guarded schema bootstrap in Postgres repository: detect undefined `outbox_events` relation, create schema once, and retry operation. Do not change business flow in service layer.

**Tech Stack:** Go, PostgreSQL (`lib/pq`), repository layer tests, existing Makefile checks.

---

### Task 1: Add failing tests for missing outbox relation detection

**Files:**
- Create: `internal/repository/postgres_outbox_schema_test.go`
- Modify: `internal/repository/postgres.go`
- Test: `internal/repository/postgres_outbox_schema_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestIsUndefinedOutboxRelationError_MatchesOutboxEvents(t *testing.T) {}
func TestIsUndefinedOutboxRelationError_IgnoresOtherCases(t *testing.T) {}
```

- [ ] **Step 2: Run targeted test**

Run: `go test ./internal/repository -run UndefinedOutboxRelation -v`  
Expected: FAIL (helper missing).

- [ ] **Step 3: Implement helper**

```go
func isUndefinedOutboxRelationError(err error) bool {
    // true only for pq error code 42P01 and relation outbox_events
}
```

- [ ] **Step 4: Re-run targeted test**

Run: `go test ./internal/repository -run UndefinedOutboxRelation -v`  
Expected: PASS.

### Task 2: Bootstrap outbox schema and retry on first use

**Files:**
- Modify: `internal/repository/postgres.go`
- Test: `internal/repository/postgres_outbox_schema_test.go`

- [ ] **Step 1: Write failing test for create SQL presence**

```go
func TestEnsureOutboxSchemaSQL_ContainsRequiredObjects(t *testing.T) {}
```

- [ ] **Step 2: Run targeted test**

Run: `go test ./internal/repository -run EnsureOutboxSchemaSQL -v`  
Expected: FAIL.

- [ ] **Step 3: Implement bootstrap SQL and guarded ensure**

```go
func outboxBootstrapSQL() string { /* CREATE TABLE IF NOT EXISTS outbox_events ... */ }
func (r *PostgresRepository) ensureOutboxSchema(ctx context.Context) error { /* once + exec */ }
```

- [ ] **Step 4: Integrate retries in outbox repository methods**

```go
// EnqueueOutboxEvent + ClaimDueOutboxEvents:
// on undefined-relation error -> ensureOutboxSchema -> retry once
```

- [ ] **Step 5: Protect booking tx entry points**

```go
// ConfirmPaidClinicBooking / CancelClinicBooking:
// call ensureOutboxSchema(ctx) before BeginTx
```

- [ ] **Step 6: Re-run repository tests**

Run: `go test ./internal/repository -v`  
Expected: PASS.

### Task 3: Full verification and integration

**Files:**
- Modify (if needed): `README.md` (only if behavior note is needed)

- [ ] **Step 1: Run full unit/integration suite**

Run: `go test ./...`  
Expected: PASS.

- [ ] **Step 2: Run required preprod gate**

Run: `make preprod`  
Expected: PASS.

- [ ] **Step 3: Run runtime verification**

Run: `make docker-compose-up`  
Expected: stack starts; outbox worker does not fail due to missing `outbox_events`.

- [ ] **Step 4: Cleanup containers**

Run: `make docker-compose-down`

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres.go internal/repository/postgres_outbox_schema_test.go docs/superpowers/plans/2026-04-08-outbox-schema-bootstrap.md
git commit -m "fix(outbox): bootstrap schema when relation is missing"
```
