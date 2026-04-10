package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

const (
	errEnqueueFmt                 = "enqueue error: %v"
	errTickFmt                    = "tick error: %v"
	errPermanentDownstreamFailure = "permanent downstream failure"
)

type analyticsCaptureRepo struct {
	*repository.MemoryRepository
	lastEventType string
	lastPayload   string
}

func (r *analyticsCaptureRepo) LogAnalyticsEvent(ctx context.Context, userID *int64, eventType, payloadJSON string) error {
	r.lastEventType = eventType
	r.lastPayload = payloadJSON
	return r.MemoryRepository.LogAnalyticsEvent(ctx, userID, eventType, payloadJSON)
}

func TestOutboxWorkerTickMarksDone(t *testing.T) {
	repo := repository.NewMemoryRepository()
	ctx := context.Background()
	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "payment_confirmed",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":1,"user_id":1,"slot_id":1}`,
	})
	if err != nil {
		t.Fatalf(errEnqueueFmt, err)
	}

	var handled int32
	worker := NewOutboxWorker(repo, func(_ context.Context, event repository.OutboxEvent) error {
		if event.EventType == "payment_confirmed" {
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
		return errors.New(errPermanentDownstreamFailure)
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

func TestOutboxWorkerDeadLetterHook(t *testing.T) {
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

	var hookCalled bool
	var hookEv repository.OutboxEvent
	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New(errPermanentDownstreamFailure)
	}, 10, 20*time.Millisecond, WithDeadLetterHook(func(_ context.Context, ev repository.OutboxEvent, _ error) {
		hookCalled = true
		hookEv = ev
	}))
	worker.maxAttempts = 1

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf(errTickFmt, err)
	}
	if !hookCalled {
		t.Fatal("expected dead letter hook")
	}
	if hookEv.EventType != "booking_refunded" {
		t.Fatalf("unexpected hook event: %+v", hookEv)
	}
}

func TestOutboxWorkerDeadLetterAnalyticsUsesEnvelope(t *testing.T) {
	repo := &analyticsCaptureRepo{MemoryRepository: repository.NewMemoryRepository()}
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := repo.EnqueueOutboxEvent(ctx, repository.OutboxEvent{
		EventType:     "booking_refunded",
		AggregateType: "clinic_booking",
		PayloadJSON:   `{"booking_id":4}`,
		AvailableAt:   now,
	})
	if err != nil {
		t.Fatalf(errEnqueueFmt, err)
	}

	worker := NewOutboxWorker(repo, func(_ context.Context, _ repository.OutboxEvent) error {
		return errors.New(errPermanentDownstreamFailure)
	}, 10, 20*time.Millisecond)
	worker.maxAttempts = 1

	if err := worker.Tick(ctx); err != nil {
		t.Fatalf(errTickFmt, err)
	}
	if repo.lastEventType != "outbox_event_dead" {
		t.Fatalf("expected analytics event outbox_event_dead, got %q", repo.lastEventType)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(repo.lastPayload), &envelope); err != nil {
		t.Fatalf("expected valid JSON envelope payload, got error: %v payload=%q", err, repo.lastPayload)
	}

	if got := envelope["event_name"]; got != "outbox_event_dead" {
		t.Fatalf("expected event_name outbox_event_dead, got %#v", got)
	}
	if got, ok := envelope["user_id"]; !ok || got != nil {
		t.Fatalf("expected user_id to be present and null, got %#v", got)
	}
	if got := envelope["source"]; got != "worker" {
		t.Fatalf("expected source=worker, got %#v", got)
	}
	if got := envelope["ts"]; got == nil || got == "" {
		t.Fatalf("expected ts to be present, got %#v", got)
	}

	contextAny, ok := envelope["context"]
	if !ok {
		t.Fatalf("expected context field in envelope, got %#v", envelope)
	}
	contextMap, ok := contextAny.(map[string]any)
	if !ok {
		t.Fatalf("expected context object, got %#v", contextAny)
	}
	if got := contextMap["event_type"]; got != "booking_refunded" {
		t.Fatalf("expected context.event_type booking_refunded, got %#v", got)
	}
}
