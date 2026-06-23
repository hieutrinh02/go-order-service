package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
)

var (
	ErrOrderNotFound  = errors.New("order not found")
	ErrOrderForbidden = errors.New("order forbidden")
)

const (
	OrderStatusPendingPayment = "pending_payment"
)

type OrderService struct {
	store *store.Store
}

type CreateOrderParams struct {
	UserID      string
	AmountCents int64
	Currency    string
	Description string
}

type ListOrdersParams struct {
	UserID string
	Role   string
}

type GetOrderParams struct {
	UserID  string
	Role    string
	OrderID string
}

func NewOrderService(store *store.Store) *OrderService {
	return &OrderService{
		store: store,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, params CreateOrderParams) (sqlc.Order, error) {
	currency := strings.TrimSpace(strings.ToUpper(params.Currency))
	if params.UserID == "" || params.AmountCents <= 0 || currency == "" {
		return sqlc.Order{}, ErrInvalidInput
	}

	return s.store.CreateOrder(ctx, store.CreateOrderParams{
		ID:          uuid.NewString(),
		UserID:      params.UserID,
		Status:      OrderStatusPendingPayment,
		AmountCents: params.AmountCents,
		Currency:    currency,
		Description: strings.TrimSpace(params.Description),
	})
}

func (s *OrderService) ListOrders(ctx context.Context, params ListOrdersParams) ([]sqlc.Order, error) {
	if params.Role == "admin" {
		return s.store.ListOrders(ctx)
	}

	return s.store.ListOrdersByUser(ctx, params.UserID)
}

func (s *OrderService) GetOrder(ctx context.Context, params GetOrderParams) (sqlc.Order, error) {
	order, err := s.store.GetOrder(ctx, params.OrderID)
	if err != nil {
		return sqlc.Order{}, ErrOrderNotFound
	}

	if params.Role != "admin" && order.UserID.String() != params.UserID {
		return sqlc.Order{}, ErrOrderForbidden
	}

	return order, nil
}
