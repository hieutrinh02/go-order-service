package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/appstart"
	"github.com/hieutrinh02/go-order-service/internal/broker"
	"github.com/hieutrinh02/go-order-service/internal/config"
	"github.com/hieutrinh02/go-order-service/internal/db"
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

	// Connect NATS
	natsBroker, err := appstart.Retry(runCtx, logger, "nats", 12, 5*time.Second, func(ctx context.Context) (*broker.NATS, error) {
		return broker.Connect(cfg.NATSURL)
	})
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

	go metrics.RunServer(runCtx, logger, cfg.PublisherMetricsPort)

	logger.Info("publisher started",
		"batch_size", cfg.PublisherBatchSize,
		"poll_interval", cfg.PublisherPollInterval.String(),
	)

	// Run publisher
	publisher.Run(runCtx)

	if err := natsBroker.Drain(); err != nil {
		logger.Error("failed to drain nats", "error", err)
	} else {
		logger.Info("nats drained")
	}
}
