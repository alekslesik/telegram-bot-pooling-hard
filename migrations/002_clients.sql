CREATE TABLE IF NOT EXISTS clients (
    telegram_user_id BIGINT PRIMARY KEY,
    full_name TEXT NOT NULL,
    phone TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
