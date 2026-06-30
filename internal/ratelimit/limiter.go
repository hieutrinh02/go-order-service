package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	redis *redis.Client
	now   func() time.Time
}

func New(redisClient *redis.Client) *Limiter {
	return &Limiter{
		redis: redisClient,
		now:   time.Now,
	}
}

type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

func (l *Limiter) Allow(ctx context.Context, scope string, key string, limit int, window time.Duration) (Result, error) {
	now := l.now().UTC()
	windowStart := now.Truncate(window)
	redisKey := fmt.Sprintf("rate_limit:%s:%s:%d", scope, key, windowStart.Unix())

	count, err := l.redis.Incr(ctx, redisKey).Result()
	if err != nil {
		return Result{}, err
	}

	if count == 1 {
		if err := l.redis.Expire(ctx, redisKey, window+time.Second).Err(); err != nil {
			return Result{}, err
		}
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	retryAfter := windowStart.Add(window).Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}

	return Result{
		Allowed:    int(count) <= limit,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: retryAfter,
	}, nil
}
