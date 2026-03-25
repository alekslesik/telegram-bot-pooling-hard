package service

import (
	"context"
	"strings"
	"testing"

	"github.com/alekslesik/telegram-bot-pooling-middle/internal/repository"
)

func TestBookingService_HappyPath(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo)
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
	final, err := svc.ConfirmClinicBooking(ctx, userID, specialties[0].ID, doctors[0].ID, slots[0].ID)
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

	svc1 := NewBookingService(repo)
	if _, err := svc1.Start(ctx, userID); err != nil {
		t.Fatalf("start error: %v", err)
	}
	if _, _, err := svc1.HandleText(ctx, userID, "Ivan Ivanov"); err != nil {
		t.Fatalf("name step error: %v", err)
	}
	if _, _, err := svc1.HandleText(ctx, userID, "+79991234567"); err != nil {
		t.Fatalf("phone step error: %v", err)
	}
	svc2 := NewBookingService(repo)
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
	svc := NewBookingService(repo)
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

func TestBookingService_RegisteredClientSkipsRegistration(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo)
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
