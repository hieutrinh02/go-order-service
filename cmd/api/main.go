package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/api"
	"github.com/hieutrinh02/go-order-service/internal/appstart"
	"github.com/hieutrinh02/go-order-service/internal/auth"
	"github.com/hieutrinh02/go-order-service/internal/cache"
	"github.com/hieutrinh02/go-order-service/internal/config"
	"github.com/hieutrinh02/go-order-service/internal/db"
	"github.com/hieutrinh02/go-order-service/internal/distributedlock"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/ratelimit"
	"github.com/hieutrinh02/go-order-service/internal/service"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load config and create logger
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Register metrics
	metrics.Register()

	// Database pool
	ctx := context.Background()
	dbPool, err := appstart.Retry(ctx, logger, "database", 12, 5*time.Second, func(ctx context.Context) (*pgxpool.Pool, error) {
		return db.Open(ctx, cfg.DatabaseURL)
	})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("connected to database")

	// Redis
	redisClient, err := appstart.Retry(ctx, logger, "redis", 12, 5*time.Second, func(ctx context.Context) (*redis.Client, error) {
		return cache.OpenRedis(ctx, cfg.RedisURL)
	})
	if err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	logger.Info("connected to redis")

	// Create store and services
	appStore := store.New(dbPool)
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authService := service.NewAuthService(appStore, tokenManager, cfg.RefreshTokenTTL)
	orderService := service.NewOrderService(appStore)
	rateLimiter := ratelimit.New(redisClient)
	lockManager := distributedlock.NewManager(redisClient, cfg.OrderLockTTL)

	// Create router and address
	router := api.NewRouter(api.RouterConfig{
		Logger:                          logger,
		DBPool:                          dbPool,
		AuthService:                     authService,
		OrderService:                    orderService,
		CookieSecure:                    cfg.CookieSecure,
		LockManager:                     lockManager,
		RateLimiter:                     rateLimiter,
		RateLimitEnabled:                cfg.RateLimitEnabled,
		RateLimitRequestsPerMinute:      cfg.RateLimitRequestsPerMinute,
		AuthRateLimitRequestsPerMinute:  cfg.AuthRateLimitRequestsPerMinute,
		LoginRateLimitRequestsPerMinute: cfg.LoginRateLimitRequestsPerMinute,
		CORSAllowedOrigin:               cfg.CORSAllowedOrigin,
	})
	addr := ":" + cfg.Port

	// Create HTTP server
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Listen and serve in goroutine
	go func() {
		logger.Info("api listening", "addr", addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	logger.Info("shutting down api server")

	// Stop accepting new requests and wait for in-flight requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("api server stopped")
}
