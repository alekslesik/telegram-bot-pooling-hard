package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
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
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clinic_bookings WHERE telegram_user_id = $1`, userID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) CancelClinicBooking(ctx context.Context, userID, bookingID int64) (ClinicBookingView, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ClinicBookingView{}, err
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
		return ClinicBookingView{}, ErrNotFound
	}
	if err != nil {
		return ClinicBookingView{}, err
	}

	if status != "cancelled" {
		if _, err = tx.ExecContext(ctx, `
			UPDATE clinic_bookings
			SET status = 'cancelled', cancelled_at = NOW()
			WHERE id = $1`, bookingID); err != nil {
			return ClinicBookingView{}, err
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE doctor_slots
			SET is_available = TRUE
			WHERE id = $1`, slotID); err != nil {
			return ClinicBookingView{}, err
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
		return ClinicBookingView{}, err
	}

	if err = tx.Commit(); err != nil {
		return ClinicBookingView{}, fmt.Errorf("commit cancel clinic booking tx: %w", err)
	}
	return item, nil
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
