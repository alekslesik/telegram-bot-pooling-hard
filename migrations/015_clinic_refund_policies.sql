-- RFC section 1 hardening: persist refund policies in DB with optional per-specialty override.
-- Priority order at runtime: specialty override -> global DB policy -> app default/env fallback.

CREATE TABLE IF NOT EXISTS clinic_refund_policies (
    id BIGSERIAL PRIMARY KEY,
    specialty_id BIGINT REFERENCES specialties(id) ON DELETE CASCADE,
    partial_window_seconds BIGINT NOT NULL CHECK (partial_window_seconds > 0),
    partial_percent INT NOT NULL CHECK (partial_percent >= 0 AND partial_percent <= 100),
    reason TEXT NOT NULL DEFAULT '',
    updated_by_admin_user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- one policy per specialty (when specialty_id is set)
CREATE UNIQUE INDEX IF NOT EXISTS uq_clinic_refund_policies_specialty
    ON clinic_refund_policies (specialty_id)
    WHERE specialty_id IS NOT NULL;

-- at most one global policy row (specialty_id is NULL)
CREATE UNIQUE INDEX IF NOT EXISTS uq_clinic_refund_policies_global
    ON clinic_refund_policies ((specialty_id IS NULL))
    WHERE specialty_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_clinic_refund_policies_updated_at
    ON clinic_refund_policies (updated_at DESC);

CREATE TABLE IF NOT EXISTS clinic_refund_policy_audit_log (
    id BIGSERIAL PRIMARY KEY,
    specialty_id BIGINT REFERENCES specialties(id) ON DELETE CASCADE,
    partial_window_seconds BIGINT NOT NULL CHECK (partial_window_seconds > 0),
    partial_percent INT NOT NULL CHECK (partial_percent >= 0 AND partial_percent <= 100),
    reason TEXT NOT NULL DEFAULT '',
    updated_by_admin_user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clinic_refund_policy_audit_created_at
    ON clinic_refund_policy_audit_log (created_at DESC);
