package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/service"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	User        userResponse `json:"user"`
}

const refreshTokenCookieName = "refresh_token"

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := s.authService.RegisterUser(r.Context(), service.RegisterUserParams{
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeJSONError(w, http.StatusBadRequest, "invalid register input")
		case errors.Is(err, service.ErrEmailAlreadyExists):
			writeJSONError(w, http.StatusConflict, "email already exists")
		default:
			s.logger.Error("failed to register user", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to register user")
		}

		return
	}

	writeJSON(w, http.StatusCreated, newUserResponse(user))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, err := s.authService.Login(r.Context(), service.LoginParams{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeJSONError(w, http.StatusBadRequest, "invalid login input")
		case errors.Is(err, service.ErrInvalidCredentials):
			writeJSONError(w, http.StatusUnauthorized, "invalid email or password")
		default:
			s.logger.Error("failed to login user", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to login user")
		}

		return
	}

	s.setRefreshTokenCookie(w, result.RefreshToken, result.RefreshTokenExpiresAt)

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		User:        newUserResponse(result.User),
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	result, err := s.authService.RefreshAccessToken(r.Context(), cookie.Value)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRefreshToken):
			writeJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		default:
			s.logger.Error("failed to refresh access token", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to refresh access token")
		}

		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		User:        newUserResponse(result.User),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		s.clearRefreshTokenCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.authService.Logout(r.Context(), cookie.Value); err != nil {
		if !errors.Is(err, service.ErrInvalidRefreshToken) {
			s.logger.Error("failed to logout user", "error", err)
		}
	}

	s.clearRefreshTokenCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := s.authService.GetUser(r.Context(), claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, newUserResponse(user))
}

func newUserResponse(user sqlc.User) userResponse {
	return userResponse{
		ID:        user.ID.String(),
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Time.UTC(),
		UpdatedAt: user.UpdatedAt.Time.UTC(),
	}
}

func (s *Server) setRefreshTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    token,
		Path:     "/auth",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearRefreshTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     "/auth",
		Expires:  time.Now().UTC().Add(-time.Hour),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
