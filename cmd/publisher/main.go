package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/appstart"
	"github.com/hieutrinh02/go-order-service/internal/config"
	"github.com/hieutrinh02/go-order-service/internal/db"
	kafkaclient "github.com/hieutrinh02/go-order-service/internal/kafka"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
	outboxpublisher "github.com/hieutrinh02/go-order-service/internal/publisher"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Load config and create logger
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Register metrics
	metrics.Register()

	// Stop context
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Database pool
	dbPool, err := appstart.Retry(runCtx, logger, "database", 12, 5*time.Second, func(ctx context.Context) (*pgxpool.Pool, error) {
		return db.Open(ctx, cfg.DatabaseURL)
	})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("connected to database")

	// Connect Kafka
	kafkaProducer, err := appstart.Retry(
		runCtx,
		logger,
		"kafka",
		12,
		5*time.Second,
		func(ctx context.Context) (*kafkaclient.Producer, error) {
			producer, err := kafkaclient.NewProducer(
				cfg.KafkaBootstrapServers,
			)
			if err != nil {
				return nil, err
			}

			if err := producer.CheckTopic(
				ctx,
				cfg.KafkaTopic,
				5*time.Second,
			); err != nil {
				_ = producer.Close()
				return nil, err
			}

			return producer, nil
		},
	)
	if err != nil {
		logger.Error("failed to connect to kafka", "error", err)
		os.Exit(1)
	}

	defer func() {
		if err := kafkaProducer.Close(); err != nil {
			logger.Error("failed to close kafka producer", "error", err)
		} else {
			logger.Info("kafka producer closed")
		}
	}()

	logger.Info(
		"connected to kafka",
		"bootstrap_servers", cfg.KafkaBootstrapServers,
		"topic", cfg.KafkaTopic,
	)

	// Create store
	appStore := store.New(dbPool)

	// Create outbox publisher
	publisher := outboxpublisher.NewOutboxPublisher(appStore, kafkaProducer, logger, outboxpublisher.Config{
		Topic:        cfg.KafkaTopic,
		BatchSize:    cfg.PublisherBatchSize,
		PollInterval: cfg.PublisherPollInterval,
	})

	go metrics.RunServer(runCtx, logger, cfg.PublisherMetricsPort)

	logger.Info("publisher started",
		"batch_size", cfg.PublisherBatchSize,
		"poll_interval", cfg.PublisherPollInterval.String(),
	)

	// Run publisher
	publisher.Run(runCtx)
}
