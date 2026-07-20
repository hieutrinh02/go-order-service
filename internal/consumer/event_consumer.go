package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/jackc/pgx/v5"
)

const (
	ConsumerNameNotification = "notification-consumer"

	NotificationChannelEmail = "email"
	NotificationStatusSent   = "sent"
)

var ErrInvalidMessage = errors.New("invalid event message")

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
	err := c.broker.Subscribe(
		">",
		func(subject string, payload []byte) {
			if err := c.Handle(ctx, subject, payload); err != nil {
				c.logger.Error(
					"failed to handle event",
					"source", subject,
					"error", err,
				)
			}
		},
	)
	if err != nil {
		return err
	}

	c.logger.Info("consumer subscribed", "subject", ">")

	<-ctx.Done()

	c.logger.Info("consumer stopped")
	return nil
}

func (c *EventConsumer) Handle(
	ctx context.Context,
	source string,
	payload []byte,
) error {
	var message OutboxMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return fmt.Errorf(
			"%w: decode envelope from %q: %v",
			ErrInvalidMessage,
			source,
			err,
		)
	}

	if message.ID == "" {
		return fmt.Errorf(
			"%w: message id is required",
			ErrInvalidMessage,
		)
	}

	if _, err := uuid.Parse(message.ID); err != nil {
		return fmt.Errorf(
			"%w: invalid message id %q",
			ErrInvalidMessage,
			message.ID,
		)
	}

	if message.EventType == "" {
		return fmt.Errorf(
			"%w: event type is required",
			ErrInvalidMessage,
		)
	}

	var eventData eventPayload
	if err := json.Unmarshal(message.Payload, &eventData); err != nil {
		return fmt.Errorf(
			"%w: decode payload for event %q: %v",
			ErrInvalidMessage,
			message.ID,
			err,
		)
	}

	if eventData.UserID == "" {
		return fmt.Errorf(
			"%w: user_id is required for event %q",
			ErrInvalidMessage,
			message.ID,
		)
	}

	if _, err := uuid.Parse(eventData.UserID); err != nil {
		return fmt.Errorf(
			"%w: invalid user_id for event %q",
			ErrInvalidMessage,
			message.ID,
		)
	}

	duplicate := false

	err := c.store.WithTx(ctx, func(txStore *store.Store) error {
		_, err := txStore.TryCreateProcessedEvent(
			ctx,
			message.ID,
			ConsumerNameNotification,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				duplicate = true
				return nil
			}

			return err
		}

		_, err = txStore.CreateNotificationDelivery(
			ctx,
			store.CreateNotificationDeliveryParams{
				ID:        uuid.NewString(),
				EventID:   message.ID,
				Channel:   NotificationChannelEmail,
				Recipient: eventData.UserID,
				Status:    NotificationStatusSent,
			},
		)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf(
			"process event %q: %w",
			message.ID,
			err,
		)
	}

	if duplicate {
		c.logger.Info(
			"event already processed",
			"event_id", message.ID,
			"event_type", message.EventType,
		)

		metrics.ConsumerEventsDuplicateTotal.
			WithLabelValues(message.EventType).
			Inc()

		return nil
	}

	c.logger.Info(
		"event processed",
		"event_id", message.ID,
		"event_type", message.EventType,
		"recipient", eventData.UserID,
	)

	metrics.ConsumerEventsProcessedTotal.
		WithLabelValues(message.EventType).
		Inc()

	return nil
}
