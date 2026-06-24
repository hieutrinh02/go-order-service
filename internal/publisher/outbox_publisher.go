package publisher

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/store"
)

type Broker interface {
	Publish(subject string, payload []byte) error
}

type Config struct {
	BatchSize    int32
	PollInterval time.Duration
}

type OutboxPublisher struct {
	store  *store.Store
	broker Broker
	logger *slog.Logger
	config Config
}

type OutboxMessage struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
}

func NewOutboxPublisher(store *store.Store, broker Broker, logger *slog.Logger, config Config) *OutboxPublisher {
	return &OutboxPublisher{
		store:  store,
		broker: broker,
		logger: logger,
		config: config,
	}
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := p.PublishBatch(ctx); err != nil {
			p.logger.Error("failed to publish outbox batch", "error", err)
		}

		select {
		case <-ctx.Done():
			p.logger.Info("outbox publisher stopped")
			return
		case <-ticker.C:
		}
	}
}

func (p *OutboxPublisher) PublishBatch(ctx context.Context) error {
	return p.store.WithTx(ctx, func(txStore *store.Store) error {
		events, err := txStore.ClaimOutboxEvents(ctx, p.config.BatchSize)
		if err != nil {
			return err
		}

		for _, event := range events {
			message, err := json.Marshal(OutboxMessage{
				ID:            event.ID.String(),
				AggregateType: event.AggregateType,
				AggregateID:   event.AggregateID.String(),
				EventType:     event.EventType,
				Payload:       event.Payload,
			})
			if err != nil {
				return err
			}

			if err := p.broker.Publish(event.EventType, message); err != nil {
				if _, markErr := txStore.MarkOutboxEventFailed(ctx, event.ID.String(), err.Error()); markErr != nil {
					return markErr
				}

				p.logger.Error("failed to publish outbox event",
					"event_id", event.ID.String(),
					"event_type", event.EventType,
					"error", err,
				)

				metrics.OutboxEventsFailedTotal.WithLabelValues(event.EventType).Inc()

				continue
			}

			if _, err := txStore.MarkOutboxEventPublished(ctx, event.ID.String()); err != nil {
				return err
			}

			p.logger.Info("published outbox event",
				"event_id", event.ID.String(),
				"event_type", event.EventType,
			)

			metrics.OutboxEventsPublishedTotal.WithLabelValues(event.EventType).Inc()
		}

		return nil
	})
}
