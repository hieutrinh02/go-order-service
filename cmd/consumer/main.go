package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/appstart"
	"github.com/hieutrinh02/go-order-service/internal/config"
	eventconsumer "github.com/hieutrinh02/go-order-service/internal/consumer"
	"github.com/hieutrinh02/go-order-service/internal/db"
	kafkaclient "github.com/hieutrinh02/go-order-service/internal/kafka"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
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

	// Create store
	appStore := store.New(dbPool)

	// Create event handler
	eventHandler := eventconsumer.NewEventHandler(
		appStore,
		logger,
	)

	kafkaConsumer, err := kafkaclient.NewConsumer(
		cfg.KafkaBootstrapServers,
		cfg.KafkaConsumerGroup,
		logger,
	)
	if err != nil {
		logger.Error(
			"failed to create kafka consumer",
			"error", err,
		)
		os.Exit(1)
	}

	handleRecord := func(
		ctx context.Context,
		record kafkaclient.Record,
	) error {
		err := eventHandler.Handle(
			ctx,
			record.Topic,
			record.Value,
		)
		if err == nil {
			return nil
		}

		if errors.Is(err, eventconsumer.ErrInvalidMessage) {
			logger.Error(
				"discarding invalid kafka record",
				"topic", record.Topic,
				"partition", record.Partition,
				"offset", record.Offset,
				"key", string(record.Key),
				"error", err,
			)

			// Return nil so the Kafka consumer commits the poison
			// record and does not get stuck at this offset.
			return nil
		}

		// DB or infrastructure error: return error to Run()
		// stop without committing the offset.
		return err
	}

	go metrics.RunServer(runCtx, logger, cfg.ConsumerMetricsPort)

	logger.Info(
		"consumer started",
		"broker", "kafka",
		"topic", cfg.KafkaTopic,
		"group_id", cfg.KafkaConsumerGroup,
	)

	// Run consumer
	runErr := kafkaConsumer.Run(
		runCtx,
		cfg.KafkaTopic,
		handleRecord,
	)

	closeErr := kafkaConsumer.Close()

	if runErr != nil {
		logger.Error(
			"kafka consumer failed",
			"error", runErr,
		)
	}

	if closeErr != nil {
		logger.Error(
			"failed to close kafka consumer",
			"error", closeErr,
		)
	} else {
		logger.Info("kafka consumer closed")
	}

	if runErr != nil || closeErr != nil {
		os.Exit(1)
	}
}
