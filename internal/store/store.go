package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool    *pgxpool.Pool
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

type CreateOrderParams struct {
	ID          string
	UserID      string
	Status      string
	AmountCents int64
	Currency    string
	Description string
}

type CreateIdempotencyKeyParams struct {
	ID           string
	UserID       string
	Key          string
	Method       string
	Path         string
	RequestHash  string
	ResponseBody json.RawMessage
	StatusCode   int32
	ResourceType string
	ResourceID   string
}

type CreateOutboxEventParams struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
}

type CreatePaymentParams struct {
	ID            string
	OrderID       string
	Status        string
	AmountCents   int64
	Provider      string
	ProviderRef   string
	FailureReason string
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
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

func (s *Store) CreateOrder(ctx context.Context, params CreateOrderParams) (sqlc.Order, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.Order{}, err
	}

	userID := pgtype.UUID{}
	if err := userID.Scan(params.UserID); err != nil {
		return sqlc.Order{}, err
	}

	description := pgtype.Text{
		String: params.Description,
		Valid:  params.Description != "",
	}

	return s.queries.CreateOrder(ctx, sqlc.CreateOrderParams{
		ID:          id,
		UserID:      userID,
		Status:      params.Status,
		AmountCents: params.AmountCents,
		Currency:    params.Currency,
		Description: description,
	})
}

func (s *Store) GetOrder(ctx context.Context, id string) (sqlc.Order, error) {
	orderID := pgtype.UUID{}
	if err := orderID.Scan(id); err != nil {
		return sqlc.Order{}, err
	}

	return s.queries.GetOrder(ctx, orderID)
}

func (s *Store) ListOrdersByUser(ctx context.Context, userID string) ([]sqlc.Order, error) {
	id := pgtype.UUID{}
	if err := id.Scan(userID); err != nil {
		return nil, err
	}

	return s.queries.ListOrdersByUser(ctx, id)
}

func (s *Store) ListOrders(ctx context.Context) ([]sqlc.Order, error) {
	return s.queries.ListOrders(ctx)
}

func (s *Store) CreateIdempotencyKey(ctx context.Context, params CreateIdempotencyKeyParams) (sqlc.IdempotencyKey, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.IdempotencyKey{}, err
	}

	userID := pgtype.UUID{}
	if err := userID.Scan(params.UserID); err != nil {
		return sqlc.IdempotencyKey{}, err
	}

	resourceID := pgtype.UUID{}
	if params.ResourceID != "" {
		if err := resourceID.Scan(params.ResourceID); err != nil {
			return sqlc.IdempotencyKey{}, err
		}
	}

	return s.queries.CreateIdempotencyKey(ctx, sqlc.CreateIdempotencyKeyParams{
		ID:           id,
		UserID:       userID,
		Key:          params.Key,
		Method:       params.Method,
		Path:         params.Path,
		RequestHash:  params.RequestHash,
		ResponseBody: params.ResponseBody,
		StatusCode:   pgtype.Int4{Int32: params.StatusCode, Valid: params.StatusCode > 0},
		ResourceType: pgtype.Text{String: params.ResourceType, Valid: params.ResourceType != ""},
		ResourceID:   resourceID,
	})
}

func (s *Store) GetIdempotencyKey(ctx context.Context, userID string, key string) (sqlc.IdempotencyKey, error) {
	id := pgtype.UUID{}
	if err := id.Scan(userID); err != nil {
		return sqlc.IdempotencyKey{}, err
	}

	return s.queries.GetIdempotencyKey(ctx, sqlc.GetIdempotencyKeyParams{
		UserID: id,
		Key:    key,
	})
}

func (s *Store) CreateOutboxEvent(ctx context.Context, params CreateOutboxEventParams) (sqlc.OutboxEvent, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.OutboxEvent{}, err
	}

	aggregateID := pgtype.UUID{}
	if err := aggregateID.Scan(params.AggregateID); err != nil {
		return sqlc.OutboxEvent{}, err
	}

	return s.queries.CreateOutboxEvent(ctx, sqlc.CreateOutboxEventParams{
		ID:            id,
		AggregateType: params.AggregateType,
		AggregateID:   aggregateID,
		EventType:     params.EventType,
		Payload:       params.Payload,
	})
}

func (s *Store) GetOrderForUpdate(ctx context.Context, id string) (sqlc.Order, error) {
	orderID := pgtype.UUID{}
	if err := orderID.Scan(id); err != nil {
		return sqlc.Order{}, err
	}

	return s.queries.GetOrderForUpdate(ctx, orderID)
}

func (s *Store) UpdateOrderStatus(ctx context.Context, id string, status string) (sqlc.Order, error) {
	orderID := pgtype.UUID{}
	if err := orderID.Scan(id); err != nil {
		return sqlc.Order{}, err
	}

	return s.queries.UpdateOrderStatus(ctx, sqlc.UpdateOrderStatusParams{
		ID:     orderID,
		Status: status,
	})
}

func (s *Store) CreatePayment(ctx context.Context, params CreatePaymentParams) (sqlc.Payment, error) {
	id := pgtype.UUID{}
	if err := id.Scan(params.ID); err != nil {
		return sqlc.Payment{}, err
	}

	orderID := pgtype.UUID{}
	if err := orderID.Scan(params.OrderID); err != nil {
		return sqlc.Payment{}, err
	}

	return s.queries.CreatePayment(ctx, sqlc.CreatePaymentParams{
		ID:            id,
		OrderID:       orderID,
		Status:        params.Status,
		AmountCents:   params.AmountCents,
		Provider:      params.Provider,
		ProviderRef:   pgtype.Text{String: params.ProviderRef, Valid: params.ProviderRef != ""},
		FailureReason: pgtype.Text{String: params.FailureReason, Valid: params.FailureReason != ""},
	})
}

func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txStore := &Store{
		pool:    s.pool,
		queries: s.queries.WithTx(tx),
	}

	if err := fn(txStore); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
