-- Level 3: personal account (balance, referrals, locale) and analytics events.

CREATE TABLE IF NOT EXISTS user_profiles (
    telegram_user_id BIGINT PRIMARY KEY,
    balance_cents BIGINT NOT NULL DEFAULT 500,
    referral_code TEXT NOT NULL UNIQUE,
    referred_by_telegram_id BIGINT,
    preferred_lang VARCHAR(5) NOT NULL DEFAULT 'ru',
    referral_reward_granted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_profiles_referred_by_fk
        FOREIGN KEY (referred_by_telegram_id) REFERENCES user_profiles (telegram_user_id)
        ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_user_profiles_referral_code ON user_profiles (referral_code);

CREATE TABLE IF NOT EXISTS analytics_events (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT,
    event_type TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_analytics_events_type_time ON analytics_events (event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_created ON analytics_events (created_at DESC);
