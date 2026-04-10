package cache

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is an optional Redis client for caching (e.g. specialty list pages).
type Redis struct {
	c *redis.Client
}

// NewRedisFromEnv returns nil if REDIS_ADDR is unset (cache disabled).
func NewRedisFromEnv() (*Redis, error) {
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if addr == "" {
		return nil, nil
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	if err := c.Ping(context.Background()).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Redis{c: c}, nil
}

func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	if r == nil || r.c == nil {
		return "", nil
	}
	v, err := r.c.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (r *Redis) Set(ctx context.Context, key string, val string, ttl time.Duration) error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Set(ctx, key, val, ttl).Err()
}

// SetNXEX stores a value only when key does not exist, with TTL.
func (r *Redis) SetNXEX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error) {
	if r == nil || r.c == nil {
		return false, errors.New("redis: nil client")
	}
	return r.c.SetNX(ctx, key, val, ttl).Result()
}

// IncrWithTTL atomically increments key and sets TTL on first increment.
func (r *Redis) IncrWithTTL(ctx context.Context, key string, ttlSeconds int) (int64, error) {
	if r == nil || r.c == nil {
		return 0, errors.New("redis: nil client")
	}
	if ttlSeconds <= 0 {
		return 0, errors.New("redis: ttlSeconds must be > 0")
	}
	const script = `
local v = redis.call("INCR", KEYS[1])
if v == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return v
`
	return redis.NewScript(script).Run(ctx, r.c, []string{key}, ttlSeconds).Int64()
}

func (r *Redis) Close() error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Close()
}

func (r *Redis) Ping(ctx context.Context) error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Ping(ctx).Err()
}
