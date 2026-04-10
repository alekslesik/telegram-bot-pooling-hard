# Reliability & Operations (RFC §5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close gaps from `docs/rfc-todo-status.md` §5: per-user rate limiting and basic anti-abuse, transport-level idempotency for Telegram update retries, tighter readiness/observability defaults, and an operator runbook (rollback, DB recovery, secret rotation).

**Architecture:** Keep `transport -> service -> repository`. Add a small `internal/telegramguard` package for limits and update deduplication, backed by Redis when `REDIS_ADDR` is set. **Degrade gracefully:** if Redis is nil or limits env is `0`/unset, **disable** rate limiting and emit one `Info` log at startup (`"telegram rate limits disabled"` with reason); do not use a process-local map for production abuse protection (misleading on multi-instance deploys). Extend `internal/cache.Redis` only with primitives the limiter needs. Enrich `slog` usage in `internal/bot` and `cmd/bot` with `telegram_user_id`, `update_id`, `callback_id` where available. Keep money idempotency in repository (`operation_id` on wallet txs); transport dedup protects non-idempotent side effects (e.g. duplicate analytics, double slot attempts before DB lock).

**Tech Stack:** Go 1.26, `slog`, Redis (`github.com/redis/go-redis/v9`), existing `internal/health` (`/healthz`, `/readyz`), Docker Compose healthchecks, `make preprod`.

---

## File map (expected touch points)

| Area | Files |
|------|--------|
| Redis primitives | `internal/cache/redis.go` |
| Rate limit + dedup | New: `internal/telegramguard/limiter.go`, `internal/telegramguard/dedup.go`, `internal/telegramguard/guard.go`, tests alongside |
| Bot wiring | `cmd/bot/main.go` (`applyTelegramUpdate` or wrapper), optionally `internal/bot/handlers.go` for log fields |
| Health | `internal/health/server.go`, `internal/health/handlers_test.go` |
| Docs | New: `docs/ops/RUNBOOK.md` (operators); optional: `.env.example` entries for new env vars |

---

### Task 1: Redis atomic increment with TTL (sliding window helper)

**Files:**
- Modify: `internal/cache/redis.go`
- Create: `internal/cache/redis_incr_ttl_test.go`

- [ ] **Step 0: Test dependency**

Run: `go get github.com/alicebob/miniredis/v2`  
Expected: `go.mod` / `go.sum` updated.

- [ ] **Step 1: Write failing test**

Add `redis_incr_ttl_test.go` using **`github.com/alicebob/miniredis/v2`** (new test-only dependency in `go.mod`) so `go test ./internal/cache -short` exercises real Redis protocol without Docker:

```go
func TestRedis_IncrWithTTL_SetsExpiryOnFirstIncrement(t *testing.T) {
    mr, err := miniredis.Run()
    if err != nil { t.Fatal(err) }
    defer mr.Close()
    r := newRedisWithAddr(t, mr.Addr()) // same package: build *Redis from go-redis client to mr.Addr()
    ctx := context.Background()
    n, err := r.IncrWithTTL(ctx, "rl:test:k", 60)
    if err != nil || n != 1 { t.Fatalf("first incr: n=%d err=%v", n, err) }
    if mr.TTL("rl:test:k") <= 0 { t.Fatal("expected TTL set on first increment") }
    n2, err := r.IncrWithTTL(ctx, "rl:test:k", 60)
    if err != nil || n2 != 2 { t.Fatalf("second incr: n=%d err=%v", n2, err) }
}
```

Implement `newRedisWithAddr` in `redis_incr_ttl_test.go` as a test helper that constructs the unexported `*Redis` (same `package cache`) wrapping `redis.NewClient(&redis.Options{Addr: addr})`.

Concrete implementation: add `IncrWithTTL` using one Lua script for atomicity:

```lua
local v = redis.call("INCR", KEYS[1])
if v == 1 then redis.call("EXPIRE", KEYS[1], ARGV[1]) end
return v
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/cache -v -short`  
Expected: FAIL (methods/script missing).

- [ ] **Step 3: Implement `IncrWithTTL` (Lua) on `*Redis`**

