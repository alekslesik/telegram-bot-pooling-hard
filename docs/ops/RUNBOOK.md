# Operations Runbook

Practical operator procedures for production incidents and routine maintenance.
This runbook assumes the deployment layout and flows from `README.md`:
- app path like `/opt/bots/telegram-bot-pooling-hard`
- deploy via GitHub Actions (`Release`/`Deploy`)
- runtime via `docker-compose.prod.yaml`

## 1) Rollback: redeploy previous image tag

Use this when the current release is unhealthy after deploy.

### Fast path (recommended): GitHub Actions Deploy workflow

1. Identify the last known-good image tag (example: `v1.0.2`).
2. In GitHub: `Actions -> Deploy -> Run workflow`.
3. Set input `IMAGE_TAG` to the known-good tag.
4. Run workflow and wait for completion.

### Verification on VPS (required)

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml pull
docker compose -f docker-compose.prod.yaml up -d
docker compose -f docker-compose.prod.yaml ps
docker compose -f docker-compose.prod.yaml logs --since=10m bot
```

Check readiness endpoint from host:

```bash
curl -fsS http://127.0.0.1:8080/readyz
```

Success criteria:
- `bot` and `postgres` containers are `Up` in `docker compose ... ps`
- `/readyz` returns HTTP `200`
- bot logs in last 10 minutes do not show crash loop/panic pattern

If verification fails, redeploy an older known-good tag and repeat checks.

## 2) DB recovery basics (PostgreSQL)

Goal: recover data safely without making the incident worse.

### Precautions (do before any restore)

1. Freeze writes:
   - temporarily stop bot traffic (or stop bot container).
2. Preserve evidence:
   - save current logs and incident timestamp.
3. Keep a snapshot of current DB state before restore (even if damaged).

### Minimal backup/restore flow (inside VPS)

Create safety dump first:

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml exec -T postgres \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > pre_restore_$(date +%F_%H%M%S).sql
```

Restore from known-good dump file (`backup.sql`):

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml exec -T postgres \
  psql -U "$POSTGRES_USER" "$POSTGRES_DB" < backup.sql
```

Post-restore checks:

```bash
docker compose -f docker-compose.prod.yaml logs --since=10m postgres
curl -fsS http://127.0.0.1:8080/readyz
```

Notes:
- Prefer restoring to a maintenance window if data volume is large.
- For destructive recovery, keep the pre-restore dump until incident closure.

## 3) Secret rotation

Rotate one secret at a time, validate, then continue with next.

### A) `TOKEN` (Telegram bot token)

1. Generate new token in BotFather.
2. Update production `.env` (`TOKEN=...`) on VPS at app path.
3. Redeploy current image tag via `Deploy` workflow (or restart service):

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml up -d
curl -fsS http://127.0.0.1:8080/readyz
```

4. Revoke old token in BotFather.

### B) Postgres password file (`secrets/postgres_password`)

1. Generate new DB password.
2. Update GitHub secret `VPS_POSTGRES_PASSWORD`.
3. Run `Deploy` workflow for current stable tag (workflow rewrites `secrets/postgres_password` on VPS).
4. Verify:
   - `docker compose ... ps` is healthy
   - `/readyz` returns `200`
   - no DB auth errors in bot/postgres logs

Important: ensure DB user password and `VPS_POSTGRES_PASSWORD` stay in sync.

### C) GitHub secret (generic procedure)

1. In GitHub: `Settings -> Secrets and variables -> Actions`.
2. Update target secret value (examples: `VPS_SSH_KEY`, `GHCR_READ_TOKEN`, `VPS_POSTGRES_PASSWORD`).
3. Trigger the workflow that consumes it (`Deploy` or `Release`).
4. Confirm workflow succeeds and production readiness is still green.

## 4) Alerting contract (minimum)

Alert sources should be simple and operationally actionable.

### Readiness (`/readyz`)

- Probe: `GET /readyz` every 30s
- Alert: 3 consecutive failures or HTTP != 200 for 2 minutes
- Severity: `critical` if sustained > 5 minutes

### Container restarts

- Signal: bot container restart count delta > 0 in 10 minutes
- Alert: immediate `warning`; escalate to `critical` if restart loop continues for 15 minutes
- Suggested command:

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml ps
docker inspect --format='{{.Name}} {{.RestartCount}}' $(docker compose -f docker-compose.prod.yaml ps -q bot)
```

### Logs

Track bot and postgres logs for:
- `panic`, `fatal`, `connection refused`, `password authentication failed`
- migration failures
- repeated `readyz` failures

Suggested triage command:

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml logs --since=15m bot postgres
```

### Operator response SLO

- Acknowledge `critical` alert within 10 minutes.
- Start rollback decision within 15 minutes if `/readyz` stays red after first mitigation.

## 5) Payments reconciliation edge cases

Use this when payment state and user balance diverge.

### A) Charge succeeded, but credit/booking is missing

Symptoms:
- Payment operation is known, but there is no corresponding booking or balance update.

Checks:

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml exec -T postgres psql -U "$POSTGRES_USER" "$POSTGRES_DB" -c "
SELECT id, telegram_user_id, operation_id, tx_type, amount_cents, related_booking_id, created_at
FROM wallet_transactions
WHERE operation_id = '<operation_id>';
"
```

Actions:
1. If a `debit` exists with expected `operation_id`, do not re-charge; treat retries as idempotent.
2. If no debit exists, collect evidence (user id, operation id, timestamps, logs) and escalate for manual correction script/replay.
3. Verify user balance and read-model after correction.

### B) Duplicate updates / duplicate payment callbacks

Symptoms:
- Same payment/update arrives multiple times.

Actions:
1. Confirm only one ledger row exists per `operation_id`.
2. Confirm only one business booking is linked to that operation.
3. If duplicates were dropped, keep logs and mark incident as idempotency-protected.

### C) Wallet/read-model mismatch checks

Checks:

```bash
cd /opt/bots/telegram-bot-pooling-hard
docker compose -f docker-compose.prod.yaml exec -T postgres psql -U "$POSTGRES_USER" "$POSTGRES_DB" -c "
SELECT COUNT(*) AS wallet_mismatches
FROM user_profiles up
LEFT JOIN wallet_balance_read_model wrm ON wrm.telegram_user_id = up.telegram_user_id
LEFT JOIN LATERAL (
  SELECT wt.balance_after
  FROM wallet_transactions wt
  WHERE wt.telegram_user_id = up.telegram_user_id
  ORDER BY wt.id DESC
  LIMIT 1
) last_tx ON TRUE
WHERE wrm.telegram_user_id IS NULL
   OR up.balance_cents <> wrm.balance_cents
   OR (last_tx.balance_after IS NOT NULL AND up.balance_cents <> last_tx.balance_after);
"
```

Actions:
1. If mismatch count is non-zero, extract affected users and freeze manual adjustments until root cause is clear.
2. Rebuild or upsert read-model rows from latest ledger balances for affected users.
3. Re-run mismatch query and close incident only when count returns to zero.
