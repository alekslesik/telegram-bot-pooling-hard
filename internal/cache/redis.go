package cache

import (
	"context"
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