```go
func (r *Redis) IncrWithTTL(ctx context.Context, key string, ttlSeconds int) (int64, error) {
    if r == nil || r.c == nil {
        return 0, errors.New("redis: nil client")
    }
    const script = `...`
    // redis.NewScript + r.c.EvalSha or Eval
}
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/cache -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/redis.go internal/cache/redis_incr_ttl_test.go go.mod go.sum
git commit -m "feat(cache): add Redis incr with TTL for rate limiting"
```

---

### Task 2: Per-user message/callback rate limiter (configurable)

**Files:**
- Create: `internal/telegramguard/limiter.go`
- Create: `internal/telegramguard/limiter_test.go`
- Modify: `.env.example` (document `TELEGRAM_RATE_LIMIT_*` vars)

- [ ] **Step 1: Write failing tests**

```go
func TestLimiter_AllowsUnderThreshold(t *testing.T) {}
func TestLimiter_BlocksOverThreshold(t *testing.T) {}
```

Use a `LimiterStore` interface:

```go
type LimiterStore interface {
    IncrWithTTL(ctx context.Context, key string, ttlSeconds int) (int64, error)
}

type fakeStore struct { counts map[string]int64 }
func (f *fakeStore) IncrWithTTL(ctx context.Context, key string, ttlSeconds int) (int64, error) {
    f.counts[key]++
    return f.counts[key], nil
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/telegramguard -v`  
Expected: FAIL.

- [ ] **Step 3: Implement `Limiter`**

```go
type Limiter struct {
    store LimiterStore
    maxPerWindow int64
    windowSec    int
    keyPrefix    string // e.g. "tg:rl"
}

func (l *Limiter) Allow(ctx context.Context, telegramUserID int64, kind string) (bool, error) {
    key := fmt.Sprintf("%s:%s:%d", l.keyPrefix, kind, telegramUserID)
    n, err := l.store.IncrWithTTL(ctx, key, l.windowSec)
    if err != nil {
        return false, err
    }
    return n <= l.maxPerWindow, nil
}
```

Env parsing in `cmd/bot` or `telegramguard`:

