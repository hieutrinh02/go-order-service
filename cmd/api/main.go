package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/hieutrinh02/go-order-service/internal/api"
	"github.com/hieutrinh02/go-order-service/internal/config"
)

func main() {
	// Load config and create logger
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Create router and address
	router := api.NewRouter(api.RouterConfig{
		Logger: logger,
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
