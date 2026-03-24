package repository

import (
	"context"
	"database/sql"
	"errors"
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
