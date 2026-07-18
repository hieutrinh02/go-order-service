package kafka

import (
	"context"
	"errors"
	"fmt"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

const producerFlushTimeoutMilliseconds = 5000

type Producer struct {
	client *confluentkafka.Producer
}

func NewProducer(bootstrapServers string) (*Producer, error) {
	if bootstrapServers == "" {
		return nil, errors.New("kafka bootstrap servers must not be empty")
	}

	client, err := confluentkafka.NewProducer(&confluentkafka.ConfigMap{
		"bootstrap.servers":  bootstrapServers,
		"client.id":          "order-outbox-publisher",
		"acks":               "all",
		"enable.idempotence": true,
		"message.timeout.ms": 10000,
	})
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Producer{
		client: client,
	}, nil
}

func (p *Producer) Publish(
	ctx context.Context,
	topic string,
	key []byte,
	payload []byte,
) error {
	if topic == "" {
		return errors.New("kafka topic must not be empty")
	}

	if len(key) == 0 {
		return errors.New("kafka message key must not be empty")
	}

	delivery := make(chan confluentkafka.Event, 1)

	err := p.client.Produce(
		&confluentkafka.Message{
			TopicPartition: confluentkafka.TopicPartition{
				Topic:     &topic,
				Partition: confluentkafka.PartitionAny,
			},
			Key:   key,
			Value: payload,
		},
		delivery,
	)
	if err != nil {
		return fmt.Errorf("enqueue kafka message: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case event := <-delivery:
		message, ok := event.(*confluentkafka.Message)
		if !ok {
			return fmt.Errorf(
				"unexpected kafka delivery event: %T",
				event,
			)
		}

		if message.TopicPartition.Error != nil {
			return fmt.Errorf(
				"deliver kafka message: %w",
				message.TopicPartition.Error,
			)
		}

		return nil
	}
}

func (p *Producer) Close() error {
	remaining := p.client.Flush(producerFlushTimeoutMilliseconds)
	p.client.Close()

	if remaining > 0 {
		return fmt.Errorf(
			"close kafka producer with %d undelivered messages",
			remaining,
		)
	}

	return nil
}
