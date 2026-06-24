package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hieutrinh02/go-order-service/internal/broker"
	"github.com/hieutrinh02/go-order-service/internal/config"
	"github.com/hieutrinh02/go-order-service/internal/db"
	outboxpublisher "github.com/hieutrinh02/go-order-service/internal/publisher"
	"github.com/hieutrinh02/go-order-service/internal/store"
)

func main() {
	// Load config and create logger
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Database pool
	ctx := context.Background()
	dbPool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("connected to database")

	// Connect NATS
	natsBroker, err := broker.Connect(cfg.NATSURL)
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer natsBroker.Close()

	// Create store
	appStore := store.New(dbPool)

	// Create outbox publisher
	publisher := outboxpublisher.NewOutboxPublisher(appStore, natsBroker, logger, outboxpublisher.Config{
		BatchSize:    cfg.PublisherBatchSize,
		PollInterval: cfg.PublisherPollInterval,
	})

	// Stop context
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("publisher started",
		"batch_size", cfg.PublisherBatchSize,
		"poll_interval", cfg.PublisherPollInterval.String(),
	)

	// Run publisher
	publisher.Run(runCtx)
}
