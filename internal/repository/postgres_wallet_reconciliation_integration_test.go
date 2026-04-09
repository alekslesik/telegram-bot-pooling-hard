package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestPostgresRepositoryCountWalletBalanceMismatches(t *testing.T) {
	db := openReconciliationIntegrationDB(t)
	defer db.Close()
	prepareWalletReconciliationSchema(t, db)

	ctx := context.Background()
	repo := NewPostgresRepository(db)
	if err := seedWalletReconciliationData(ctx, db); err != nil {
		t.Fatalf("seed reconciliation data: %v", err)
	}

	got, err := repo.CountWalletBalanceMismatches(ctx)
	if err != nil {
		t.Fatalf("count mismatches error: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected 2 wallet mismatches, got %d", got)
	}
}

func openReconciliationIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN is not set; skipping postgres integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func prepareWalletReconciliationSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ddl := `
DROP TABLE IF EXISTS wallet_balance_read_model CASCADE;
DROP TABLE IF EXISTS wallet_transactions CASCADE;
DROP TABLE IF EXISTS outbox_events CASCADE;
DROP TABLE IF EXISTS user_profiles CASCADE;

CREATE TABLE user_profiles (
  telegram_user_id BIGINT PRIMARY KEY,
  balance_cents BIGINT NOT NULL,
  referral_code TEXT NOT NULL UNIQUE,
  preferred_lang TEXT NOT NULL DEFAULT 'ru',
  referral_reward_granted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallet_balance_read_model (
  telegram_user_id BIGINT PRIMARY KEY,
  balance_cents BIGINT NOT NULL,
  last_tx_id BIGINT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallet_transactions (
  id BIGSERIAL PRIMARY KEY,
  telegram_user_id BIGINT NOT NULL,
  operation_id TEXT NOT NULL UNIQUE,
  tx_type TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  balance_before BIGINT NOT NULL,
  balance_after BIGINT NOT NULL,
  related_booking_id BIGINT NULL,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("prepare schema: %v", err)
	}
}

func seedWalletReconciliationData(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO user_profiles (telegram_user_id, balance_cents, referral_code) VALUES
  (9001, 500, 'r9001'),
  (9002, 400, 'r9002'),
  (9003, 300, 'r9003');

INSERT INTO wallet_balance_read_model (telegram_user_id, balance_cents) VALUES
  (9001, 500),
  (9003, 300);

INSERT INTO wallet_transactions (
  telegram_user_id, operation_id, tx_type, amount_cents,
  balance_before, balance_after, related_booking_id, metadata_json
) VALUES (
  9003, 'op-reconcile-9003', 'debit', -100, 400, 200, NULL, '{}'::jsonb
);`)
	return err
}
