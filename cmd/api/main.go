package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/hieutrinh02/go-order-service/internal/api"
	"github.com/hieutrinh02/go-order-service/internal/auth"
	"github.com/hieutrinh02/go-order-service/internal/config"
	"github.com/hieutrinh02/go-order-service/internal/db"
	"github.com/hieutrinh02/go-order-service/internal/service"
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

	// Create store and services
	appStore := store.New(dbPool)
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authService := service.NewAuthService(appStore, tokenManager, cfg.RefreshTokenTTL)

	// Create router and address
	router := api.NewRouter(api.RouterConfig{
		Logger:       logger,
		DBPool:       dbPool,
		AuthService:  authService,
		CookieSecure: cfg.CookieSecure,
	})
	addr := ":" + cfg.Port

	// Create HTTP server
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	logger.Info("api listening", "addr", addr)

	// Listen and serve
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("api server failed", "error", err)
		os.Exit(1)
	}
}
