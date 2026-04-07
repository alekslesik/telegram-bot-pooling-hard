-- Hard roadmap phase 2: transactional outbox for async/background processing.

CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGSERIAL PRIMARY KEY,
    dedupe_key TEXT UNIQUE,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id BIGINT,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'done')),
    attempts INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending_available ON outbox_events (status, available_at, id);
CREATE INDEX IF NOT EXISTS idx_outbox_event_type_created ON outbox_events (event_type, created_at DESC);
