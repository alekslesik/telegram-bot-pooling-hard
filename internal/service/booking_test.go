package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/i18n"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func TestBookingService_HappyPath(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()
	const userID int64 = 42

	start, err := svc.Start(ctx, userID)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	if !strings.Contains(start, "ФИО") {
		t.Fatalf("unexpected start text: %q", start)
	}

	handled, msg, err := svc.HandleText(ctx, userID, "Ivan Ivanov")
	if err != nil || !handled {
		t.Fatalf("name step failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(msg, "phone") {
		t.Fatalf("unexpected phone prompt: %q", msg)
	}

	handled, msg, err = svc.HandleText(ctx, userID, "+79991234567")
	if err != nil || !handled {
		t.Fatalf("phone step failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(msg, "Профиль сохранен") {
		t.Fatalf("unexpected profile saved prompt: %q", msg)
	}
	handled, msg, err = svc.HandleText(ctx, userID, "extra")
	if err != nil {
		t.Fatalf("extra message error: %v", err)
	}
	if handled || msg != "" {
		t.Fatalf("expected no active text flow after registration, got handled=%v msg=%q", handled, msg)
	}
	specialties, totalSpecialties, err := svc.ListSpecialtiesPage(ctx, 0, 4)
	if err != nil || totalSpecialties == 0 || len(specialties) == 0 {
		t.Fatalf("specialties list error: total=%d len=%d err=%v", totalSpecialties, len(specialties), err)
	}
	doctors, totalDoctors, err := svc.ListDoctorsPage(ctx, specialties[0].ID, 0, 4)
	if err != nil || totalDoctors == 0 || len(doctors) == 0 {
		t.Fatalf("doctors list error: total=%d len=%d err=%v", totalDoctors, len(doctors), err)
	}
	slots, totalSlots, err := svc.ListSlotsPage(ctx, specialties[0].ID, doctors[0].ID, 0, 4)
	if err != nil || totalSlots == 0 || len(slots) == 0 {
		t.Fatalf("slots list error: total=%d len=%d err=%v", totalSlots, len(slots), err)
	}
	final, err := svc.ConfirmClinicBooking(ctx, userID, specialties[0].ID, doctors[0].ID, slots[0].ID, i18n.Ru)
	if err != nil {
		t.Fatalf("confirm clinic booking error: %v", err)
	}
	if !strings.Contains(final, "Запись подтверждена") {
		t.Fatalf("unexpected confirmation text: %q", final)
	}
}

func TestBookingService_StatePersistenceAcrossServiceInstances(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	const userID int64 = 11

	svc1 := NewBookingService(repo, nil)
	if _, err := svc1.Start(ctx, userID); err != nil {
		t.Fatalf("start error: %v", err)
	}
	if _, _, err := svc1.HandleText(ctx, userID, "Ivan Ivanov"); err != nil {
		t.Fatalf("name step error: %v", err)
	}
	if _, _, err := svc1.HandleText(ctx, userID, "+79991234567"); err != nil {
		t.Fatalf("phone step error: %v", err)
	}
	svc2 := NewBookingService(repo, nil)
	handled, msg, err := svc2.HandleText(ctx, userID, "1")
	if err != nil {
		t.Fatalf("slot selection error after restart: %v", err)
	}
	if handled || msg != "" {
		t.Fatalf("unexpected result after restart: handled=%v msg=%q", handled, msg)
	}
}

func TestBookingService_Cancel(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()
	const userID int64 = 7

	if _, err := svc.Start(ctx, userID); err != nil {
		t.Fatalf("start error: %v", err)
	}
	msg, err := svc.Cancel(ctx, userID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if !strings.Contains(msg, "cancelled") {
		t.Fatalf("unexpected cancel text: %q", msg)
	}
}

func TestBookingService_CancelClinicBooking_PartialRefundMessage(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()
	const userID int64 = 5610

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	target := time.Now().UTC().Add(2 * time.Hour)
	day := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	startMinute := target.Hour()*60 + target.Minute()
	if _, err := repo.GenerateDoctorSlots(ctx, 1, 1, day, startMinute, startMinute+30, 30); err != nil {
		t.Fatalf("generate near slots error: %v", err)
	}

	slots, _, err := svc.ListSlotsPage(ctx, 1, 1, 0, 100)
	if err != nil {
		t.Fatalf("list slots error: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}
	slotID := slots[0].ID
	for _, slot := range slots {
		diff := slot.StartAt.Sub(target)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 20*time.Minute {
			slotID = slot.ID
			break
		}
	}

	paid, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, slotID, "op-service-cancel-partial-5610")
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}

	msg, err := svc.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel clinic booking error: %v", err)
	}
	if !strings.Contains(msg, "частичный возврат") {
		t.Fatalf("expected partial refund message, got %q", msg)
	}
}

func TestBookingService_RegisteredClientSkipsRegistration(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()
	const userID int64 = 123

	if _, err := repo.UpsertClient(ctx, repository.Client{
		TelegramUserID: userID,
		FullName:       "John Doe",
		Phone:          "+12345678901",
	}); err != nil {
		t.Fatalf("upsert client error: %v", err)
	}
	start, err := svc.Start(ctx, userID)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	if !strings.Contains(start, "Выберите направление") {
		t.Fatalf("registered client should skip registration, got %q", start)
	}
}

func TestBookingService_AdminCapabilities_ByRole(t *testing.T) {
	repo := repository.NewMemoryRepository()
	repo.EnsureUserProfile(context.Background(), 777)
	repo.EnsureUserProfile(context.Background(), 888)
	repo.EnsureUserProfile(context.Background(), 999)

	repo.SetAdminRole(777, repository.AdminRoleOperator)
	repo.SetAdminRole(888, repository.AdminRoleAdmin)

	svc := NewBookingService(repo, nil)
	ctx := context.Background()

	opCaps, err := svc.AdminCapabilities(ctx, 777)
	if err != nil {
		t.Fatalf("operator caps error: %v", err)
	}
	if !opCaps.CanOpenPanel || !opCaps.CanManageDaySlots || opCaps.CanManageCatalog || opCaps.CanViewAnalytics {
		t.Fatalf("unexpected operator caps: %+v", opCaps)
	}

	adminCaps, err := svc.AdminCapabilities(ctx, 888)
	if err != nil {
		t.Fatalf("admin caps error: %v", err)
	}
	if !adminCaps.CanOpenPanel || !adminCaps.CanManageDaySlots || !adminCaps.CanManageCatalog || !adminCaps.CanViewAnalytics {
		t.Fatalf("unexpected admin caps: %+v", adminCaps)
	}
}

func TestBookingService_AdminHandleText_DeniesOperatorForCatalogState(t *testing.T) {
	repo := repository.NewMemoryRepository()
	repo.SetAdminRole(5001, repository.AdminRoleOperator)
	svc := NewBookingService(repo, nil)
	ctx := context.Background()

	if err := repo.SaveConversationState(ctx, repository.ConversationState{
		TelegramUserID: 5001,
		State:          StateAdminAddSpecialty,
		PayloadJSON:    "{}",
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save state error: %v", err)
	}

	handled, msg, err := svc.HandleText(ctx, 5001, "Кардиохирург|10")
	if err != nil {
		t.Fatalf("handle text error: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if !strings.Contains(msg, "Нет доступа") {
		t.Fatalf("expected access denied, got %q", msg)
	}
}

func TestBookingService_CancelClinicBooking_PolicyBlockedMessage(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()
	const userID int64 = 5601

	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatal(err)
	}

	pastDay := time.Now().UTC().Add(-24 * time.Hour)
	if _, err := repo.GenerateDoctorSlots(ctx, 1, 1, pastDay, 10*60, 11*60, 30); err != nil {
		t.Fatalf("generate past slots error: %v", err)
	}

	slots, _, err := svc.ListSlotsPage(ctx, 1, 1, 0, 50)
	if err != nil {
		t.Fatalf("list slots error: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}

	paidText, err := svc.ConfirmClinicBooking(ctx, userID, 1, 1, slots[0].ID, i18n.Ru)
	if err != nil {
		t.Fatalf("paid booking error: %v", err)
	}
	if !strings.Contains(paidText, "Запись подтверждена") {
		t.Fatalf("unexpected paid booking response: %q", paidText)
	}

	idPrefix := "ID: "
	idx := strings.Index(paidText, idPrefix)
	if idx < 0 {
		t.Fatalf("failed to parse booking id from response: %q", paidText)
	}
	idParts := strings.Fields(paidText[idx+len(idPrefix):])
	if len(idParts) == 0 {
		t.Fatalf("failed to parse booking id tail from response: %q", paidText)
	}
	bookingID, err := strconv.ParseInt(idParts[0], 10, 64)
	if err != nil {
		t.Fatalf("parse booking id error: %v", err)
	}

	msg, err := svc.CancelClinicBooking(ctx, userID, bookingID)
	if err != nil {
		t.Fatalf("cancel clinic booking error: %v", err)
	}
	if !strings.Contains(msg, "Возврат недоступен") {
		t.Fatalf("expected policy blocked message, got %q", msg)
	}
}
