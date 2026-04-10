# Reliability Alert Webhook

`RELIABILITY_ALERT_WEBHOOK` enables best-effort POST alerts for reliability-critical failures in bot runtime.

Set in environment:

```env
RELIABILITY_ALERT_WEBHOOK=https://hooks.example.com/reliability-alerts
```

## Payload format

The bot sends JSON with this shape:

```json
{
  "event": "dedup_check_error",
  "message": "telegram update dedup failed",
  "context": {
    "error": "dial tcp 127.0.0.1:6379: connect: connection refused",
    "dedup_key": "update:123456789",
    "update_id": 123456789
  }
}
```

Notes:
- `event` and `message` are always present.
- `context` is optional and event-specific.
- Delivery timeout is 2 seconds; non-2xx response is treated as failure.
- Delivery uses a short retry (up to 2 attempts) with small backoff.
- Same `event` is throttled in-process for 60 seconds to reduce alert storms.
- Failures to deliver are logged as warnings and do not stop bot processing.

## Events currently emitted

The following events are currently emitted by the runtime:

1. `dedup_check_error`
   - Trigger: update deduplication check failed.
   - Context fields: `error`, `dedup_key`, `update_id`.
2. `global_limiter_check_error`
   - Trigger: global rate-limit check failed.
   - Context fields: `error`, `telegram_user_id`, `kind`, `update_id`.
3. `health_server_startup_failure`
   - Trigger: health server failed to start.
   - Context fields: `error`, `addr`.
4. `readiness_degraded`
   - Trigger: readiness state transitioned from `ready` to `not_ready`.
   - Context fields: `status`, `checks`.
5. `readiness_recovered`
   - Trigger: readiness state transitioned from `not_ready` to `ready`.
   - Context fields: `status`, `checks`.

## Tuning notes

- Route by `event` first. Keep one alert rule per event to avoid noisy catch-all policies.
- Start with warning severity for `dedup_check_error` and `global_limiter_check_error`; escalate to critical only on sustained repetition.
- Treat `health_server_startup_failure` as critical immediately on production.
- Deduplicate in your alerting system by (`event`, key context field), for example:
  - (`dedup_check_error`, `context.dedup_key`)
  - (`global_limiter_check_error`, `context.telegram_user_id`)
- Use short evaluation windows (1-5 min) and require repeated hits before paging for non-startup events.
- Track delivery health by searching logs for `reliability alert delivery failed`.
