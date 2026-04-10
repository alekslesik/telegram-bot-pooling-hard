package telegramguard

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DedupStore interface {
	SetNXEX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error)
}

type Deduplicator struct {
	store     DedupStore
	ttl       time.Duration
	keyPrefix string
}

func NewDeduplicator(store DedupStore, ttl time.Duration, keyPrefix string) *Deduplicator {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = "tg:upd"
	}
	return &Deduplicator{
		store:     store,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

func (d *Deduplicator) Seen(ctx context.Context, updateID int) (bool, error) {
	if d == nil || d.store == nil {
		return false, nil
	}
	key := fmt.Sprintf("%s:%d", d.keyPrefix, updateID)
	isNew, err := d.store.SetNXEX(ctx, key, "1", d.ttl)
	if err != nil {
		return false, err
	}
	return !isNew, nil
}

func (d *Deduplicator) SeenKey(ctx context.Context, key string) (bool, error) {
	if d == nil || d.store == nil {
		return false, nil
	}
	if strings.TrimSpace(key) == "" {
		return false, nil
	}
	fullKey := fmt.Sprintf("%s:%s", d.keyPrefix, key)
	isNew, err := d.store.SetNXEX(ctx, fullKey, "1", d.ttl)
	if err != nil {
		return false, err
	}
	return !isNew, nil
}
