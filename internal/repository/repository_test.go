package repository

import (
	"context"
	"testing"
	"time"
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

	booking, err := repo.ConfirmServiceBooking(ctx, Booking{
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

	updated, err := repo.GetSlotByID(ctx, slot.ID)
	if err != nil {
		t.Fatalf("get slot error: %v", err)
	}
	if updated.IsAvailable {
		t.Fatal("slot should be unavailable")
	}

	if _, err := repo.ConfirmServiceBooking(ctx, Booking{
		TelegramUserID: 2,
		ServiceID:      services[0].ID,
		SlotID:         slot.ID,
		Status:         "confirmed",
	}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for already-booked slot, got: %v", err)
	}
}

func TestMemoryRepository_ClientUpsertAndGet(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 99

	client, err := repo.UpsertClient(ctx, Client{
		TelegramUserID: userID,
		FullName:       "Jane Doe",
		Phone:          "+79990001122",
	})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if client.TelegramUserID != userID {
		t.Fatalf("unexpected user id: %d", client.TelegramUserID)
	}

	got, err := repo.GetClientByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("get client error: %v", err)
	}
	if got.FullName != "Jane Doe" || got.Phone != "+79990001122" {
		t.Fatalf("unexpected client data: %+v", got)
	}
}

func TestMemoryRepository_AdminDayTools_CloseOpenAndView(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	var day time.Time
	for _, slot := range repo.doctorSlots {
		if slot.DoctorID == 1 && slot.SpecialtyID == 1 {
			day = time.Date(slot.StartAt.Year(), slot.StartAt.Month(), slot.StartAt.Day(), 0, 0, 0, 0, time.UTC)
			break
		}
	}
	if day.IsZero() {
		t.Fatal("expected at least one initial doctor slot for doctor_id=1 specialty_id=1")
	}

	// Ensure the day has multiple slots for a meaningful view/open/close test.
	_, err := repo.GenerateDoctorSlots(ctx, 1, 1, day, 9*60, 12*60, 30)
	if err != nil {
		t.Fatalf("generate slots error: %v", err)
	}

	if _, err := repo.CloseDoctorDay(ctx, 1, 1, day); err != nil {
		t.Fatalf("close day error: %v", err)
	}

	slots, err := repo.ListDoctorSlotsForDay(ctx, 1, 1, day)
	if err != nil {
		t.Fatalf("list slots error: %v", err)
	}
	if len(slots) < 2 {
		t.Fatalf("expected >=2 slots on the day, got %d", len(slots))
	}

	// Book the earliest slot as confirmed.
	bookedSlotID := slots[0].ID
	if _, err := repo.CreateClinicBooking(ctx, ClinicBooking{
		TelegramUserID: 1,
		SpecialtyID:    1,
		DoctorID:       1,
		DoctorSlotID:   bookedSlotID,
		Status:         "confirmed",
	}); err != nil {
		t.Fatalf("create clinic booking error: %v", err)
	}

	updated, err := repo.OpenDoctorDay(ctx, 1, 1, day)
	if err != nil {
		t.Fatalf("open day error: %v", err)
	}

	// After closing, all slots were unavailable; open should re-enable all except the booked one.
	if updated != len(slots)-1 {
		t.Fatalf("unexpected updated count: want %d got %d", len(slots)-1, updated)
	}

	after, err := repo.ListDoctorSlotsForDay(ctx, 1, 1, day)
	if err != nil {
		t.Fatalf("list slots after open error: %v", err)
	}

	for _, s := range after {
		if s.ID == bookedSlotID {
			if !s.IsBooked {
				t.Fatalf("expected booked slot to be marked as booked (id=%d)", s.ID)
			}
			if s.IsAvailable {
				t.Fatalf("expected booked slot to remain unavailable (id=%d)", s.ID)
			}
		} else {
			if s.IsBooked {
				t.Fatalf("expected slot not to be booked (id=%d)", s.ID)
			}
			if !s.IsAvailable {
				t.Fatalf("expected slot to become available (id=%d)", s.ID)
			}
		}
	}
}

func TestMemoryRepository_GetAdminRole(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	role, err := repo.GetAdminRole(ctx, 892122714)
	if err != nil {
		t.Fatalf("expected admin role for seeded user, got error: %v", err)
	}
	if role != AdminRoleAdmin {
		t.Fatalf("expected role %q, got %q", AdminRoleAdmin, role)
	}

	if _, err := repo.GetAdminRole(ctx, 42424242); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for unknown admin role, got: %v", err)
	}
}

func TestMemoryRepository_GenerateDoctorSlotsDateRange(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	from := time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 5, 2, 0, 0, 0, 0, time.UTC)

	inserted, err := repo.GenerateDoctorSlotsDateRange(ctx, 1, 1, from, to, 10*60, 11*60, 30)
	if err != nil {
		t.Fatalf("range generate error: %v", err)
	}
	if inserted < 4 {
		t.Fatalf("expected at least 4 inserts, got %d", inserted)
	}

	again, err := repo.GenerateDoctorSlotsDateRange(ctx, 1, 1, from, to, 10*60, 11*60, 30)
	if err != nil {
		t.Fatalf("range generate second call error: %v", err)
	}
	if again != 0 {
		t.Fatalf("expected 0 new inserts on duplicate call, got %d", again)
	}
}

func TestMemoryRepository_AdminRosterUpsertAndList(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	rec, err := repo.UpsertAdmin(ctx, 42, AdminRoleOperator, true)
	if err != nil {
		t.Fatalf("upsert admin error: %v", err)
	}
	if rec.Role != AdminRoleOperator || !rec.IsActive {
		t.Fatalf("unexpected admin record: %+v", rec)
	}
	rec, err = repo.UpsertAdmin(ctx, 42, AdminRoleAdmin, false)
	if err != nil {
		t.Fatalf("upsert admin update error: %v", err)
	}
	if rec.Role != AdminRoleAdmin || rec.IsActive {
		t.Fatalf("unexpected updated record: %+v", rec)
	}
	active, err := repo.ListAdmins(ctx, false, 0, 0)
	if err != nil {
		t.Fatalf("list admins error: %v", err)
	}
	for _, item := range active {
		if item.TelegramUserID == 42 {
			t.Fatalf("inactive admin should be filtered out")
		}
	}
}
