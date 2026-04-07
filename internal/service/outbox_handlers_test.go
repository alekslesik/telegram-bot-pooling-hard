package service

import (
	"context"
	"testing"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func TestBookingOutboxHandler_EnqueuesReminderEvent(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()

	payload := `{"booking_id":9001,"user_id":42,"slot_id":1}`
	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   payload,
		AvailableAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("enqueue booking_confirmed: %v", err)
	}

	worker := NewOutboxWorker(repo, NewBookingOutboxHandler(repo), 20, 100*time.Millisecond)
	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick error: %v", err)
	}
	// Reprocessing should not enqueue duplicate reminder due events.
	if _, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   payload,
		AvailableAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("enqueue duplicate booking_confirmed: %v", err)
	}
	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick duplicate error: %v", err)
	}

	claimed, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("claim reminders: %v", err)
	}
	found := false
	reminderCount := 0
	for _, ev := range claimed {
		if ev.EventType == "booking_reminder_due" {
			found = true
			reminderCount++
		}
	}
	if !found {
		t.Fatalf("expected booking_reminder_due event to be enqueued")
	}
	if reminderCount != 1 {
		t.Fatalf("expected exactly one reminder event, got %d", reminderCount)
	}
}

func TestBookingOutboxHandler_LogsReminderAnalytics(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_reminder_due",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":9002,"user_id":77,"slot_id":1}`,
		AvailableAt:   now,
	})
	if err != nil {
		t.Fatalf("enqueue booking_reminder_due: %v", err)
	}

	worker := NewOutboxWorker(repo, NewBookingOutboxHandler(repo), 20, 100*time.Millisecond)
	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick error: %v", err)
	}

	counts, err := repo.CountAnalyticsByEventSince(ctx, now.Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("analytics count error: %v", err)
	}
	if counts["booking_reminder_sent"] != 1 {
		t.Fatalf("expected booking_reminder_sent=1, got %d", counts["booking_reminder_sent"])
	}
}
