package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedisWithAddr(t *testing.T, addr string) *Redis {
	t.Helper()
	return &Redis{c: redis.NewClient(&redis.Options{Addr: addr})}
}

func TestRedis_IncrWithTTL_SetsExpiryOnFirstIncrement(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	r := newRedisWithAddr(t, mr.Addr())
	ctx := context.Background()

	n, err := r.IncrWithTTL(ctx, "rl:test:k", 60)
	if err != nil || n != 1 {
		t.Fatalf("first incr: n=%d err=%v", n, err)
	}
	if ttl := mr.TTL("rl:test:k"); ttl <= 0 {
		t.Fatalf("expected ttl set on first increment, got %s", ttl)
	}

	n2, err := r.IncrWithTTL(ctx, "rl:test:k", 60)
	if err != nil || n2 != 2 {
		t.Fatalf("second incr: n=%d err=%v", n2, err)
	}
	if ttl := mr.TTL("rl:test:k"); ttl <= 0 || ttl > 60*time.Second {
		t.Fatalf("unexpected ttl after second increment: %s", ttl)
	}
}

func TestRedis_SetNXEX(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	r := newRedisWithAddr(t, mr.Addr())
	ctx := context.Background()

	ok, err := r.SetNXEX(ctx, "tg:upd:1", "1", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("first setnxex: ok=%v err=%v", ok, err)
	}
	ok, err = r.SetNXEX(ctx, "tg:upd:1", "1", 30*time.Second)
	if err != nil {
		t.Fatalf("second setnxex err: %v", err)
	}
	if ok {
		t.Fatal("expected second setnxex to return false")
	}
}
