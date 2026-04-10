package telegramguard

import (
	"context"
	"testing"
	"time"
)

type fakeDedupStore struct {
	seen map[string]struct{}
}

func (f *fakeDedupStore) SetNXEX(_ context.Context, key string, _ string, _ time.Duration) (bool, error) {
	if _, ok := f.seen[key]; ok {
		return false, nil
	}
	f.seen[key] = struct{}{}
	return true, nil
}

func TestDedup_FirstSeenReturnsFalseSecondReturnsTrue(t *testing.T) {
	d := NewDeduplicator(&fakeDedupStore{seen: map[string]struct{}{}}, 24*time.Hour, "tg:upd")
	dup, err := d.Seen(context.Background(), 123)
	if err != nil {
		t.Fatalf("first seen err: %v", err)
	}
	if dup {
		t.Fatal("expected first update to be non-duplicate")
	}
	dup, err = d.Seen(context.Background(), 123)
	if err != nil {
		t.Fatalf("second seen err: %v", err)
	}
	if !dup {
		t.Fatal("expected second update to be duplicate")
	}
}

func TestDedup_NoStoreGraceful(t *testing.T) {
	d := NewDeduplicator(nil, 24*time.Hour, "tg:upd")
	dup, err := d.Seen(context.Background(), 456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dup {
		t.Fatal("expected non-duplicate when store disabled")
	}
}
