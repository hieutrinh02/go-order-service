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

func newUserResponse(user sqlc.User) userResponse {
	return userResponse{
		ID:        user.ID.String(),
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Time.UTC(),
		UpdatedAt: user.UpdatedAt.Time.UTC(),
	}
}
