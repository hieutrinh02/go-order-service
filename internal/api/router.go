package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hieutrinh02/go-order-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	logger      *slog.Logger
	dbPool      *pgxpool.Pool
	authService *service.AuthService
}

type RouterConfig struct {
	Logger      *slog.Logger
	DBPool      *pgxpool.Pool
	AuthService *service.AuthService
}

func NewRouter(cfg RouterConfig) http.Handler {
	server := &Server{
		logger:      cfg.Logger,
		dbPool:      cfg.DBPool,
		authService: cfg.AuthService,
	}

	r := chi.NewRouter()

	r.Get("/healthz", server.handleHealthz)
	r.Get("/readyz", server.handleReadyz)

	r.Post("/auth/register", server.handleRegister)
	r.Post("/auth/login", server.handleLogin)

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.dbPool.Ping(r.Context()); err != nil {
		s.logger.Error("database readiness check failed", "error", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
