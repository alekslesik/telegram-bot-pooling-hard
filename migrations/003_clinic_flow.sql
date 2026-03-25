CREATE TABLE IF NOT EXISTS specialties (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    sort_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS doctors (
    id BIGSERIAL PRIMARY KEY,
    full_name TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS doctor_specialties (
    doctor_id BIGINT NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    specialty_id BIGINT NOT NULL REFERENCES specialties(id) ON DELETE CASCADE,
    PRIMARY KEY (doctor_id, specialty_id)
);

CREATE TABLE IF NOT EXISTS doctor_slots (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    specialty_id BIGINT NOT NULL REFERENCES specialties(id) ON DELETE CASCADE,
    start_at TIMESTAMPTZ NOT NULL,
    is_available BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (doctor_id, specialty_id, start_at)
);

CREATE TABLE IF NOT EXISTS clinic_bookings (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL,
    specialty_id BIGINT NOT NULL REFERENCES specialties(id),
    doctor_id BIGINT NOT NULL REFERENCES doctors(id),
    doctor_slot_id BIGINT NOT NULL REFERENCES doctor_slots(id),
    status TEXT NOT NULL DEFAULT 'confirmed',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cancelled_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_specialties_sort ON specialties(sort_order, id);
CREATE INDEX IF NOT EXISTS idx_doctor_slots_lookup ON doctor_slots(specialty_id, doctor_id, start_at);
CREATE INDEX IF NOT EXISTS idx_clinic_bookings_user ON clinic_bookings(telegram_user_id, created_at DESC);

INSERT INTO specialties (name, sort_order, is_active)
VALUES
    ('Терапевт', 1, TRUE),
    ('Кардиолог', 2, TRUE),
    ('ЛОР', 3, TRUE),
    ('Невролог', 4, TRUE)
ON CONFLICT DO NOTHING;

INSERT INTO doctors (full_name, is_active)
VALUES
    ('Иванов И.И.', TRUE),
    ('Петрова А.С.', TRUE),
    ('Смирнов Д.К.', TRUE)
ON CONFLICT DO NOTHING;

INSERT INTO doctor_specialties (doctor_id, specialty_id)
SELECT d.id, s.id
FROM doctors d
JOIN specialties s ON
    (d.full_name = 'Иванов И.И.' AND s.name IN ('Терапевт', 'Невролог')) OR
    (d.full_name = 'Петрова А.С.' AND s.name = 'Кардиолог') OR
    (d.full_name = 'Смирнов Д.К.' AND s.name IN ('Терапевт', 'ЛОР'))
ON CONFLICT DO NOTHING;

INSERT INTO doctor_slots (doctor_id, specialty_id, start_at, is_available)
SELECT d.id, s.id, NOW() + i.day_offset * INTERVAL '1 day' + i.hour_offset * INTERVAL '1 hour', TRUE
FROM (VALUES (1, 10), (2, 12), (3, 14)) AS i(day_offset, hour_offset)
JOIN doctors d ON d.full_name = 'Иванов И.И.'
JOIN specialties s ON s.name = 'Терапевт'
UNION ALL
SELECT d.id, s.id, NOW() + i.day_offset * INTERVAL '1 day' + i.hour_offset * INTERVAL '1 hour', TRUE
FROM (VALUES (1, 11), (2, 13), (3, 15)) AS i(day_offset, hour_offset)
JOIN doctors d ON d.full_name = 'Петрова А.С.'
JOIN specialties s ON s.name = 'Кардиолог'
UNION ALL
SELECT d.id, s.id, NOW() + i.day_offset * INTERVAL '1 day' + i.hour_offset * INTERVAL '1 hour', TRUE
FROM (VALUES (1, 16), (2, 17), (3, 18)) AS i(day_offset, hour_offset)
JOIN doctors d ON d.full_name = 'Смирнов Д.К.'
JOIN specialties s ON s.name = 'ЛОР';
