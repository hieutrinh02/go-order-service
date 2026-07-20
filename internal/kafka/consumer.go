package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

const consumerPollTimeoutMilliseconds = 500

type Record struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
}

type Handler func(context.Context, Record) error

type Consumer struct {
	client *confluentkafka.Consumer
	logger *slog.Logger
}

func NewConsumer(
	bootstrapServers string,
	groupID string,
	logger *slog.Logger,
) (*Consumer, error) {
	if bootstrapServers == "" {
		return nil, errors.New(
			"kafka bootstrap servers must not be empty",
		)
	}

	if groupID == "" {
		return nil, errors.New(
			"kafka consumer group must not be empty",
		)
	}

	if logger == nil {
		return nil, errors.New(
			"kafka consumer logger must not be nil",
		)
	}

	client, err := confluentkafka.NewConsumer(
		&confluentkafka.ConfigMap{
			"bootstrap.servers":        bootstrapServers,
			"group.id":                 groupID,
			"client.id":                "notification-consumer",
			"enable.auto.commit":       false,
			"enable.auto.offset.store": false,
			"auto.offset.reset":        "earliest",
			"isolation.level":          "read_committed",
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create kafka consumer: %w",
			err,
		)
	}

	return &Consumer{
		client: client,
		logger: logger,
	}, nil
}

func (c *Consumer) Run(
	ctx context.Context,
	topic string,
	handler Handler,
) error {
	if topic == "" {
		return errors.New("kafka topic must not be empty")
	}

	if handler == nil {
		return errors.New("kafka handler must not be nil")
	}

	if err := c.client.SubscribeTopics(
		[]string{topic},
		nil,
	); err != nil {
		return fmt.Errorf(
			"subscribe to kafka topic %q: %w",
			topic,
			err,
		)
	}

	c.logger.Info(
		"kafka consumer subscribed",
		"topic", topic,
	)

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		event := c.client.Poll(
			consumerPollTimeoutMilliseconds,
		)

		switch current := event.(type) {
		case nil:
			continue

		case *confluentkafka.Message:
			recordTopic := topic
			if current.TopicPartition.Topic != nil {
				recordTopic = *current.TopicPartition.Topic
			}

			record := Record{
				Topic:     recordTopic,
				Partition: current.TopicPartition.Partition,
				Offset:    int64(current.TopicPartition.Offset),
				Key:       current.Key,
				Value:     current.Value,
			}

			if err := handler(ctx, record); err != nil {
				if ctx.Err() != nil {
					return nil
				}

				return fmt.Errorf(
					"handle Kafka record %s[%d]@%d: %w",
					record.Topic,
					record.Partition,
					record.Offset,
					err,
				)
			}

			// If a shutdown occurs after the DB commit but before
			// the offset commit, the record will be re-read. An
			// idempotent handler will ignore the duplicate.
			if ctx.Err() != nil {
				return nil
			}

			if _, err := c.client.CommitMessage(current); err != nil {
				return fmt.Errorf(
					"commit Kafka record %s[%d]@%d: %w",
					record.Topic,
					record.Partition,
					record.Offset,
					err,
				)
			}

			c.logger.Debug(
				"kafka offset committed",
				"topic", record.Topic,
				"partition", record.Partition,
				"offset", record.Offset,
			)

		case confluentkafka.Error:
			if ctx.Err() != nil {
				return nil
			}

			if current.IsFatal() {
				return fmt.Errorf(
					"fatal kafka consumer error: %w",
					current,
				)
			}

			c.logger.Warn(
				"recoverable kafka consumer event",
				"code", current.Code().String(),
				"error", current,
			)
		}
	}
}

func (c *Consumer) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf(
			"close kafka consumer: %w",
			err,
		)
	}

	return nil
}
