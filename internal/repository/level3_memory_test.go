package repository

import (
	"context"
	"testing"
	"time"
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

func TestMemoryRepository_CancelClinicBooking_AfterStart_NoRefund(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 401

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	// Move seeded slot to the past so refund policy blocks refund.
	repo.mu.Lock()
	slot := repo.doctorSlots[1]
	slot.StartAt = time.Now().UTC().Add(-30 * time.Minute)
	repo.doctorSlots[1] = slot
	repo.mu.Unlock()

	p0, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-cancel-401")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	res, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if res.RefundApplied || res.RefundedCents != 0 {
		t.Fatalf("expected no refund after slot start, got applied=%v refunded=%d", res.RefundApplied, res.RefundedCents)
	}
	if !res.RefundBlockedByPolicy {
		t.Fatal("expected refund blocked by policy after slot start")
	}
	if res.RefundIsPartial {
		t.Fatal("partial refund flag must be false when refund is blocked")
	}

	p1, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	want := p0.BalanceCents - 100
	if p1.BalanceCents != want {
		t.Fatalf("expected balance to stay debited at %d, got %d", want, p1.BalanceCents)
	}
}

func TestMemoryRepository_CancelClinicBooking_BeforeStartPartialRefund(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 450

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	repo.mu.Lock()
	slot := repo.doctorSlots[1]
	slot.StartAt = time.Now().UTC().Add(2 * time.Hour)
	repo.doctorSlots[1] = slot
	repo.mu.Unlock()

	p0, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-cancel-partial-450")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	res, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if !res.RefundApplied {
		t.Fatal("expected refund to be applied")
	}
	if !res.RefundIsPartial {
		t.Fatal("expected partial refund flag")
	}
	if res.RefundBlockedByPolicy {
		t.Fatal("policy blocked must be false for partial refund")
	}
	if res.RefundedCents != 50 {
		t.Fatalf("expected 50 cents partial refund, got %d", res.RefundedCents)
	}

	p1, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	wantBalance := p0.BalanceCents - 50
	if p1.BalanceCents != wantBalance {
		t.Fatalf("unexpected balance after partial refund: want=%d got=%d", wantBalance, p1.BalanceCents)
	}
}

func TestMemoryRepository_WalletReadModel_UpdatedAfterDebitAndRefund(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 402

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-read-model-402")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	modelAfterDebit, err := repo.GetWalletBalanceReadModel(ctx, userID)
	if err != nil {
		t.Fatalf("get read model after debit error: %v", err)
	}
	if modelAfterDebit.BalanceCents != paid.BalanceAfter {
		t.Fatalf("read-model balance mismatch after debit: got=%d want=%d", modelAfterDebit.BalanceCents, paid.BalanceAfter)
	}
	if modelAfterDebit.LastTxID == nil {
		t.Fatal("expected last_tx_id after debit")
	}

	if _, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID); err != nil {
		t.Fatalf("cancel booking error: %v", err)
	}
	modelAfterRefund, err := repo.GetWalletBalanceReadModel(ctx, userID)
	if err != nil {
		t.Fatalf("get read model after refund error: %v", err)
	}
	if modelAfterRefund.BalanceCents <= modelAfterDebit.BalanceCents {
		t.Fatalf("expected balance to increase after refund, got before=%d after=%d", modelAfterDebit.BalanceCents, modelAfterRefund.BalanceCents)
	}
	if modelAfterRefund.LastTxID == nil {
		t.Fatal("expected last_tx_id after refund")
	}
}

func TestMemoryRepository_CancelClinicBooking_UsesConfiguredPartialRefundPolicy(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.SetClinicBookingRefundPolicy(ClinicBookingRefundPolicy{
		PartialWindow:  2 * time.Hour,
		PartialPercent: 25,
	}); err != nil {
		t.Fatalf("set refund policy: %v", err)
	}

	ctx := context.Background()
	const userID int64 = 403

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	repo.mu.Lock()
	slot := repo.doctorSlots[1]
	slot.StartAt = time.Now().UTC().Add(30 * time.Minute)
	repo.doctorSlots[1] = slot
	repo.mu.Unlock()

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-policy-403")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	res, err := repo.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if !res.RefundApplied || !res.RefundIsPartial {
		t.Fatalf("expected configured partial refund, got applied=%v partial=%v", res.RefundApplied, res.RefundIsPartial)
	}
	if res.RefundedCents != 25 {
		t.Fatalf("expected configured partial refund amount 25, got %d", res.RefundedCents)
	}
}

func TestMemoryRepositoryCountWalletBalanceMismatches(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	const (
		userConsistent  int64 = 7001
		userMissingRM   int64 = 7002
		userLedgerDrift int64 = 7003
	)

	if _, err := repo.EnsureUserProfile(ctx, userConsistent); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertWalletBalanceReadModel(ctx, userConsistent, 500, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.EnsureUserProfile(ctx, userMissingRM); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.EnsureUserProfile(ctx, userLedgerDrift); err != nil {
		t.Fatal(err)
	}
	paid, err := repo.ConfirmPaidClinicBooking(ctx, userLedgerDrift, 100, 1, 1, 1, "op-drift-7003")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}
	if paid.BalanceAfter <= 0 {
		t.Fatalf("unexpected balance after booking: %d", paid.BalanceAfter)
	}

	repo.mu.Lock()
	p := repo.userProfiles[userLedgerDrift]
	p.BalanceCents++
	repo.userProfiles[userLedgerDrift] = p
	repo.mu.Unlock()

	got, err := repo.CountWalletBalanceMismatches(ctx)
	if err != nil {
		t.Fatalf("count mismatches error: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected 2 mismatches, got %d", got)
	}
}
