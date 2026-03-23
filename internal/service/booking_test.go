package service

import (
	"context"
	"strings"
	"testing"

	"github.com/alekslesik/telegram-bot-simple/internal/repository"
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
	if !strings.Contains(start, "Choose a service") {
		t.Fatalf("unexpected start text: %q", start)
	}

	handled, msg, err := svc.HandleText(ctx, userID, "1")
	if err != nil || !handled {
		t.Fatalf("service selection failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(msg, "Choose a slot") {
		t.Fatalf("unexpected slot prompt: %q", msg)
	}

	handled, msg, err = svc.HandleText(ctx, userID, "1")
	if err != nil || !handled {
		t.Fatalf("slot selection failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(msg, "Reply YES to confirm") {
		t.Fatalf("unexpected confirmation text: %q", msg)
	}

	handled, msg, err = svc.HandleText(ctx, userID, "YES")
	if err != nil || !handled {
		t.Fatalf("confirmation failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(msg, "Booking confirmed") {
		t.Fatalf("unexpected final text: %q", msg)
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
	if _, _, err := svc1.HandleText(ctx, userID, "1"); err != nil {
		t.Fatalf("service selection error: %v", err)
	}

	svc2 := NewBookingService(repo)
	handled, msg, err := svc2.HandleText(ctx, userID, "1")
	if err != nil {
		t.Fatalf("slot selection error after restart: %v", err)
	}
	if !handled || !strings.Contains(msg, "Reply YES to confirm") {
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
