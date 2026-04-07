-- Outbox dedupe support: prevent duplicate logical events.

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS dedupe_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_outbox_events_dedupe_key
    ON outbox_events (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
