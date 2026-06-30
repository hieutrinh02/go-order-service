package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hieutrinh02/go-order-service/internal/distributedlock"
	"github.com/hieutrinh02/go-order-service/internal/ratelimit"
	"github.com/hieutrinh02/go-order-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	logger       *slog.Logger
	dbPool       *pgxpool.Pool
	authService  *service.AuthService
	orderService *service.OrderService
	cookieSecure bool
	lockManager  *distributedlock.Manager
}

type RouterConfig struct {
	Logger                          *slog.Logger
	DBPool                          *pgxpool.Pool
	AuthService                     *service.AuthService
	OrderService                    *service.OrderService
	CookieSecure                    bool
	LockManager                     *distributedlock.Manager
	RateLimiter                     *ratelimit.Limiter
	RateLimitEnabled                bool
	RateLimitRequestsPerMinute      int
	AuthRateLimitRequestsPerMinute  int
	LoginRateLimitRequestsPerMinute int
}

func NewRouter(cfg RouterConfig) http.Handler {
	server := &Server{
		logger:       cfg.Logger,
		dbPool:       cfg.DBPool,
		authService:  cfg.AuthService,
		orderService: cfg.OrderService,
		cookieSecure: cfg.CookieSecure,
		lockManager:  cfg.LockManager,
	}

	r := chi.NewRouter()
	r.Use(metricsMiddleware)

	r.Get("/healthz", server.handleHealthz)
	r.Get("/readyz", server.handleReadyz)
	r.Handle("/metrics", promhttp.Handler())

	app := r.Group(nil)
	var rateLimiter *ratelimit.Limiter
	if cfg.RateLimitEnabled {
		rateLimiter = cfg.RateLimiter
	}
	if rateLimiter != nil {
		app.Use(rateLimitMiddleware(rateLimiter, "global", cfg.RateLimitRequestsPerMinute))
	}

	app.With(rateLimitMiddleware(rateLimiter, "auth_register", cfg.AuthRateLimitRequestsPerMinute)).
		Post("/auth/register", server.handleRegister)
	app.With(rateLimitMiddleware(rateLimiter, "auth_login", cfg.LoginRateLimitRequestsPerMinute)).
		Post("/auth/login", server.handleLogin)
	app.Post("/auth/refresh", server.handleRefresh)
	app.Post("/auth/logout", server.handleLogout)
	app.Get("/me", server.requireAuth(server.handleMe))

	app.Post("/orders", server.requireAuth(server.handleCreateOrder))
	app.Get("/orders", server.requireAuth(server.handleListOrders))
	app.Get("/orders/{id}", server.requireAuth(server.handleGetOrder))
	app.Post("/orders/{id}/pay", server.requireAuth(server.handlePayOrder))
	app.Post("/orders/{id}/cancel", server.requireAuth(server.handleCancelOrder))

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
