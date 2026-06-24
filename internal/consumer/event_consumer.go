package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/jackc/pgx/v5"
)

const (
	ConsumerNameNotification = "notification-consumer"

	NotificationChannelEmail = "email"
	NotificationStatusSent   = "sent"
)

type Subscriber interface {
	Subscribe(subject string, handler func(subject string, payload []byte)) error
}

type EventConsumer struct {
	store  *store.Store
	broker Subscriber
	logger *slog.Logger
}

type OutboxMessage struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
}

type eventPayload struct {
	UserID string `json:"user_id"`
}

func NewEventConsumer(store *store.Store, broker Subscriber, logger *slog.Logger) *EventConsumer {
	return &EventConsumer{
		store:  store,
		broker: broker,
		logger: logger,
	}
}

func (c *EventConsumer) Run(ctx context.Context) error {
	if err := c.broker.Subscribe(">", c.handleMessage); err != nil {
		return err
	}

	c.logger.Info("consumer subscribed", "subject", ">")

	<-ctx.Done()
	c.logger.Info("consumer stopped")
	return nil
}

func (c *EventConsumer) handleMessage(subject string, payload []byte) {
	ctx := context.Background()

	var message OutboxMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		c.logger.Error("failed to decode message", "subject", subject, "error", err)
		return
	}

	if message.ID == "" || message.EventType == "" {
		c.logger.Error("invalid message", "subject", subject)
		return
	}

	var eventData eventPayload
	if err := json.Unmarshal(message.Payload, &eventData); err != nil {
		c.logger.Error("failed to decode event payload",
			"event_id", message.ID,
			"event_type", message.EventType,
			"error", err,
		)
		return
	}

	if eventData.UserID == "" {
		c.logger.Error("event payload missing user_id",
			"event_id", message.ID,
			"event_type", message.EventType,
		)
		return
	}

	err := c.store.WithTx(ctx, func(txStore *store.Store) error {
		if _, err := txStore.TryCreateProcessedEvent(ctx, message.ID, ConsumerNameNotification); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.logger.Info("event already processed",
					"event_id", message.ID,
					"event_type", message.EventType,
				)
				return nil
			}

			return err
		}

		_, err := txStore.CreateNotificationDelivery(ctx, store.CreateNotificationDeliveryParams{
			ID:        uuid.NewString(),
			EventID:   message.ID,
			Channel:   NotificationChannelEmail,
			Recipient: eventData.UserID,
			Status:    NotificationStatusSent,
		})
		if err != nil {
			return err
		}

		c.logger.Info("event processed",
			"event_id", message.ID,
			"event_type", message.EventType,
			"recipient", eventData.UserID,
		)

		return nil
	})
	if err != nil {
		c.logger.Error("failed to process event",
			"event_id", message.ID,
			"event_type", message.EventType,
			"error", err,
		)
	}
}
