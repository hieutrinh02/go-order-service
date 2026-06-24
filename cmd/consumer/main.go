package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hieutrinh02/go-order-service/internal/broker"
	"github.com/hieutrinh02/go-order-service/internal/config"
	eventconsumer "github.com/hieutrinh02/go-order-service/internal/consumer"
	"github.com/hieutrinh02/go-order-service/internal/db"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/store"
)

func main() {
	// Load config and create logger
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Register metrics
	metrics.Register()

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

	// Create event consumer
	consumer := eventconsumer.NewEventConsumer(appStore, natsBroker, logger)

	// Stop context
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go metrics.RunServer(runCtx, logger, cfg.ConsumerMetricsPort)

	logger.Info("consumer started")

	// Run consumer
	if err := consumer.Run(runCtx); err != nil {
		logger.Error("consumer failed", "error", err)
		os.Exit(1)
	}

	if err := natsBroker.Drain(); err != nil {
		logger.Error("failed to drain nats", "error", err)
	} else {
		logger.Info("nats drained")
	}
}
