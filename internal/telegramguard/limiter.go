package telegramguard

import (
	"context"
	"fmt"
	"strings"
)

type LimiterStore interface {
	IncrWithTTL(ctx context.Context, key string, ttlSeconds int) (int64, error)
}

type Limiter struct {
	store        LimiterStore
	maxPerWindow int64
	windowSec    int
	keyPrefix    string
}

func NewLimiter(store LimiterStore, maxPerWindow int64, windowSec int, keyPrefix string) *Limiter {
	if store == nil || maxPerWindow <= 0 || windowSec <= 0 {
		return nil
	}
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = "tg:rl"
	}
	return &Limiter{
		store:        store,
		maxPerWindow: maxPerWindow,
		windowSec:    windowSec,
		keyPrefix:    keyPrefix,
	}
}

func (l *Limiter) Allow(ctx context.Context, telegramUserID int64, kind string) (bool, error) {
	if l == nil {
		return true, nil
	}
	key := fmt.Sprintf("%s:%s:%d", l.keyPrefix, strings.TrimSpace(kind), telegramUserID)
	n, err := l.store.IncrWithTTL(ctx, key, l.windowSec)
	if err != nil {
		return false, err
	}
	return n <= l.maxPerWindow, nil
}
