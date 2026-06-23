package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/auth"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidInput       = errors.New("invalid input")
)

type AuthService struct {
	store        *store.Store
	tokenManager *auth.TokenManager
}

type RegisterUserParams struct {
	Email    string
	Password string
	Role     string
}

type LoginParams struct {
	Email    string
	Password string
}

type LoginResult struct {
	User        sqlc.User
	AccessToken string
}

func NewAuthService(store *store.Store, tokenManager *auth.TokenManager) *AuthService {
	return &AuthService{
		store:        store,
		tokenManager: tokenManager,
	}
}

func (s *AuthService) RegisterUser(ctx context.Context, params RegisterUserParams) (sqlc.User, error) {
	email := strings.TrimSpace(strings.ToLower(params.Email))
	if email == "" || params.Password == "" {
		return sqlc.User{}, ErrInvalidInput
	}

	role := params.Role
	if role == "" {
		role = "customer"
	}

	if role != "customer" && role != "admin" {
		return sqlc.User{}, ErrInvalidInput
	}

	passwordHash, err := auth.HashPassword(params.Password)
	if err != nil {
		return sqlc.User{}, err
	}

	user, err := s.store.CreateUser(ctx, store.CreateUserParams{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return sqlc.User{}, ErrEmailAlreadyExists
		}

		return sqlc.User{}, err
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, params LoginParams) (LoginResult, error) {
	email := strings.TrimSpace(strings.ToLower(params.Email))
	if email == "" || params.Password == "" {
		return LoginResult{}, ErrInvalidInput
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	if !auth.CheckPasswordHash(params.Password, user.PasswordHash) {
		return LoginResult{}, ErrInvalidCredentials
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:        user,
		AccessToken: accessToken,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	return false
}
