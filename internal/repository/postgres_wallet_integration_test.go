package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

const errSeedDataFmt = "seed data error: %v"

func TestPostgresWalletIntegrationConfirmPaidClinicBookingIdempotent(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()
	prepareWalletIntegrationSchema(t, db)

	ctx := context.Background()
	repo := NewPostgresRepository(db)
	seedWalletIntegrationData(t, db, time.Now().UTC().Add(48*time.Hour))

	first, err := repo.ConfirmPaidClinicBooking(ctx, 7001, 100, 1, 1, 1, "op-int-idem-7001")
	if err != nil {
		t.Fatalf("first confirm error: %v", err)
	}
	second, err := repo.ConfirmPaidClinicBooking(ctx, 7001, 100, 1, 1, 1, "op-int-idem-7001")
	if err != nil {
		t.Fatalf("second confirm error: %v", err)
	}
	if first.BookingID != second.BookingID {
		t.Fatalf("expected same booking id, got %d and %d", first.BookingID, second.BookingID)
	}
	if first.BalanceAfter != second.BalanceAfter {
		t.Fatalf("expected same balance after idempotent confirm, got %d and %d", first.BalanceAfter, second.BalanceAfter)
	}

	var txCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_transactions WHERE operation_id = 'op-int-idem-7001'`).Scan(&txCount); err != nil {
		t.Fatalf("count wallet tx error: %v", err)
	}
	if txCount != 1 {
		t.Fatalf("expected exactly one debit tx for idempotent op, got %d", txCount)
	}
}

func TestPostgresWalletIntegrationCancelClinicBookingRefundAndReadModel(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()
	prepareWalletIntegrationSchema(t, db)

	ctx := context.Background()
	repo := NewPostgresRepository(db)
	seedWalletIntegrationData(t, db, time.Now().UTC().Add(48*time.Hour))

	paid, err := repo.ConfirmPaidClinicBooking(ctx, 7001, 100, 1, 1, 1, "op-int-cancel-7001")
	if err != nil {
		t.Fatalf("confirm error: %v", err)
	}
	if paid.BalanceAfter != 900 {
		t.Fatalf("expected debited balance 900, got %d", paid.BalanceAfter)
	}

	cancelled, err := repo.CancelClinicBooking(ctx, 7001, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if !cancelled.RefundApplied || cancelled.RefundedCents != 100 {
		t.Fatalf("expected full refund 100, got applied=%v refunded=%d", cancelled.RefundApplied, cancelled.RefundedCents)
	}

	model, err := repo.GetWalletBalanceReadModel(ctx, 7001)
	if err != nil {
		t.Fatalf("read model fetch error: %v", err)
	}
	if model.BalanceCents != 1000 {
		t.Fatalf("expected read model balance restored to 1000, got %d", model.BalanceCents)
	}
}

func TestPostgresWalletIntegrationCancelClinicBookingAfterStartNoRefund(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()
	prepareWalletIntegrationSchema(t, db)

	ctx := context.Background()
	repo := NewPostgresRepository(db)
	seedWalletIntegrationData(t, db, time.Now().UTC().Add(-30*time.Minute))

	paid, err := repo.ConfirmPaidClinicBooking(ctx, 7001, 100, 1, 1, 1, "op-int-after-start-7001")
	if err != nil {
		t.Fatalf("confirm error: %v", err)
	}
	if paid.BalanceAfter != 900 {
		t.Fatalf("expected debited balance 900, got %d", paid.BalanceAfter)
	}

	cancelled, err := repo.CancelClinicBooking(ctx, 7001, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if cancelled.RefundApplied || cancelled.RefundedCents != 0 {
		t.Fatalf("expected no refund after slot start, got applied=%v refunded=%d", cancelled.RefundApplied, cancelled.RefundedCents)
	}

	var balance int64
	if err := db.QueryRowContext(ctx, `SELECT balance_cents FROM user_profiles WHERE telegram_user_id = 7001`).Scan(&balance); err != nil {
		t.Fatalf("select user balance error: %v", err)
	}
	if balance != 900 {
		t.Fatalf("expected balance to stay debited at 900, got %d", balance)
	}
}

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN is not set; skipping postgres integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql open error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("db ping error: %v", err)
	}
	return db
}

func prepareWalletIntegrationSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ddl := `
DROP TABLE IF EXISTS wallet_balance_read_model CASCADE;
DROP TABLE IF EXISTS wallet_transactions CASCADE;
DROP TABLE IF EXISTS outbox_events CASCADE;
DROP TABLE IF EXISTS clinic_bookings CASCADE;
DROP TABLE IF EXISTS doctor_slots CASCADE;
DROP TABLE IF EXISTS doctors CASCADE;
DROP TABLE IF EXISTS specialties CASCADE;
DROP TABLE IF EXISTS user_profiles CASCADE;

CREATE TABLE specialties (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 1,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE doctors (
  id BIGINT PRIMARY KEY,
  full_name TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE doctor_slots (
  id BIGINT PRIMARY KEY,
  doctor_id BIGINT NOT NULL,
  specialty_id BIGINT NOT NULL,
  start_at TIMESTAMPTZ NOT NULL,
  is_available BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE clinic_bookings (
  id BIGSERIAL PRIMARY KEY,
  telegram_user_id BIGINT NOT NULL,
  specialty_id BIGINT NOT NULL,
  doctor_id BIGINT NOT NULL,
  doctor_slot_id BIGINT NOT NULL,
  status TEXT NOT NULL DEFAULT 'confirmed',
  cancelled_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_profiles (
  telegram_user_id BIGINT PRIMARY KEY,
  balance_cents BIGINT NOT NULL DEFAULT 500,
  referral_code TEXT NOT NULL UNIQUE,
  referred_by_telegram_id BIGINT NULL,
  preferred_lang TEXT NOT NULL DEFAULT 'ru',
  referral_reward_granted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallet_transactions (
  id BIGSERIAL PRIMARY KEY,
  telegram_user_id BIGINT NOT NULL,
  operation_id TEXT NOT NULL UNIQUE,
  tx_type TEXT NOT NULL CHECK (tx_type IN ('debit', 'credit', 'refund', 'bonus')),
  amount_cents BIGINT NOT NULL,
  balance_before BIGINT NOT NULL,
  balance_after BIGINT NOT NULL,
  related_booking_id BIGINT NULL,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallet_balance_read_model (
  telegram_user_id BIGINT PRIMARY KEY,
  balance_cents BIGINT NOT NULL,
  last_tx_id BIGINT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("schema setup error: %v", err)
	}
}

func seedWalletIntegrationData(t *testing.T, db *sql.DB, slotStart time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO specialties (id, name, sort_order, is_active) VALUES (1, 'Therapy', 1, TRUE)`); err != nil {
		t.Fatalf(errSeedDataFmt, err)
	}
	if _, err := db.Exec(`INSERT INTO doctors (id, full_name, is_active) VALUES (1, 'Dr. Integration', TRUE)`); err != nil {
		t.Fatalf(errSeedDataFmt, err)
	}
	if _, err := db.Exec(`INSERT INTO doctor_slots (id, doctor_id, specialty_id, start_at, is_available) VALUES (1, 1, 1, $1, TRUE)`, slotStart); err != nil {
		t.Fatalf(errSeedDataFmt, err)
	}
	if _, err := db.Exec(`INSERT INTO user_profiles (telegram_user_id, balance_cents, referral_code, preferred_lang, referral_reward_granted) VALUES (7001, 1000, 'int7001', 'ru', FALSE)`); err != nil {
		t.Fatalf(errSeedDataFmt, err)
	}
}
