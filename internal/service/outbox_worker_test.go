package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

const (
	errEnqueueFmt = "enqueue error: %v"
	errTickFmt    = "tick error: %v"
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
		t.Fatalf(errEnqueueFmt, err)
	}

	var handled int32
	worker := NewOutboxWorker(repo, func(_ context.Context, event repository.OutboxEvent) error {
		if event.EventType == "booking_confirmed" {
			atomic.AddInt32(&handled, 1)
		}
		return nil
	}, 10, 2*time.Second)

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf(errTickFmt, err)
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
		t.Fatalf(errEnqueueFmt, err)
	}

	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New("temporary downstream failure")
	}, 10, 20*time.Millisecond)

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf(errTickFmt, err)
	}

	immediate, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("immediate claim error: %v", err)
	}
	if len(immediate) != 0 {
		t.Fatalf("expected no immediate retry events, got %d", len(immediate))
	}

	time.Sleep(30 * time.Millisecond)
	retry, err := repo.ClaimDueOutboxEvents(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("retry claim error: %v", err)
	}
	if len(retry) != 1 || retry[0].ID != ev.ID {
		t.Fatalf("expected retry for event %d, got %+v", ev.ID, retry)
	}
}

func TestOutboxWorkerRetryBackoffExponential(t *testing.T) {
	repo := repository.NewMemoryRepository()
	worker := NewOutboxWorker(repo, nil, 10, 1*time.Second)
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: 1 * time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 4, want: 8 * time.Second},
	}
	for _, tc := range cases {
		got := worker.retryBackoff(tc.attempts)
		if got != tc.want {
			t.Fatalf("attempts=%d: want %s, got %s", tc.attempts, tc.want, got)
		}
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
		t.Fatalf(errEnqueueFmt, err)
	}

	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New("permanent downstream failure")
	}, 10, 20*time.Millisecond)
	worker.maxAttempts = 1

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf(errTickFmt, err)
	}
	counts, err := repo.CountOutboxByStatus(ctx)
	if err != nil {
		t.Fatalf("count status error: %v", err)
	}
	if counts["failed"] != 1 {
		t.Fatalf("expected failed=1, got %+v", counts)
	}

	analytics, err := repo.CountAnalyticsByEventSince(ctx, now.Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("analytics count error: %v", err)
	}
	if analytics["outbox_event_dead"] != 1 {
		t.Fatalf("expected outbox_event_dead=1, got %d", analytics["outbox_event_dead"])
	}
}
