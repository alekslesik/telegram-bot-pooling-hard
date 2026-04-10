package telegramguard

import (
	"context"
	"testing"
)

type fakeLimiterStore struct {
	counts map[string]int64
}

func (f *fakeLimiterStore) IncrWithTTL(_ context.Context, key string, _ int) (int64, error) {
	f.counts[key]++
	return f.counts[key], nil
}

func TestLimiter_AllowsUnderThreshold(t *testing.T) {
	store := &fakeLimiterStore{counts: map[string]int64{}}
	lim := NewLimiter(store, 2, 60, "tg:rl")
	ok, err := lim.Allow(context.Background(), 11, "message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected request to be allowed")
	}
}

func TestLimiter_BlocksOverThreshold(t *testing.T) {
	store := &fakeLimiterStore{counts: map[string]int64{}}
	lim := NewLimiter(store, 1, 60, "tg:rl")

	ok, err := lim.Allow(context.Background(), 11, "callback")
	if err != nil || !ok {
		t.Fatalf("first allow: ok=%v err=%v", ok, err)
	}
	ok, err = lim.Allow(context.Background(), 11, "callback")
	if err != nil {
		t.Fatalf("second allow err: %v", err)
	}
	if ok {
		t.Fatal("expected second request to be blocked")
	}
}