- `TELEGRAM_RATE_LIMIT_MSG_PER_MIN` (default: 0 = disabled)
- `TELEGRAM_RATE_LIMIT_CALLBACK_PER_MIN` (default: 0 = disabled)

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/telegramguard -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/telegramguard/ .env.example
git commit -m "feat(bot): add configurable per-user Telegram rate limits"
```

---

### Task 3: Wire limiter into update loop + user-visible behavior

**Files:**
- Modify: `cmd/bot/main.go`
- Create: `cmd/bot/main_rate_limit_test.go` (optional: extract `applyTelegramUpdate` to testable function)

- [ ] **Step 1: Build `*telegramguard.Limiter` when Redis and env enabled**

After `redisCache, err := cache.NewRedisFromEnv()`, if non-nil and limits enabled, wrap Redis as `LimiterStore` (adapter type in `cmd/bot` or `telegramguard`).

- [ ] **Step 2: On `Allow == false`, log and skip or send short reply**

```go
logger.Warn("rate limited",
    "telegram_user_id", uid,
    "kind", "message",
)
// Option A: return without sending (silent drop)
// Option B: one-line reply once per window (harder; needs second key) — YAGNI: silent drop + log
```

Apply inside a new function:

```go
func dispatchUpdate(logger *slog.Logger, h *bot.Handlers, lim *telegramguard.Limiter, u tgbotapi.Update) {
    // derive user id from Message or CallbackQuery.From
    // lim.Allow for message vs callback
    // applyTelegramUpdate(h, u)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/bot -v`  
Expected: PASS.

- [ ] **Step 4: Run preprod**

Run: `make preprod`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/bot/main.go cmd/bot/main_rate_limit_test.go
git commit -m "feat(bot): enforce Telegram rate limits in update dispatcher"
```

---

### Task 4: Update-ID deduplication (Telegram retries)

**Files:**
- Create: `internal/telegramguard/dedup.go`
- Create: `internal/telegramguard/dedup_test.go`
- Modify: `cmd/bot/main.go`

- [ ] **Step 1: Write failing test**

```go
func TestDedup_FirstSeenReturnsFalseSecondReturnsTrue(t *testing.T) {
    // fake store with SetNX semantic
}
```

Implement `Seen(ctx, updateID int) (duplicate bool, err error)` using Redis `SET key NX EX 86400` where `key = fmt.Sprintf("tg:upd:%d", updateID)`. If Redis nil, return `duplicate=false` (no-op) and log once at startup.

- [ ] **Step 2: Integrate before `dispatchUpdate`**

If duplicate, `return` immediately after debug log.

- [ ] **Step 3: Run**

Run: `go test ./...`  
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/telegramguard/dedup.go internal/telegramguard/dedup_test.go cmd/bot/main.go
git commit -m "fix(bot): ignore duplicate Telegram updates by update_id"
```

---

### Task 5: Structured logs and readiness hardening

**Files:**
- Modify: `internal/bot/handlers.go` (add attrs to `Error` logs where `msg` / callback present)
- Modify: `internal/health/server.go`
- Modify: `internal/health/handlers_test.go`

- [ ] **Step 1: Add `version` / `commit` to `/healthz` JSON** (optional second endpoint `/version` to avoid changing health probe semantics)

Prefer new fields on existing response only if Docker healthcheck still matches; `wget /healthz` only checks HTTP 200, so adding JSON fields is safe:

```go
type healthResponse struct {
    Status  string `json:"status"`
    Version string `json:"version,omitempty"`
    Commit  string `json:"commit,omitempty"`
}
```

Pass `version`, `commit` from `main` into `health.NewServer` (extend constructor).

- [ ] **Step 2: Handler logs**

Example patch pattern:

```go
h.Logger.Error("booking flow failed",
    "err", err,
    "telegram_user_id", telegramUserID(msg),
    "update_id", msg.MessageID, // note: MessageID != UpdateID — pass UpdateID from outer layer if possible
)
```

Best: pass `updateID int` into handlers from `main` (signature change) **or** add unexported field on `Handlers` set per update (not thread-safe if concurrent—**do not** use shared field). Correct approach: thread `updateID` through `dispatchUpdate` → `HandleMessageWithContext(updateID, ...)` — plan this as a focused refactor in this task.

Minimal YAGNI: only `cmd/bot` logs `update_id` once per received update at debug level:

```go
logger.Debug("update", "update_id", u.UpdateID, "telegram_user_id", uid)
```

- [ ] **Step 3: Tests**

Run: `go test ./internal/health ./cmd/bot -v`  
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/health/server.go internal/health/handlers_test.go cmd/bot/main.go internal/bot/handlers.go
git commit -m "feat(ops): enrich health payload and structured logs for incidents"
```

---

### Task 6: Operator runbook + alerting notes

**Files:**
- Create: `docs/ops/RUNBOOK.md`
- Modify: `README.md` (one short link under Deployment if needed — optional)

- [ ] **Step 1: Write runbook sections**

1. **Rollback:** redeploy previous `IMAGE_TAG` via `deploy.yml` or compose on VPS; verify `/readyz`.
2. **DB recovery:** Postgres volume backup expectations; `migrations/` order; never run ad-hoc against prod without snapshot.
3. **Secret rotation:** rotate `TOKEN`, `secrets/postgres_password`, GitHub `VPS_POSTGRES_PASSWORD`; restart stack; verify bot auth and DB ping.
4. **Alerting (contract):** monitor HTTP `GET /readyz` → non-200; monitor container restart count; optional: log `rate limited` / `outbox` errors in JSON (`LOG_FORMAT=json`) shipped to collector. No external vendor lock-in—list placeholders for Uptime Kuma / Prometheus blackbox.

- [ ] **Step 2: Run preprod** (docs-only still run gate per repo rules)

Run: `make preprod`  
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add docs/ops/RUNBOOK.md
git commit -m "docs(ops): add runbook for rollback, DB, secrets, and alerts"
```

---

## Self-review (plan quality)

1. **Spec coverage (RFC §5 rows):** Rate limiting → Tasks 1–3. Idempotent handler ops → Task 4 + existing repo `operation_id` (no change required unless audit finds gaps). Health/logs/alerting → Task 5 + §6 of runbook. Runbook → Task 6.
2. **Placeholder scan:** No `TBD` steps; env defaults must be chosen explicitly in implementation (document in `.env.example`).
3. **Consistency:** `LimiterStore` matches new Redis method names from Task 1; health constructor signature change propagated to `main` and tests.

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-10-reliability-operations-rfc-section-5.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
