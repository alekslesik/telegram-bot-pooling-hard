CREATE TABLE IF NOT EXISTS schedule_blackout_rules (
    id BIGSERIAL PRIMARY KEY,
    scope TEXT NOT NULL CHECK (scope IN ('global', 'doctor_specialty')),
    kind  TEXT NOT NULL CHECK (kind IN ('blackout', 'holiday')),
    doctor_id BIGINT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    specialty_id BIGINT NULL REFERENCES specialties(id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at),
    CHECK (
        (scope = 'global' AND doctor_id IS NULL AND specialty_id IS NULL)
        OR
        (scope = 'doctor_specialty' AND doctor_id IS NOT NULL AND specialty_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_blackout_active_global_range
    ON schedule_blackout_rules (starts_at, ends_at)
    WHERE is_active = TRUE AND scope = 'global';

CREATE INDEX IF NOT EXISTS idx_blackout_active_doctor_spec_range
    ON schedule_blackout_rules (doctor_id, specialty_id, starts_at, ends_at)
    WHERE is_active = TRUE AND scope = 'doctor_specialty';
