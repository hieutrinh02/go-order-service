package store

import (
	"context"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	queries *sqlc.Queries
}

type CreateUserParams struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
}

type CreateRefreshTokenParams struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		queries: sqlc.New(pool),
	}
}

func (s *Store) CreateUser(ctx context.Context, params CreateUserParams) (sqlc.User, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.User{}, err
	}

	return s.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:           id,
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		Role:         params.Role,
	})
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (sqlc.User, error) {
	return s.queries.GetUserByEmail(ctx, email)
}

func (s *Store) GetUser(ctx context.Context, id string) (sqlc.User, error) {
	userID := pgtype.UUID{}
	if err := userID.Scan(id); err != nil {
		return sqlc.User{}, err
	}

	return s.queries.GetUser(ctx, userID)
}

func (s *Store) CreateRefreshToken(ctx context.Context, params CreateRefreshTokenParams) (sqlc.RefreshToken, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.RefreshToken{}, err
	}

	userID := pgtype.UUID{}
	if err := userID.Scan(params.UserID); err != nil {
		return sqlc.RefreshToken{}, err
	}

	expiresAt := pgtype.Timestamptz{
		Time:  params.ExpiresAt,
		Valid: true,
	}

	return s.queries.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		ID:        id,
		UserID:    userID,
		TokenHash: params.TokenHash,
		ExpiresAt: expiresAt,
	})
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (sqlc.RefreshToken, error) {
	return s.queries.GetRefreshTokenByHash(ctx, tokenHash)
}

func (s *Store) RevokeRefreshToken(ctx context.Context, id string) (sqlc.RefreshToken, error) {
	tokenID := pgtype.UUID{}
	if err := tokenID.Scan(id); err != nil {
		return sqlc.RefreshToken{}, err
	}

	return s.queries.RevokeRefreshToken(ctx, tokenID)
}
