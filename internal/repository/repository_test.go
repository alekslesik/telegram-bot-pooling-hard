package repository

import (
	"context"
	"testing"
)

func TestMemoryRepository_StateCRUD(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 55

	if err := repo.SaveConversationState(ctx, ConversationState{
		TelegramUserID: userID,
		State:          "waiting_service",
		PayloadJSON:    "{}",
	}); err != nil {
		t.Fatalf("save state error: %v", err)
	}

	st, err := repo.GetConversationState(ctx, userID)
	if err != nil {
		t.Fatalf("get state error: %v", err)
	}
	if st.State != "waiting_service" {
		t.Fatalf("unexpected state: %q", st.State)
	}

	if err := repo.DeleteConversationState(ctx, userID); err != nil {
		t.Fatalf("delete state error: %v", err)
	}
	if _, err := repo.GetConversationState(ctx, userID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestMemoryRepository_BookingLifecycle(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	services, err := repo.ListActiveServices(ctx)
	if err != nil || len(services) == 0 {
		t.Fatalf("services error: %v len=%d", err, len(services))
	}

	slots, err := repo.ListAvailableSlots(ctx, services[0].ID)
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots error: %v len=%d", err, len(slots))
	}
	slot := slots[0]

	booking, err := repo.CreateBooking(ctx, Booking{
		TelegramUserID: 1,
		ServiceID:      services[0].ID,
		SlotID:         slot.ID,
		Status:         "confirmed",
	})
	if err != nil {
		t.Fatalf("create booking error: %v", err)
	}
	if booking.ID == 0 {
		t.Fatal("expected booking ID")
	}

	if err := repo.MarkSlotUnavailable(ctx, slot.ID); err != nil {
		t.Fatalf("mark slot unavailable error: %v", err)
	}
	updated, err := repo.GetSlotByID(ctx, slot.ID)
	if err != nil {
		t.Fatalf("get slot error: %v", err)
	}
	if updated.IsAvailable {
		t.Fatal("slot should be unavailable")
	}
}
