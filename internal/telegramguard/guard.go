package telegramguard

import "context"

type Guard struct {
	MessageLimiter  *Limiter
	CallbackLimiter *Limiter
	Deduplicator    *Deduplicator
}

func (g *Guard) IsDuplicate(ctx context.Context, updateID int) (bool, error) {
	if g == nil || g.Deduplicator == nil {
		return false, nil
	}
	return g.Deduplicator.Seen(ctx, updateID)
}

func (g *Guard) AllowMessage(ctx context.Context, telegramUserID int64) (bool, error) {
	if g == nil || g.MessageLimiter == nil {
		return true, nil
	}
	return g.MessageLimiter.Allow(ctx, telegramUserID, "message")
}

func (g *Guard) AllowCallback(ctx context.Context, telegramUserID int64) (bool, error) {
	if g == nil || g.CallbackLimiter == nil {
		return true, nil
	}
	return g.CallbackLimiter.Allow(ctx, telegramUserID, "callback")
}
