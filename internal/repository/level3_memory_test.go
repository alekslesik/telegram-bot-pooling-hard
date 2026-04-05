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

	_, err = repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1)
	if err != ErrInsufficientFunds {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
}
