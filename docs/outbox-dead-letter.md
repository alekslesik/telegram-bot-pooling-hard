# Outbox dead-letter contract (RFC §2)

## Terminal state

When the outbox worker exhausts retries (`attempts >= maxAttempts`, default **5**), the row is moved to terminal status **`failed`** via `MarkOutboxEventDead`.

## Observability

1. **Analytics** — one row is written to `analytics_events` with `event_type = outbox_event_dead` and JSON payload including `event_id`, `event_type`, `attempts`.
2. **Optional webhook** — if `OUTBOX_DEAD_LETTER_WEBHOOK` is set, the process POSTs JSON to that URL after the row is marked failed:

```json
{
  "event_id": 123,
  "event_type": "booking_reminder_due",
  "dedupe_key": "booking_reminder_due:42",
  "last_error": "downstream error message"
}
```

Use this for PagerDuty, Slack incoming webhooks, or internal alerting. The HTTP client timeout is **5 seconds**; failures are not retried from the bot (avoid feedback loops).

## Backward compatibility

Older outbox rows may still use `event_type = booking_confirmed`; the worker treats `booking_confirmed` and `payment_confirmed` identically for reminder scheduling.
