package service

import (
	"context"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

type OutboxHandler func(ctx context.Context, event repository.OutboxEvent) error

type OutboxWorker struct {
	repo       repository.BookingRepository
	handler    OutboxHandler
	batchSize  int
	retryDelay time.Duration
}

func NewOutboxWorker(repo repository.BookingRepository, handler OutboxHandler, batchSize int, retryDelay time.Duration) *OutboxWorker {
	if batchSize <= 0 {
		batchSize = 20
	}
	if retryDelay <= 0 {
		retryDelay = 30 * time.Second
	}
	return &OutboxWorker{
		repo:       repo,
		handler:    handler,
		batchSize:  batchSize,
		retryDelay: retryDelay,
	}
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
			_ = w.repo.MarkOutboxEventFailed(ctx, item.ID, err.Error(), time.Now().UTC().Add(w.retryDelay))
			continue
		}
		_ = w.repo.MarkOutboxEventDone(ctx, item.ID)
	}
	return nil
}
