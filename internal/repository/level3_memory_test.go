package repository

import (
	"context"
	"testing"
)

func TestMemoryRepository_ConfirmPaidClinicBooking_Insufficient(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 99

	p, err := repo.EnsureUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	repo.mu.Lock()
	p.BalanceCents = 10
	repo.userProfiles[userID] = p
	repo.mu.Unlock()

	_, err = repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-insufficient-1")
	if err != ErrInsufficientFunds {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
}

func TestMemoryRepository_ConfirmPaidClinicBooking_IdempotentByOperationID(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 201

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	first, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-unique-201")
	if err != nil {
		t.Fatalf("first booking error: %v", err)
	}
	second, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-unique-201")
	if err != nil {
		t.Fatalf("second booking error: %v", err)
	}

	if first.BookingID != second.BookingID {
		t.Fatalf("expected same booking id for idempotent op, got %d and %d", first.BookingID, second.BookingID)
	}
	if first.BalanceAfter != second.BalanceAfter {
		t.Fatalf("expected same balance after idempotent op, got %d and %d", first.BalanceAfter, second.BalanceAfter)
	}
}

func TestMemoryRepository_CancelClinicBooking_RefundsOnce(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 301

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}
	p0, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-cancel-301")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	res1, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel #1 error: %v", err)
	}
	if !res1.RefundApplied || res1.RefundedCents != 100 {
		t.Fatalf("expected one-time refund of 100, got applied=%v refunded=%d", res1.RefundApplied, res1.RefundedCents)
	}

	p1, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if p1.BalanceCents != p0.BalanceCents {
		t.Fatalf("expected balance restored to %d, got %d", p0.BalanceCents, p1.BalanceCents)
	}

	res2, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel #2 error: %v", err)
	}
	if res2.RefundApplied || res2.RefundedCents != 0 {
		t.Fatalf("expected no second refund, got applied=%v refunded=%d", res2.RefundApplied, res2.RefundedCents)
	}
}
