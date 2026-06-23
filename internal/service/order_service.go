package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
)

var (
	ErrIdempotencyKeyRequired = errors.New("idempotency key required")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderForbidden         = errors.New("order forbidden")
)

const (
	OrderStatusPendingPayment = "pending_payment"

	AggregateTypeOrder    = "order"
	EventTypeOrderCreated = "order.created"
)

type OrderService struct {
	store *store.Store
}

type CreateOrderParams struct {
	UserID         string
	IdempotencyKey string
	Method         string
	Path           string
	AmountCents    int64
	Currency       string
	Description    string
}

type CreateOrderResult struct {
	Order      sqlc.Order
	StatusCode int
	Cached     bool
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

type orderCreatedEventPayload struct {
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Description string `json:"description"`
}

func NewOrderService(store *store.Store) *OrderService {
	return &OrderService{
		store: store,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, params CreateOrderParams) (CreateOrderResult, error) {
	idempotencyKey := strings.TrimSpace(params.IdempotencyKey)
	if idempotencyKey == "" {
		return CreateOrderResult{}, ErrIdempotencyKeyRequired
	}

	currency := strings.TrimSpace(strings.ToUpper(params.Currency))
	description := strings.TrimSpace(params.Description)
	if params.UserID == "" || params.AmountCents <= 0 || currency == "" {
		return CreateOrderResult{}, ErrInvalidInput
	}

	requestHash, err := HashRequestBody(struct {
		AmountCents int64  `json:"amount_cents"`
		Currency    string `json:"currency"`
		Description string `json:"description"`
	}{
		AmountCents: params.AmountCents,
		Currency:    currency,
		Description: description,
	})
	if err != nil {
		return CreateOrderResult{}, err
	}

	existing, err := s.store.GetIdempotencyKey(ctx, params.UserID, idempotencyKey)
	if err == nil {
		if existing.RequestHash != requestHash {
			return CreateOrderResult{}, ErrIdempotencyConflict
		}

		order, err := s.store.GetOrder(ctx, existing.ResourceID.String())
		if err != nil {
			return CreateOrderResult{}, err
		}

		statusCode := int(existing.StatusCode.Int32)
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		return CreateOrderResult{
			Order:      order,
			StatusCode: statusCode,
			Cached:     true,
		}, nil
	}

	orderID := uuid.NewString()

	eventPayload, err := json.Marshal(orderCreatedEventPayload{
		OrderID:     orderID,
		UserID:      params.UserID,
		Status:      OrderStatusPendingPayment,
		AmountCents: params.AmountCents,
		Currency:    currency,
		Description: description,
	})
	if err != nil {
		return CreateOrderResult{}, err
	}

	var createdOrder sqlc.Order

	if err := s.store.WithTx(ctx, func(txStore *store.Store) error {
		order, err := txStore.CreateOrder(ctx, store.CreateOrderParams{
			ID:          orderID,
			UserID:      params.UserID,
			Status:      OrderStatusPendingPayment,
			AmountCents: params.AmountCents,
			Currency:    currency,
			Description: description,
		})
		if err != nil {
			return err
		}

		if _, err := txStore.CreateOutboxEvent(ctx, store.CreateOutboxEventParams{
			ID:            uuid.NewString(),
			AggregateType: AggregateTypeOrder,
			AggregateID:   order.ID.String(),
			EventType:     EventTypeOrderCreated,
			Payload:       eventPayload,
		}); err != nil {
			return err
		}

		responseBody, err := json.Marshal(order)
		if err != nil {
			return err
		}

		if _, err := txStore.CreateIdempotencyKey(ctx, store.CreateIdempotencyKeyParams{
			ID:           uuid.NewString(),
			UserID:       params.UserID,
			Key:          idempotencyKey,
			Method:       params.Method,
			Path:         params.Path,
			RequestHash:  requestHash,
			ResponseBody: responseBody,
			StatusCode:   http.StatusCreated,
			ResourceType: "order",
			ResourceID:   order.ID.String(),
		}); err != nil {
			return err
		}

		createdOrder = order
		return nil
	}); err != nil {
		return CreateOrderResult{}, err
	}

	return CreateOrderResult{
		Order:      createdOrder,
		StatusCode: http.StatusCreated,
		Cached:     false,
	}, nil
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
