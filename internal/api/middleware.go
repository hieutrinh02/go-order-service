package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/hieutrinh02/go-order-service/internal/auth"
)

type contextKey string

const authClaimsContextKey contextKey = "auth_claims"

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.Fields(authHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeJSONError(w, http.StatusUnauthorized, "invalid authorization header")
			return
		}

		tokenString := parts[1]

		claims, err := s.authService.ParseAccessToken(tokenString)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid access token")
			return
		}

		ctx := context.WithValue(r.Context(), authClaimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func authClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(authClaimsContextKey).(*auth.Claims)
	return claims, ok
}
