package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryRepository_OutboxLifecycle(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	ev, err := repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":1}`,
		AvailableAt:   now,
	})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}
	if ev.ID == 0 || ev.Status != "pending" {
		t.Fatalf("unexpected outbox event after enqueue: %+v", ev)
	}

	items, err := repo.ClaimDueOutboxEvents(ctx, 10, now.Add(time.Second))
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if len(items) != 1 || items[0].ID != ev.ID {
		t.Fatalf("expected claimed event %d, got %+v", ev.ID, items)
	}
	if items[0].Status != "processing" || items[0].Attempts != 1 {
		t.Fatalf("expected processing status and attempts=1, got %+v", items[0])
	}

	if err := repo.MarkOutboxEventDone(ctx, ev.ID); err != nil {
		t.Fatalf("mark done error: %v", err)
	}
	again, err := repo.ClaimDueOutboxEvents(ctx, 10, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("reclaim error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no events after done, got %d", len(again))
	}
}

func TestMemoryRepository_OutboxRetryFlow(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	ev, err := repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		EventType:     "booking_refunded",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":2}`,
		AvailableAt:   now,
	})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}
	claimed, err := repo.ClaimDueOutboxEvents(ctx, 1, now)
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed event, got %d", len(claimed))
	}

	next := now.Add(15 * time.Second)
	if err := repo.MarkOutboxEventFailed(ctx, ev.ID, "temporary error", next); err != nil {
		t.Fatalf("mark failed error: %v", err)
	}

	none, err := repo.ClaimDueOutboxEvents(ctx, 10, now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("claim before next attempt error: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no events before retry window, got %d", len(none))
	}

	retry, err := repo.ClaimDueOutboxEvents(ctx, 10, now.Add(20*time.Second))
	if err != nil {
		t.Fatalf("claim after retry window error: %v", err)
	}
	if len(retry) != 1 || retry[0].ID != ev.ID {
		t.Fatalf("expected retry event %d, got %+v", ev.ID, retry)
	}
	if retry[0].Attempts != 2 {
		t.Fatalf("expected attempts=2 after retry, got %d", retry[0].Attempts)
	}
}

func TestMemoryRepository_OutboxNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if err := repo.MarkOutboxEventDone(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for mark done, got %v", err)
	}
	if err := repo.MarkOutboxEventFailed(ctx, 999999, "x", time.Now().UTC()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for mark failed, got %v", err)
	}
}

func TestMemoryRepository_OutboxDedupeKey(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	key := "booking_reminder_due:42"
	ev1, err := repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		DedupeKey:     key,
		EventType:     "booking_reminder_due",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":42}`,
	})
	if err != nil {
		t.Fatalf("enqueue #1 error: %v", err)
	}
	ev2, err := repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		DedupeKey:     key,
		EventType:     "booking_reminder_due",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":42}`,
	})
	if err != nil {
		t.Fatalf("enqueue #2 error: %v", err)
	}
	if ev1.ID != ev2.ID {
		t.Fatalf("expected deduped outbox id, got %d and %d", ev1.ID, ev2.ID)
	}
}

func TestMemoryRepository_CountOutboxByStatus(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":11}`,
		AvailableAt:   now,
	})
	_, _ = repo.EnqueueOutboxEvent(ctx, OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":12}`,
		AvailableAt:   now.Add(1 * time.Hour),
	})
	claimed, err := repo.ClaimDueOutboxEvents(ctx, 1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim error: %v len=%d", err, len(claimed))
	}

	counts, err := repo.CountOutboxByStatus(ctx)
	if err != nil {
		t.Fatalf("count outbox by status error: %v", err)
	}
	if counts["pending"] != 1 || counts["processing"] != 1 || counts["done"] != 0 {
		t.Fatalf("unexpected outbox counts: %+v", counts)
	}
}
