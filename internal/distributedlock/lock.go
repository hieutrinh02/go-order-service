package distributedlock

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrLockNotAcquired = errors.New("lock not acquired")
)

type Manager struct {
	redis *redis.Client
	ttl   time.Duration
}

type Lock struct {
	manager *Manager
	key     string
	token   string
}

func NewManager(redisClient *redis.Client, ttl time.Duration) *Manager {
	return &Manager{
		redis: redisClient,
		ttl:   ttl,
	}
}

func (m *Manager) Acquire(ctx context.Context, key string) (*Lock, error) {
	token := uuid.NewString()

	acquired, err := m.redis.SetNX(ctx, key, token, m.ttl).Result()
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrLockNotAcquired
	}

	return &Lock{
		manager: m,
		key:     key,
		token:   token,
	}, nil
}

func (l *Lock) Release(ctx context.Context) error {
	const releaseScript = `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`

	return l.manager.redis.Eval(ctx, releaseScript, []string{l.key}, l.token).Err()
}
