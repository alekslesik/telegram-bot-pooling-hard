package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func TestOutboxWorkerTickMarksDone(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":1}`,
	})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}

	var handled int32
	worker := NewOutboxWorker(repo, func(_ context.Context, event repository.OutboxEvent) error {
		if event.EventType == "booking_confirmed" {
			atomic.AddInt32(&handled, 1)
		}
		return nil
	}, 10, 2*time.Second)

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick error: %v", err)
	}
	if atomic.LoadInt32(&handled) != 1 {
		t.Fatalf("expected 1 handled event, got %d", handled)
	}

	items, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("claim after done error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no pending events after successful processing, got %d", len(items))
	}
}

func TestOutboxWorkerTickRetriesOnFailure(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	ev, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_refunded",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":2}`,
	})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}

	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New("temporary downstream failure")
	}, 10, 100*time.Millisecond)

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick error: %v", err)
	}

	immediate, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("immediate claim error: %v", err)
	}
	if len(immediate) != 0 {
		t.Fatalf("expected no immediate retry events, got %d", len(immediate))
	}

	time.Sleep(120 * time.Millisecond)
	retry, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("retry claim error: %v", err)
	}
	if len(retry) != 1 || retry[0].ID != ev.ID {
		t.Fatalf("expected retry for event %d, got %+v", ev.ID, retry)
	}
}

func TestOutboxWorkerTickMarksDeadAfterMaxAttempts(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_refunded",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":3}`,
		AvailableAt:   now,
	})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}

	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New("permanent downstream failure")
	}, 10, 20*time.Millisecond)
	worker.maxAttempts = 1

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf("tick error: %v", err)
	}
	counts, err := repo.CountOutboxByStatus(ctx)
	if err != nil {
		t.Fatalf("count status error: %v", err)
	}
	if counts["failed"] != 1 {
		t.Fatalf("expected failed=1, got %+v", counts)
	}
}
