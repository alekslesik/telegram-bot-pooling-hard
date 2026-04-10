package service

import (
	"context"
	"strconv"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

type OutboxHandler func(ctx context.Context, event repository.OutboxEvent) error

type OutboxWorker struct {
	repo        repository.BookingRepository
	handler     OutboxHandler
	batchSize   int
	retryDelay  time.Duration
	maxAttempts int
	// deadLetterHook runs after MarkOutboxEventDead (terminal status=failed); use for webhooks/alerts (RFC §2).
	deadLetterHook func(context.Context, repository.OutboxEvent, error)
}

// WithDeadLetterHook registers a callback invoked when an outbox event exhausts retries (dead letter).
func WithDeadLetterHook(fn func(context.Context, repository.OutboxEvent, error)) func(*OutboxWorker) {
	return func(w *OutboxWorker) {
		w.deadLetterHook = fn
	}
}

func NewOutboxWorker(repo repository.BookingRepository, handler OutboxHandler, batchSize int, retryDelay time.Duration, opts ...func(*OutboxWorker)) *OutboxWorker {
	if batchSize <= 0 {
		batchSize = 20
	}
	if retryDelay <= 0 {
		retryDelay = 30 * time.Second
	}
	maxAttempts := 5
	w := &OutboxWorker{
		repo:        repo,
		handler:     handler,
		batchSize:   batchSize,
		retryDelay:  retryDelay,
		maxAttempts: maxAttempts,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		_ = w.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *OutboxWorker) Tick(ctx context.Context) error {
	now := time.Now().UTC()
	items, err := w.repo.ClaimDueOutboxEvents(ctx, w.batchSize, now)
	if err != nil {
		return err
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if w.handler == nil {
			_ = w.repo.MarkOutboxEventDone(ctx, item.ID)
			continue
		}
		if err := w.handler(ctx, item); err != nil {
			if w.maxAttempts > 0 && item.Attempts >= w.maxAttempts {
				_ = w.repo.LogAnalyticsEvent(ctx, nil, "outbox_event_dead", `{"event_id":`+strconv.FormatInt(item.ID, 10)+`,"event_type":"`+item.EventType+`","attempts":`+strconv.Itoa(item.Attempts)+`}`)
				_ = w.repo.MarkOutboxEventDead(ctx, item.ID, err.Error())
				if w.deadLetterHook != nil {
					w.deadLetterHook(ctx, item, err)
				}
				continue
			}
			nextAttemptAt := time.Now().UTC().Add(w.retryBackoff(item.Attempts))
			_ = w.repo.MarkOutboxEventFailed(ctx, item.ID, err.Error(), nextAttemptAt)
			continue
		}
		_ = w.repo.MarkOutboxEventDone(ctx, item.ID)
	}
	return nil
}

func (w *OutboxWorker) retryBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := w.retryDelay
	// Exponential backoff with safe cap to avoid unbounded retry gaps.
	for i := 1; i < attempts; i++ {
		if delay >= 10*time.Minute {
			return 10 * time.Minute
		}
		delay *= 2
	}
	if delay > 10*time.Minute {
		return 10 * time.Minute
	}
	return delay
}
