package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/ratelimit"
)

func rateLimitMiddleware(limiter *ratelimit.Limiter, scope string, limit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := clientIPFromRequest(r)

			result, err := limiter.Allow(r.Context(), scope, clientIP, limit, time.Minute)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "rate limiter unavailable")
				return
			}

			w.Header().Set("RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(result.Remaining))

			if !result.Allowed {
				metrics.RateLimitBlockedTotal.WithLabelValues(scope).Inc()
				retryAfter := int(result.RetryAfter.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}

				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			metrics.RateLimitAllowedTotal.WithLabelValues(scope).Inc()
			next.ServeHTTP(w, r)
		})
	}
}

func clientIPFromRequest(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
