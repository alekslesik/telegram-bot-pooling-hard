-- Grant admin access to Telegram user 892122714.

INSERT INTO admins (telegram_user_id, is_active)
VALUES (892122714, TRUE)
ON CONFLICT (telegram_user_id) DO UPDATE
SET is_active = TRUE,
    updated_at = NOW();
