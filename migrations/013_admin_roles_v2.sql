ALTER TABLE admins
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'admin';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'admins_role_check'
    ) THEN
        ALTER TABLE admins
            ADD CONSTRAINT admins_role_check
            CHECK (role IN ('owner', 'admin', 'operator'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_admins_role_active
    ON admins(role)
    WHERE is_active = TRUE;

UPDATE admins
SET role = 'admin'
WHERE role IS NULL OR role = '';
