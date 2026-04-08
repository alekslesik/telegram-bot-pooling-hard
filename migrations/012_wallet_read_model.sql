-- Wallet read-model for fast admin/reporting reads.
-- Ledger (wallet_transactions) remains the source of truth.

CREATE TABLE IF NOT EXISTS wallet_balance_read_model (
    telegram_user_id BIGINT PRIMARY KEY REFERENCES user_profiles(telegram_user_id) ON DELETE CASCADE,
    balance_cents BIGINT NOT NULL,
    last_tx_id BIGINT REFERENCES wallet_transactions(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallet_balance_read_model_updated_at
    ON wallet_balance_read_model (updated_at DESC);
