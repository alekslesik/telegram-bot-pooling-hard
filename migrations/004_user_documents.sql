CREATE TABLE IF NOT EXISTS user_documents (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL,
    file_id TEXT NOT NULL,
    file_name TEXT,
    mime_type TEXT,
    file_size INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_documents_user_created
    ON user_documents(telegram_user_id, created_at DESC);
