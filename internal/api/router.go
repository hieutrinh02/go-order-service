package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	logger *slog.Logger
}

type RouterConfig struct {
	Logger *slog.Logger
}

func NewRouter(cfg RouterConfig) http.Handler {
	server := &Server{
		logger: cfg.Logger,
	}

	r := chi.NewRouter()

	r.Get("/healthz", server.handleHealthz)

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
