package appstart

import (
	"context"
	"log/slog"
	"time"
)

func Retry[T any](
	ctx context.Context,
	logger *slog.Logger,
	name string,
	attempts int,
	delay time.Duration,
	fn func(context.Context) (T, error),
) (T, error) {
	var zero T
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := fn(ctx)
		if err == nil {
			if attempt > 1 {
				logger.Info("startup dependency connected", "dependency", name, "attempt", attempt)
			}
			return result, nil
		}

		lastErr = err
		logger.Warn("startup dependency not ready",
			"dependency", name,
			"attempt", attempt,
			"max_attempts", attempts,
			"error", err,
		)

		if attempt == attempts {
			break
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	return zero, lastErr
}
