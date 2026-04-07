-- Level 3 hard roadmap (phase 1): wallet ledger with idempotent operation keys.

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL REFERENCES user_profiles(telegram_user_id) ON DELETE CASCADE,
    operation_id TEXT NOT NULL UNIQUE,
    tx_type TEXT NOT NULL CHECK (tx_type IN ('debit', 'credit', 'refund', 'bonus')),
    amount_cents BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    related_booking_id BIGINT REFERENCES clinic_bookings(id) ON DELETE SET NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallet_tx_user_created ON wallet_transactions (telegram_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_booking ON wallet_transactions (related_booking_id);
