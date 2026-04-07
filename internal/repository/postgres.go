package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListActiveServices(ctx context.Context) ([]Service, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, duration_min, is_active
		FROM services
		WHERE is_active = TRUE
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.Name, &s.DurationMin, &s.IsActive); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetServiceByID(ctx context.Context, serviceID int64) (Service, error) {
	var s Service
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, duration_min, is_active
		FROM services
		WHERE id = $1`, serviceID).Scan(&s.ID, &s.Name, &s.DurationMin, &s.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) ListAvailableSlots(ctx context.Context, serviceID int64) ([]Slot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, service_id, start_at, is_available
		FROM slots
		WHERE service_id = $1 AND is_available = TRUE
		ORDER BY start_at ASC`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Slot
	for rows.Next() {
		var s Slot
		if err := rows.Scan(&s.ID, &s.ServiceID, &s.StartAt, &s.IsAvailable); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetSlotByID(ctx context.Context, slotID int64) (Slot, error) {
	var s Slot
	err := r.db.QueryRowContext(ctx, `
		SELECT id, service_id, start_at, is_available
		FROM slots
		WHERE id = $1`, slotID).Scan(&s.ID, &s.ServiceID, &s.StartAt, &s.IsAvailable)
	if errors.Is(err, sql.ErrNoRows) {
		return Slot{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) GetClientByUserID(ctx context.Context, userID int64) (Client, error) {
	var c Client
	err := r.db.QueryRowContext(ctx, `
		SELECT telegram_user_id, full_name, phone, created_at, updated_at
		FROM clients
		WHERE telegram_user_id = $1`, userID).
		Scan(&c.TelegramUserID, &c.FullName, &c.Phone, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	return c, err
}

func (r *PostgresRepository) UpsertClient(ctx context.Context, client Client) (Client, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO clients (telegram_user_id, full_name, phone)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_user_id) DO UPDATE
		SET full_name = EXCLUDED.full_name,
		    phone = EXCLUDED.phone,
		    updated_at = NOW()
		RETURNING telegram_user_id, full_name, phone, created_at, updated_at`,
		client.TelegramUserID, client.FullName, client.Phone,
	).Scan(&client.TelegramUserID, &client.FullName, &client.Phone, &client.CreatedAt, &client.UpdatedAt)
	return client, err
}

func (r *PostgresRepository) ListSpecialties(ctx context.Context, limit, offset int) ([]Specialty, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, sort_order, is_active
		FROM specialties
		WHERE is_active = TRUE
		ORDER BY sort_order ASC, id ASC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Specialty
	for rows.Next() {
		var s Specialty
		if err := rows.Scan(&s.ID, &s.Name, &s.SortOrder, &s.IsActive); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountSpecialties(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM specialties WHERE is_active = TRUE`).Scan(&count)
	return count, err
}

func (r *PostgresRepository) GetSpecialtyByID(ctx context.Context, specialtyID int64) (Specialty, error) {
	var s Specialty
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, sort_order, is_active
		FROM specialties
		WHERE id = $1`, specialtyID).
		Scan(&s.ID, &s.Name, &s.SortOrder, &s.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return Specialty{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) ListDoctorsBySpecialty(ctx context.Context, specialtyID int64, limit, offset int) ([]Doctor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.full_name, d.is_active
		FROM doctors d
		INNER JOIN doctor_specialties ds ON ds.doctor_id = d.id
		WHERE ds.specialty_id = $1 AND d.is_active = TRUE
		ORDER BY d.full_name ASC, d.id ASC
		LIMIT $2 OFFSET $3`, specialtyID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Doctor
	for rows.Next() {
		var d Doctor
		if err := rows.Scan(&d.ID, &d.FullName, &d.IsActive); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountDoctorsBySpecialty(ctx context.Context, specialtyID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM doctors d
		INNER JOIN doctor_specialties ds ON ds.doctor_id = d.id
		WHERE ds.specialty_id = $1 AND d.is_active = TRUE`, specialtyID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) GetDoctorByID(ctx context.Context, doctorID int64) (Doctor, error) {
	var d Doctor
	err := r.db.QueryRowContext(ctx, `
		SELECT id, full_name, is_active
		FROM doctors
		WHERE id = $1`, doctorID).Scan(&d.ID, &d.FullName, &d.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return Doctor{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresRepository) ListAvailableDoctorSlots(ctx context.Context, specialtyID, doctorID int64, limit, offset int) ([]DoctorSlot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, doctor_id, specialty_id, start_at, is_available
		FROM doctor_slots
		WHERE specialty_id = $1
		  AND doctor_id = $2
		  AND is_available = TRUE
		  AND start_at >= NOW()
		ORDER BY start_at ASC
		LIMIT $3 OFFSET $4`, specialtyID, doctorID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DoctorSlot
	for rows.Next() {
		var s DoctorSlot
		if err := rows.Scan(&s.ID, &s.DoctorID, &s.SpecialtyID, &s.StartAt, &s.IsAvailable); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountAvailableDoctorSlots(ctx context.Context, specialtyID, doctorID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM doctor_slots
		WHERE specialty_id = $1
		  AND doctor_id = $2
		  AND is_available = TRUE
		  AND start_at >= NOW()`, specialtyID, doctorID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) GetDoctorSlotByID(ctx context.Context, slotID int64) (DoctorSlot, error) {
	var s DoctorSlot
	err := r.db.QueryRowContext(ctx, `
		SELECT id, doctor_id, specialty_id, start_at, is_available
		FROM doctor_slots
		WHERE id = $1`, slotID).Scan(&s.ID, &s.DoctorID, &s.SpecialtyID, &s.StartAt, &s.IsAvailable)
	if errors.Is(err, sql.ErrNoRows) {
		return DoctorSlot{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) CreateBooking(ctx context.Context, booking Booking) (Booking, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO bookings (telegram_user_id, service_id, slot_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		booking.TelegramUserID,
		booking.ServiceID,
		booking.SlotID,
		booking.Status,
	).Scan(&booking.ID, &booking.CreatedAt)
	return booking, err
}

func (r *PostgresRepository) MarkSlotUnavailable(ctx context.Context, slotID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE slots
		SET is_available = FALSE
		WHERE id = $1 AND is_available = TRUE`, slotID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ConfirmServiceBooking(ctx context.Context, booking Booking) (Booking, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Booking{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE slots
		SET is_available = FALSE
		WHERE id = $1 AND is_available = TRUE`, booking.SlotID)
	if err != nil {
		return Booking{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Booking{}, err
	}
	if affected == 0 {
		return Booking{}, ErrNotFound
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO bookings (telegram_user_id, service_id, slot_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		booking.TelegramUserID,
		booking.ServiceID,
		booking.SlotID,
		booking.Status,
	).Scan(&booking.ID, &booking.CreatedAt)
	if err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return booking, nil
}

func (r *PostgresRepository) CreateClinicBooking(ctx context.Context, booking ClinicBooking) (ClinicBooking, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO clinic_bookings (telegram_user_id, specialty_id, doctor_id, doctor_slot_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		booking.TelegramUserID,
		booking.SpecialtyID,
		booking.DoctorID,
		booking.DoctorSlotID,
		booking.Status,
	).Scan(&booking.ID, &booking.CreatedAt)
	return booking, err
}

func (r *PostgresRepository) MarkDoctorSlotUnavailable(ctx context.Context, slotID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE doctor_slots
		SET is_available = FALSE
		WHERE id = $1 AND is_available = TRUE`, slotID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListUserClinicBookings(ctx context.Context, userID int64, limit, offset int) ([]ClinicBookingView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cb.id, s.name, d.full_name, ds.start_at, cb.status, cb.created_at
		FROM clinic_bookings cb
		INNER JOIN specialties s ON s.id = cb.specialty_id
		INNER JOIN doctors d ON d.id = cb.doctor_id
		INNER JOIN doctor_slots ds ON ds.id = cb.doctor_slot_id
		WHERE cb.telegram_user_id = $1
		  AND cb.status = 'confirmed'
		  AND ds.start_at >= NOW()
		ORDER BY ds.start_at ASC, cb.id ASC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClinicBookingView
	for rows.Next() {
		var item ClinicBookingView
		if err := rows.Scan(&item.ID, &item.SpecialtyName, &item.DoctorName, &item.StartAt, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountUserClinicBookings(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM clinic_bookings cb
		INNER JOIN doctor_slots ds ON ds.id = cb.doctor_slot_id
		WHERE cb.telegram_user_id = $1
		  AND cb.status = 'confirmed'
		  AND ds.start_at >= NOW()`, userID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) CancelClinicBooking(ctx context.Context, userID, bookingID int64) (CancelClinicBookingResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CancelClinicBookingResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var slotID int64
	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT doctor_slot_id, status
		FROM clinic_bookings
		WHERE id = $1 AND telegram_user_id = $2
		FOR UPDATE`, bookingID, userID).Scan(&slotID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return CancelClinicBookingResult{}, ErrNotFound
	}
	if err != nil {
		return CancelClinicBookingResult{}, err
	}

	var refunded int64
	var balanceAfter int64
	var refundApplied bool
	if status != "cancelled" {
		if _, err = tx.ExecContext(ctx, `
			UPDATE clinic_bookings
			SET status = 'cancelled', cancelled_at = NOW()
			WHERE id = $1`, bookingID); err != nil {
			return CancelClinicBookingResult{}, err
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE doctor_slots
			SET is_available = TRUE
			WHERE id = $1`, slotID); err != nil {
			return CancelClinicBookingResult{}, err
		}
		bookingIDCopy := bookingID
		if err = r.enqueueOutboxEventTx(ctx, tx, OutboxEvent{
			DedupeKey:     fmt.Sprintf("booking_cancelled:%d", bookingID),
			EventType:     "booking_cancelled",
			AggregateType: "clinic_booking",
			AggregateID:   &bookingIDCopy,
			PayloadJSON:   fmt.Sprintf(`{"booking_id":%d,"user_id":%d}`, bookingID, userID),
		}); err != nil {
			return CancelClinicBookingResult{}, err
		}

		var debitAmount int64
		err = tx.QueryRowContext(ctx, `
			SELECT amount_cents
			FROM wallet_transactions
			WHERE related_booking_id = $1
			  AND tx_type = 'debit'
			ORDER BY id DESC
			LIMIT 1`, bookingID).Scan(&debitAmount)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else if err != nil {
			return CancelClinicBookingResult{}, err
		}
		if debitAmount < 0 {
			refunded = -debitAmount
			refundOp := fmt.Sprintf("clinic_booking:refund:%d", bookingID)
			var existing int64
			err = tx.QueryRowContext(ctx, `
				SELECT id
				FROM wallet_transactions
				WHERE operation_id = $1
				LIMIT 1`, refundOp).Scan(&existing)
			if errors.Is(err, sql.ErrNoRows) {
				err = nil
			} else if err != nil {
				return CancelClinicBookingResult{}, err
			}
			if existing == 0 {
				var before int64
				err = tx.QueryRowContext(ctx, `
					SELECT balance_cents
					FROM user_profiles
					WHERE telegram_user_id = $1
					FOR UPDATE`, userID).Scan(&before)
				if errors.Is(err, sql.ErrNoRows) {
					return CancelClinicBookingResult{}, ErrNotFound
				}
				if err != nil {
					return CancelClinicBookingResult{}, err
				}
				err = tx.QueryRowContext(ctx, `
					UPDATE user_profiles
					SET balance_cents = balance_cents + $2, updated_at = NOW()
					WHERE telegram_user_id = $1
					RETURNING balance_cents`, userID, refunded).Scan(&balanceAfter)
				if err != nil {
					return CancelClinicBookingResult{}, err
				}
				_, err = tx.ExecContext(ctx, `
					INSERT INTO wallet_transactions (
						telegram_user_id, operation_id, tx_type, amount_cents,
						balance_before, balance_after, related_booking_id, metadata_json
					) VALUES ($1, $2, 'refund', $3, $4, $5, $6, '{}'::jsonb)`,
					userID, refundOp, refunded, before, balanceAfter, bookingID)
				if err != nil {
					return CancelClinicBookingResult{}, err
				}
				if err = r.enqueueOutboxEventTx(ctx, tx, OutboxEvent{
					DedupeKey:     fmt.Sprintf("booking_refunded:%d", bookingID),
					EventType:     "booking_refunded",
					AggregateType: "clinic_booking",
					AggregateID:   &bookingIDCopy,
					PayloadJSON:   fmt.Sprintf(`{"booking_id":%d,"user_id":%d,"refunded_cents":%d}`, bookingID, userID, refunded),
				}); err != nil {
					return CancelClinicBookingResult{}, err
				}
				refundApplied = true
			}
		}
	}

	var item ClinicBookingView
	err = tx.QueryRowContext(ctx, `
		SELECT cb.id, s.name, d.full_name, ds.start_at, cb.status, cb.created_at
		FROM clinic_bookings cb
		INNER JOIN specialties s ON s.id = cb.specialty_id
		INNER JOIN doctors d ON d.id = cb.doctor_id
		INNER JOIN doctor_slots ds ON ds.id = cb.doctor_slot_id
		WHERE cb.id = $1`, bookingID).
		Scan(&item.ID, &item.SpecialtyName, &item.DoctorName, &item.StartAt, &item.Status, &item.CreatedAt)
	if err != nil {
		return CancelClinicBookingResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return CancelClinicBookingResult{}, fmt.Errorf("commit cancel clinic booking tx: %w", err)
	}
	return CancelClinicBookingResult{
		Booking:       item,
		RefundedCents: refunded,
		BalanceAfter:  balanceAfter,
		RefundApplied: refundApplied,
	}, nil
}

func (r *PostgresRepository) SaveUserDocument(ctx context.Context, doc UserDocument) (UserDocument, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO user_documents (telegram_user_id, file_id, file_name, mime_type, file_size)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		doc.TelegramUserID, doc.FileID, doc.FileName, doc.MimeType, doc.FileSize).
		Scan(&doc.ID, &doc.CreatedAt)
	return doc, err
}

func (r *PostgresRepository) ListRecentUserDocuments(ctx context.Context, userID int64, limit int) ([]UserDocument, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, telegram_user_id, file_id, file_name, mime_type, file_size, created_at
		FROM user_documents
		WHERE telegram_user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserDocument
	for rows.Next() {
		var d UserDocument
		if err := rows.Scan(&d.ID, &d.TelegramUserID, &d.FileID, &d.FileName, &d.MimeType, &d.FileSize, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM admins
			WHERE telegram_user_id = $1 AND is_active = TRUE
		)`, userID).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) ListAllSpecialties(ctx context.Context) ([]Specialty, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, sort_order, is_active
		FROM specialties
		ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Specialty
	for rows.Next() {
		var s Specialty
		if err := rows.Scan(&s.ID, &s.Name, &s.SortOrder, &s.IsActive); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListAllDoctors(ctx context.Context) ([]Doctor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, full_name, is_active
		FROM doctors
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Doctor
	for rows.Next() {
		var d Doctor
		if err := rows.Scan(&d.ID, &d.FullName, &d.IsActive); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateSpecialty(ctx context.Context, name string, sortOrder int) (Specialty, error) {
	name = strings.TrimSpace(name)
	var s Specialty
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO specialties (name, sort_order, is_active)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (name) DO UPDATE
		SET sort_order = EXCLUDED.sort_order,
		    is_active = TRUE
		RETURNING id, name, sort_order, is_active`, name, sortOrder).
		Scan(&s.ID, &s.Name, &s.SortOrder, &s.IsActive)
	return s, err
}

func (r *PostgresRepository) CreateDoctor(ctx context.Context, fullName string) (Doctor, error) {
	fullName = strings.TrimSpace(fullName)
	var d Doctor
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO doctors (full_name, is_active)
		VALUES ($1, TRUE)
		ON CONFLICT (full_name) DO UPDATE
		SET is_active = TRUE
		RETURNING id, full_name, is_active`, fullName).
		Scan(&d.ID, &d.FullName, &d.IsActive)
	return d, err
}

func (r *PostgresRepository) LinkDoctorToSpecialty(ctx context.Context, doctorID, specialtyID int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO doctor_specialties (doctor_id, specialty_id)
		VALUES ($1, $2)
		ON CONFLICT (doctor_id, specialty_id) DO NOTHING`, doctorID, specialtyID)
	return err
}

func (r *PostgresRepository) GenerateDoctorSlots(ctx context.Context, doctorID, specialtyID int64, date time.Time, startMinute, endMinute, stepMinutes int) (int, error) {
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	startAt := base.Add(time.Duration(startMinute) * time.Minute)
	endAt := base.Add(time.Duration(endMinute) * time.Minute)

	// We treat endAt as exclusive (endMinute not included).
	var count int
	err := r.db.QueryRowContext(ctx, `
		WITH ins AS (
			INSERT INTO doctor_slots (doctor_id, specialty_id, start_at, is_available)
			SELECT $1, $2, gs, TRUE
			FROM generate_series(
				$3::timestamptz,
				$4::timestamptz - make_interval(mins => $5::int),
				make_interval(mins => $5::int)
			) gs
			ON CONFLICT (doctor_id, specialty_id, start_at) DO NOTHING
			RETURNING 1
		)
		SELECT COUNT(*) FROM ins
	`, doctorID, specialtyID, startAt, endAt, stepMinutes).Scan(&count)
	return count, err
}

func (r *PostgresRepository) CloseDoctorDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) (int, error) {
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	res, err := r.db.ExecContext(ctx, `
		UPDATE doctor_slots
		SET is_available = FALSE
		WHERE doctor_id = $1
		  AND specialty_id = $2
		  AND start_at >= $3
		  AND start_at < ($3 + INTERVAL '1 day')
	`, doctorID, specialtyID, base)
	if err != nil {
		return 0, err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(aff), nil
}

func (r *PostgresRepository) OpenDoctorDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) (int, error) {
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	res, err := r.db.ExecContext(ctx, `
		UPDATE doctor_slots ds
		SET is_available = TRUE
		WHERE ds.doctor_id = $1
		  AND ds.specialty_id = $2
		  AND ds.start_at >= $3
		  AND ds.start_at < ($3 + INTERVAL '1 day')
		  AND NOT EXISTS (
			SELECT 1
			FROM clinic_bookings cb
			WHERE cb.doctor_slot_id = ds.id
			  AND cb.status = 'confirmed'
		  )
	`, doctorID, specialtyID, base)
	if err != nil {
		return 0, err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(aff), nil
}

func (r *PostgresRepository) ListDoctorSlotsForDay(ctx context.Context, doctorID, specialtyID int64, date time.Time) ([]DoctorSlotDayView, error) {
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			ds.id,
			ds.start_at,
			ds.is_available,
			EXISTS (
				SELECT 1
				FROM clinic_bookings cb
				WHERE cb.doctor_slot_id = ds.id
				  AND cb.status = 'confirmed'
			) AS is_booked
		FROM doctor_slots ds
		WHERE ds.doctor_id = $1
		  AND ds.specialty_id = $2
		  AND ds.start_at >= $3
		  AND ds.start_at < ($3 + INTERVAL '1 day')
		ORDER BY ds.start_at ASC
	`, doctorID, specialtyID, base)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DoctorSlotDayView
	for rows.Next() {
		var v DoctorSlotDayView
		if err := rows.Scan(&v.ID, &v.StartAt, &v.IsAvailable, &v.IsBooked); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) LogAdminAction(ctx context.Context, adminUserID int64, action, details string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_audit_logs (admin_user_id, action, details)
		VALUES ($1, $2, $3)`, adminUserID, action, details)
	return err
}

func (r *PostgresRepository) GetConversationState(ctx context.Context, userID int64) (ConversationState, error) {
	var st ConversationState
	err := r.db.QueryRowContext(ctx, `
		SELECT telegram_user_id, state, payload_json, updated_at
		FROM conversation_states
		WHERE telegram_user_id = $1`, userID).Scan(&st.TelegramUserID, &st.State, &st.PayloadJSON, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationState{}, ErrNotFound
	}
	return st, err
}

func (r *PostgresRepository) SaveConversationState(ctx context.Context, state ConversationState) error {
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO conversation_states (telegram_user_id, state, payload_json, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (telegram_user_id) DO UPDATE
		SET state = EXCLUDED.state,
		    payload_json = EXCLUDED.payload_json,
		    updated_at = EXCLUDED.updated_at`,
		state.TelegramUserID, state.State, state.PayloadJSON, state.UpdatedAt,
	)
	return err
}

func (r *PostgresRepository) DeleteConversationState(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM conversation_states WHERE telegram_user_id = $1`, userID)
	return err
}

func (r *PostgresRepository) EnsureUserProfile(ctx context.Context, userID int64) (UserProfile, error) {
	if p, err := r.GetUserProfile(ctx, userID); err == nil {
		return p, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UserProfile{}, err
	}

	for i := 0; i < 12; i++ {
		code, err := randomReferralCode()
		if err != nil {
			return UserProfile{}, err
		}
		_, err = r.db.ExecContext(ctx, `
			INSERT INTO user_profiles (telegram_user_id, referral_code)
			VALUES ($1, $2)`, userID, code)
		if err == nil {
			return r.GetUserProfile(ctx, userID)
		}
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			continue
		}
		return UserProfile{}, err
	}
	return UserProfile{}, fmt.Errorf("could not allocate unique referral code")
}

func (r *PostgresRepository) GetUserProfile(ctx context.Context, userID int64) (UserProfile, error) {
	var p UserProfile
	var referred sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT telegram_user_id, balance_cents, referral_code, referred_by_telegram_id,
		       preferred_lang, referral_reward_granted, created_at, updated_at
		FROM user_profiles
		WHERE telegram_user_id = $1`, userID).
		Scan(&p.TelegramUserID, &p.BalanceCents, &p.ReferralCode, &referred,
			&p.PreferredLang, &p.ReferralRewardGranted, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UserProfile{}, ErrNotFound
	}
	if err != nil {
		return UserProfile{}, err
	}
	if referred.Valid {
		v := referred.Int64
		p.ReferredByTelegramID = &v
	}
	return p, nil
}

func (r *PostgresRepository) SetPreferredLang(ctx context.Context, userID int64, lang string) error {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang != "en" && lang != "ru" {
		lang = "ru"
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE user_profiles SET preferred_lang = $2, updated_at = NOW()
		WHERE telegram_user_id = $1`, userID, lang)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ApplyReferralCodeIfNew(ctx context.Context, userID int64, code string) error {
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return nil
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE user_profiles u
		SET referred_by_telegram_id = r.telegram_user_id, updated_at = NOW()
		FROM user_profiles r
		WHERE u.telegram_user_id = $1
		  AND u.referred_by_telegram_id IS NULL
		  AND r.referral_code = $2
		  AND r.telegram_user_id <> $1`, userID, code)
	if err != nil {
		return err
	}
	_, _ = res.RowsAffected()
	return nil
}

func (r *PostgresRepository) GrantReferralRewardsOnRegistration(ctx context.Context, userID, refereeBonusCents, referrerBonusCents int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var referred sql.NullInt64
	var granted bool
	err = tx.QueryRowContext(ctx, `
		SELECT referred_by_telegram_id, referral_reward_granted
		FROM user_profiles
		WHERE telegram_user_id = $1
		FOR UPDATE`, userID).Scan(&referred, &granted)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if granted || !referred.Valid {
		return tx.Commit()
	}
	refID := referred.Int64

	if _, err = tx.ExecContext(ctx, `
		UPDATE user_profiles
		SET balance_cents = balance_cents + $2,
		    referral_reward_granted = TRUE,
		    updated_at = NOW()
		WHERE telegram_user_id = $1`, userID, refereeBonusCents); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE user_profiles
		SET balance_cents = balance_cents + $2,
		    updated_at = NOW()
		WHERE telegram_user_id = $1`, refID, referrerBonusCents); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PostgresRepository) LogAnalyticsEvent(ctx context.Context, userID *int64, eventType, payloadJSON string) error {
	if strings.TrimSpace(payloadJSON) == "" {
		payloadJSON = "{}"
	}
	var uid any
	if userID != nil {
		uid = *userID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO analytics_events (telegram_user_id, event_type, payload_json)
		VALUES ($1, $2, $3::jsonb)`, uid, eventType, payloadJSON)
	return err
}

func (r *PostgresRepository) CountAnalyticsByEventSince(ctx context.Context, since time.Time) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT event_type, COUNT(*)
		FROM analytics_events
		WHERE created_at >= $1
		GROUP BY event_type`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var typ string
		var n int64
		if err := rows.Scan(&typ, &n); err != nil {
			return nil, err
		}
		out[typ] = n
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ConfirmPaidClinicBooking(ctx context.Context, userID, feeCents, specialtyID, doctorID, slotID int64, operationID string) (PaidBookingResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PaidBookingResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if strings.TrimSpace(operationID) == "" {
		operationID = fmt.Sprintf("clinic_booking:confirm:%d:%d", userID, slotID)
	}
	var existing WalletTransaction
	err = tx.QueryRowContext(ctx, `
		SELECT id, telegram_user_id, operation_id, tx_type, amount_cents, balance_before, balance_after,
		       related_booking_id, metadata_json, created_at
		FROM wallet_transactions
		WHERE operation_id = $1
		LIMIT 1`, operationID).
		Scan(&existing.ID, &existing.TelegramUserID, &existing.OperationID, &existing.TxType, &existing.AmountCents,
			&existing.BalanceBefore, &existing.BalanceAfter, &existing.RelatedBookingID, &existing.MetadataJSON, &existing.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PaidBookingResult{}, err
	}
	if err == nil && existing.TxType == "debit" && existing.RelatedBookingID != nil {
		var out PaidBookingResult
		err = tx.QueryRowContext(ctx, `
			SELECT cb.id, s.name, d.full_name, ds.start_at, cb.created_at
			FROM clinic_bookings cb
			INNER JOIN specialties s ON s.id = cb.specialty_id
			INNER JOIN doctors d ON d.id = cb.doctor_id
			INNER JOIN doctor_slots ds ON ds.id = cb.doctor_slot_id
			WHERE cb.id = $1`, *existing.RelatedBookingID).
			Scan(&out.BookingID, &out.SpecialtyName, &out.DoctorName, &out.SlotStart, &out.BookingCreated)
		if err != nil {
			return PaidBookingResult{}, err
		}
		out.BalanceAfter = existing.BalanceAfter
		if err = tx.Commit(); err != nil {
			return PaidBookingResult{}, err
		}
		return out, nil
	}

	var balance int64
	err = tx.QueryRowContext(ctx, `
		SELECT balance_cents FROM user_profiles WHERE telegram_user_id = $1 FOR UPDATE`, userID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
		return PaidBookingResult{}, err
	}
	if err != nil {
		return PaidBookingResult{}, err
	}
	if balance < feeCents {
		err = ErrInsufficientFunds
		return PaidBookingResult{}, err
	}

	var newBal int64
	balanceBefore := balance
	err = tx.QueryRowContext(ctx, `
		UPDATE user_profiles
		SET balance_cents = balance_cents - $2, updated_at = NOW()
		WHERE telegram_user_id = $1
		RETURNING balance_cents`, userID, feeCents).Scan(&newBal)
	if err != nil {
		return PaidBookingResult{}, err
	}

	var slotStart time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE doctor_slots
		SET is_available = FALSE
		WHERE id = $1 AND doctor_id = $2 AND specialty_id = $3 AND is_available = TRUE
		RETURNING start_at`, slotID, doctorID, specialtyID).Scan(&slotStart)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
		return PaidBookingResult{}, err
	}
	if err != nil {
		return PaidBookingResult{}, err
	}

	var bookingID int64
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO clinic_bookings (telegram_user_id, specialty_id, doctor_id, doctor_slot_id, status)
		VALUES ($1, $2, $3, $4, 'confirmed')
		RETURNING id, created_at`,
		userID, specialtyID, doctorID, slotID).Scan(&bookingID, &createdAt)
	if err != nil {
		return PaidBookingResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO wallet_transactions (
			telegram_user_id, operation_id, tx_type, amount_cents,
			balance_before, balance_after, related_booking_id, metadata_json
		) VALUES ($1, $2, 'debit', $3, $4, $5, $6, '{}'::jsonb)`,
		userID, operationID, -feeCents, balanceBefore, newBal, bookingID)
	if err != nil {
		return PaidBookingResult{}, err
	}
	bookingIDCopy := bookingID
	if err = r.enqueueOutboxEventTx(ctx, tx, OutboxEvent{
		DedupeKey:     fmt.Sprintf("booking_confirmed:%d", bookingID),
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		AggregateID:   &bookingIDCopy,
		PayloadJSON:   fmt.Sprintf(`{"booking_id":%d,"user_id":%d,"specialty_id":%d,"doctor_id":%d,"slot_id":%d}`, bookingID, userID, specialtyID, doctorID, slotID),
	}); err != nil {
		return PaidBookingResult{}, err
	}

	var specName, docName string
	err = tx.QueryRowContext(ctx, `
		SELECT s.name, d.full_name
		FROM specialties s, doctors d
		WHERE s.id = $1 AND d.id = $2`, specialtyID, doctorID).Scan(&specName, &docName)
	if err != nil {
		return PaidBookingResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return PaidBookingResult{}, err
	}

	return PaidBookingResult{
		BookingID:      bookingID,
		SpecialtyName:  specName,
		DoctorName:     docName,
		SlotStart:      slotStart,
		BalanceAfter:   newBal,
		BookingCreated: createdAt,
	}, nil
}

func (r *PostgresRepository) EnqueueOutboxEvent(ctx context.Context, event OutboxEvent) (OutboxEvent, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO outbox_events (dedupe_key, event_type, aggregate_type, aggregate_id, payload_json, status, attempts, available_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, 'pending', 0, COALESCE($6, NOW()))
		ON CONFLICT (dedupe_key) DO UPDATE
		SET dedupe_key = EXCLUDED.dedupe_key
		RETURNING id, dedupe_key, status, attempts, available_at, created_at, updated_at`,
		nullIfBlank(event.DedupeKey), event.EventType, event.AggregateType, event.AggregateID, coalesceJSON(event.PayloadJSON), nullIfZeroTime(event.AvailableAt)).
		Scan(&event.ID, &event.DedupeKey, &event.Status, &event.Attempts, &event.AvailableAt, &event.CreatedAt, &event.UpdatedAt)
	if err != nil {
		return OutboxEvent{}, err
	}
	event.PayloadJSON = coalesceJSON(event.PayloadJSON)
	return event, nil
}

func (r *PostgresRepository) ClaimDueOutboxEvents(ctx context.Context, limit int, now time.Time) ([]OutboxEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH picked AS (
			SELECT id
			FROM outbox_events
			WHERE status = 'pending'
			  AND available_at <= $1
			ORDER BY id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE outbox_events o
		SET status = 'processing',
		    attempts = o.attempts + 1,
		    locked_at = NOW(),
		    updated_at = NOW()
		FROM picked
		WHERE o.id = picked.id
		RETURNING o.id, o.dedupe_key, o.event_type, o.aggregate_type, o.aggregate_id, o.payload_json::text,
		          o.status, o.attempts, o.available_at, o.locked_at, o.processed_at, o.last_error, o.created_at, o.updated_at`,
		now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var ev OutboxEvent
		if err := rows.Scan(&ev.ID, &ev.DedupeKey, &ev.EventType, &ev.AggregateType, &ev.AggregateID, &ev.PayloadJSON,
			&ev.Status, &ev.Attempts, &ev.AvailableAt, &ev.LockedAt, &ev.ProcessedAt, &ev.LastError, &ev.CreatedAt, &ev.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MarkOutboxEventDone(ctx context.Context, eventID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET status = 'done',
		    processed_at = NOW(),
		    locked_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`, eventID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkOutboxEventFailed(ctx context.Context, eventID int64, lastError string, nextAttemptAt time.Time) error {
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC().Add(30 * time.Second)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET status = 'pending',
		    locked_at = NULL,
		    last_error = $2,
		    available_at = $3,
		    updated_at = NOW()
		WHERE id = $1`, eventID, strings.TrimSpace(lastError), nextAttemptAt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountOutboxByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM outbox_events
		GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{
		"pending":    0,
		"processing": 0,
		"done":       0,
	}
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

func (r *PostgresRepository) enqueueOutboxEventTx(ctx context.Context, tx *sql.Tx, event OutboxEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (dedupe_key, event_type, aggregate_type, aggregate_id, payload_json, status, attempts, available_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, 'pending', 0, COALESCE($6, NOW()))
		ON CONFLICT (dedupe_key) DO NOTHING`,
		nullIfBlank(event.DedupeKey), event.EventType, event.AggregateType, event.AggregateID, coalesceJSON(event.PayloadJSON), nullIfZeroTime(event.AvailableAt))
	return err
}

func nullIfZeroTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func coalesceJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func nullIfBlank(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return raw
}
