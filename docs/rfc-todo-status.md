# RFC To-Do List — status snapshot

Last updated: 2026-04-10 (repo snapshot; re-verify when scope changes).

This file mirrors the user-maintained RFC checklist and records **implementation status** in this repository.

---

## 1) Wallet Ledger

| Item | Status |
|------|--------|
| Partial refund policy (percent / time windows) | **Done** — `ClinicBookingRefundPolicy`, env (`CLINIC_REFUND_*`), `calculateClinicBookingRefund` |
| Formalize policy model (config / table / per service) | **Done** — DB table `clinic_refund_policies` (+ audit log), runtime fallback order `specialty -> global -> env/default`; API methods allow global and per-specialty updates |
| Postgres integration tests (debit / refund / idempotency / read-model) | **Done** — e.g. wallet integration tests, PR #31 |
| UX signal “refund blocked by policy” + tests | **Done** — PR #29 |
| (Optional) reconcile `user_profiles` vs `wallet_balance_read_model` vs `wallet_transactions` | **Done** — admin/report metric (#33) |

**Open follow-up (optional):** add service-level policy overrides (current granularity is global + per-specialty).

---

## 2) Outbox + Worker

| Item | Status |
|------|--------|
| End-to-end `booking_created`, `payment_confirmed` | **Done** — paid booking path enqueues `booking_created` then `payment_confirmed` (same transaction); `CreateClinicBooking` enqueues `booking_created`; worker handles `payment_confirmed` + legacy `booking_confirmed` |
| Dead-letter contract + alerts | **Done** — terminal `failed` status, `outbox_event_dead` analytics, optional `OUTBOX_DEAD_LETTER_WEBHOOK`, see [outbox-dead-letter.md](./outbox-dead-letter.md) |
| Operational metrics (retry rate, oldest pending) | **Done** — `GetOutboxOperationalStats`: `outbox_oldest_pending_age_sec`, `outbox_pending_with_retries`, `outbox_sum_attempts_queued`; admin report also lists `outbox_failed` |

---

## 3) Analytics & funnels

| Item | Status |
|------|--------|
| Unified event schema across bot/service/worker | **Partial** |
| Funnel metrics (steps, conversion) | **Not done** (beyond basic events) |
| Reports: cancellations / no-show proxy / retention / referral | **Not done** |
| Admin reports: period + segments | **Not done** |

---

## 4) Admin v2 & roles

**Implementation plan (writing-plans):** [`docs/superpowers/plans/2026-04-10-admin-v2-roles-implementation.md`](superpowers/plans/2026-04-10-admin-v2-roles-implementation.md)
**Const:** `ready_section_4_admin_v2_roles = 100%`

| Item | Status |
|------|--------|
| Roles owner/admin/operator to full RFC scope | **Done** — role matrix + owner/admin/operator gates, owner admin-roster operations, bot capability-based visibility |
| Batch slot operations | **Done** — range generate/close/open flows with repository+service+bot coverage |
| Blackout / holiday rules | **Done** — blackout rules migration, enforcement in generation/listing/booking, add/list/deactivate admin flows |
| Extended audit trail for critical ops | **Done** — structured JSON details, deny/open/access logs, audit tail read path |

---

## 5) Reliability & operations

**Const:** `ready_section_5_reliability_operations = 100%`

| Item | Status |
|------|--------|
| Rate limiting + anti-abuse | **Done** — per-user limits, global limiter, and operator blocklist in dispatcher (`TELEGRAM_RATE_LIMIT_*`, `TELEGRAM_BLOCKLIST_USER_IDS`) |
| Idempotent handler ops for critical paths | **Done** — repo-layer money idempotency + transport dedup for `update_id`, callback ID, and chat/message ID |
| Health/readiness + structured logs + alerting | **Done** — `/healthz` + `/readyz`, structured logs, reliability alert webhook with retries/throttle, transition alerts (`readiness_degraded` / `readiness_recovered`) |
| Runbook (rollback, DB recovery, secret rotation) | **Done** — `docs/ops/RUNBOOK.md` |

---

## 6) Online payments

**Const:** `ready_section_6_online_payments = 100%`

| Item | Status |
|------|--------|
| Telegram Payments / Stars / external PSP | **Done** — Telegram Stars transport/service flow is implemented, and external PSP callback path is supported via provider abstraction and idempotent operation mapping |
| Reconciliation procedures for edge cases | **Done** — reconciliation query API + provider/operation filters and runbook procedures cover duplicate, missing-credit, and mismatch investigations |
| Resilience test/live scenarios for payments | **Done** — test suite covers live-like invoice/precheckout/success flows, malformed payloads, wrong currency/amount, retries, and repeated delivery idempotency |

**Implementation plan (writing-plans):** [docs/superpowers/plans/2026-04-10-online-payments-rfc-section-6.md](superpowers/plans/2026-04-10-online-payments-rfc-section-6.md) — Telegram Stars top-up first; PSP stub + reconciliation queries; integration tests for idempotent credit.

---

## Summary

- **Wallet Ledger (section 1)** is largely complete; main gap is **policy storage model** (DB / per service) if required.
- **Section 2 (Outbox + Worker)** is **complete** for the RFC checklist above.
- **Sections 3–6** are mostly **future work** or **partially** addressed.
