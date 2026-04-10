# RFC To-Do List — status snapshot

Last updated: 2026-04-10 (repo snapshot; re-verify when scope changes).

This file mirrors the user-maintained RFC checklist and records **implementation status** in this repository.

---

## 1) Wallet Ledger

| Item | Status |
|------|--------|
| Partial refund policy (percent / time windows) | **Done** — `ClinicBookingRefundPolicy`, env (`CLINIC_REFUND_*`), `calculateClinicBookingRefund` |
| Formalize policy model (config / table / per service) | **Partial** — in code + env; **no** DB table or per-service policy |
| Postgres integration tests (debit / refund / idempotency / read-model) | **Done** — e.g. wallet integration tests, PR #31 |
| UX signal “refund blocked by policy” + tests | **Done** — PR #29 |
| (Optional) reconcile `user_profiles` vs `wallet_balance_read_model` vs `wallet_transactions` | **Done** — admin/report metric (#33) |

**Open follow-up (if still in scope):** persist policy in DB or per specialty/service.

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

| Item | Status |
|------|--------|
| Roles owner/admin/operator to full RFC scope | **Partial** — roles + `canManage*` helpers |
| Batch slot operations | **Not done** |
| Blackout / holiday rules | **Not done** |
| Extended audit trail for critical ops | **Partial** |

---

## 5) Reliability & operations

| Item | Status |
|------|--------|
| Rate limiting + anti-abuse | **Partial** — per-user Telegram limits via env `TELEGRAM_RATE_LIMIT_MSG_PER_MIN` / `TELEGRAM_RATE_LIMIT_CALLBACK_PER_MIN`; Redis-based update dedup in dispatcher; remaining gap: no broader abuse controls (e.g. global/IP-level throttling or operator blocklist flow) |
| Idempotent handler ops for critical paths | **Partial** — wallet/outbox idempotency in repository + Telegram `update_id` dedup at transport layer; duplicate updates are now explicitly logged with structured fields (`update_id`, `telegram_user_id`, `kind`); remaining gap: no unified idempotency contract across all critical handlers/channels |
| Health/readiness + structured logs + alerting | **Partial** — `/healthz` + `/readyz` implemented, health includes metadata (`version`, `commit`); dead-letter webhook failures now emit structured warning/error logs (request build/send/non-2xx), but external alert delivery/escalation automation is still not wired |
| Runbook (rollback, DB recovery, secret rotation) | **Done** — `docs/ops/RUNBOOK.md` covers rollback, DB recovery basics, secret rotation, and minimum alerting contract |

---

## 6) Online payments

| Item | Status |
|------|--------|
| Telegram Payments / Stars / external PSP | **Partial** — internal paid booking + ledger/outbox flow is in place (`ConfirmPaidClinicBooking`, `payment_confirmed`, `operation_id` idempotency), but Telegram Stars/`sendInvoice`/external PSP transport is not implemented |
| Reconciliation procedures for edge cases | **Partial** — mismatch detection exists (`CountWalletBalanceMismatches`) and reconciliation integration tests are present; manual operator runbook is documented in `docs/ops/RUNBOOK.md` |
| Resilience test/live scenarios for payments | **Partial** — memory+Postgres tests cover idempotency, insufficient funds, refund-after-start policy, and read-model consistency; no live Telegram payment scenario tests yet |

**Implementation plan (writing-plans):** [docs/superpowers/plans/2026-04-10-online-payments-rfc-section-6.md](superpowers/plans/2026-04-10-online-payments-rfc-section-6.md) — Telegram Stars top-up first; PSP stub + reconciliation queries; integration tests for idempotent credit.

---

## Summary

- **Wallet Ledger (section 1)** is largely complete; main gap is **policy storage model** (DB / per service) if required.
- **Section 2 (Outbox + Worker)** is **complete** for the RFC checklist above.
- **Sections 3–6** remain mostly **partial** with key gaps in payment transport and product analytics depth.
