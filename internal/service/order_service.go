package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hieutrinh02/go-order-service/internal/metrics"
	"github.com/hieutrinh02/go-order-service/internal/store"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
)

var (
	ErrIdempotencyKeyRequired = errors.New("idempotency key required")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrOrderForbidden         = errors.New("order forbidden")
	ErrOrderInvalidStatus     = errors.New("order invalid status")
	ErrOrderNotFound          = errors.New("order not found")
)

const (
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusPaymentFailed  = "payment_failed"
	OrderStatusCancelled      = "cancelled"

	PaymentStatusSucceeded = "succeeded"
	PaymentStatusFailed    = "failed"

	PaymentProviderMock = "mock"

	AggregateTypeOrder   = "order"
	AggregateTypePayment = "payment"

	EventTypeOrderCreated     = "order.created"
	EventTypeOrderCancelled   = "order.cancelled"
	EventTypePaymentSucceeded = "payment.succeeded"
	EventTypePaymentFailed    = "payment.failed"
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

type PayOrderParams struct {
	UserID         string
	Role           string
	OrderID        string
	IdempotencyKey string
	Method         string
	Path           string
}

type PayOrderResult struct {
	Order      sqlc.Order
	Payment    sqlc.Payment
	StatusCode int
	Cached     bool
}

type CancelOrderParams struct {
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

type paymentEventPayload struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	Status        string `json:"status"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	Provider      string `json:"provider"`
	ProviderRef   string `json:"provider_ref,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type orderCancelledEventPayload struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Status  string `json:"status"`
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
		if existing.Method != params.Method || existing.Path != params.Path || existing.RequestHash != requestHash {
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

	var createdOrder sqlc.Order

	if err := s.store.WithTx(ctx, func(txStore *store.Store) error {
		orderID := uuid.NewString()

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

		eventPayload, err := json.Marshal(orderCreatedEventPayload{
			OrderID:     orderID,
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

	metrics.OrdersCreatedTotal.Inc()

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

func (s *OrderService) PayOrder(ctx context.Context, params PayOrderParams) (PayOrderResult, error) {
	if params.UserID == "" || params.OrderID == "" {
		return PayOrderResult{}, ErrInvalidInput
	}

	idempotencyKey := strings.TrimSpace(params.IdempotencyKey)
	if idempotencyKey == "" {
		return PayOrderResult{}, ErrIdempotencyKeyRequired
	}

	requestHash, err := HashRequestBody(struct {
		OrderID string `json:"order_id"`
	}{
		OrderID: params.OrderID,
	})
	if err != nil {
		return PayOrderResult{}, err
	}

	existing, err := s.store.GetIdempotencyKey(ctx, params.UserID, idempotencyKey)
	if err == nil {
		if existing.Method != params.Method || existing.Path != params.Path || existing.RequestHash != requestHash {
			return PayOrderResult{}, ErrIdempotencyConflict
		}

		payment, err := s.store.GetPayment(ctx, existing.ResourceID.String())
		if err != nil {
			return PayOrderResult{}, err
		}

		order, err := s.store.GetOrder(ctx, payment.OrderID.String())
		if err != nil {
			return PayOrderResult{}, err
		}

		statusCode := int(existing.StatusCode.Int32)
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		return PayOrderResult{
			Order:      order,
			Payment:    payment,
			StatusCode: statusCode,
			Cached:     true,
		}, nil
	}

	var result PayOrderResult

	if err := s.store.WithTx(ctx, func(txStore *store.Store) error {
		order, err := txStore.GetOrderForUpdate(ctx, params.OrderID)
		if err != nil {
			return ErrOrderNotFound
		}

		if params.Role != "admin" && order.UserID.String() != params.UserID {
			return ErrOrderForbidden
		}

		if order.Status != OrderStatusPendingPayment && order.Status != OrderStatusPaymentFailed {
			return ErrOrderInvalidStatus
		}

		paymentID := uuid.NewString()
		paymentStatus, providerRef, failureReason := mockPayment(paymentID)

		nextOrderStatus := OrderStatusPaid
		eventType := EventTypePaymentSucceeded
		if paymentStatus == PaymentStatusFailed {
			nextOrderStatus = OrderStatusPaymentFailed
			eventType = EventTypePaymentFailed
		}

		payment, err := txStore.CreatePayment(ctx, store.CreatePaymentParams{
			ID:            paymentID,
			OrderID:       order.ID.String(),
			Status:        paymentStatus,
			AmountCents:   order.AmountCents,
			Provider:      PaymentProviderMock,
			ProviderRef:   providerRef,
			FailureReason: failureReason,
		})
		if err != nil {
			return err
		}

		updatedOrder, err := txStore.UpdateOrderStatus(ctx, order.ID.String(), nextOrderStatus)
		if err != nil {
			return err
		}

		eventPayload, err := json.Marshal(paymentEventPayload{
			PaymentID:     payment.ID.String(),
			OrderID:       order.ID.String(),
			UserID:        order.UserID.String(),
			Status:        payment.Status,
			AmountCents:   payment.AmountCents,
			Currency:      order.Currency,
			Provider:      payment.Provider,
			ProviderRef:   providerRef,
			FailureReason: failureReason,
		})
		if err != nil {
			return err
		}

		if _, err := txStore.CreateOutboxEvent(ctx, store.CreateOutboxEventParams{
			ID:            uuid.NewString(),
			AggregateType: AggregateTypePayment,
			AggregateID:   payment.ID.String(),
			EventType:     eventType,
			Payload:       eventPayload,
		}); err != nil {
			return err
		}

		responseBody, err := json.Marshal(payment)
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
			StatusCode:   http.StatusOK,
			ResourceType: "payment",
			ResourceID:   payment.ID.String(),
		}); err != nil {
			return err
		}

		result = PayOrderResult{
			Order:      updatedOrder,
			Payment:    payment,
			StatusCode: http.StatusOK,
			Cached:     false,
		}

		return nil
	}); err != nil {
		return PayOrderResult{}, err
	}

	metrics.PaymentsTotal.WithLabelValues(result.Payment.Status).Inc()

	return result, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, params CancelOrderParams) (sqlc.Order, error) {
	if params.UserID == "" || params.OrderID == "" {
		return sqlc.Order{}, ErrInvalidInput
	}

	var cancelledOrder sqlc.Order

	if err := s.store.WithTx(ctx, func(txStore *store.Store) error {
		order, err := txStore.GetOrderForUpdate(ctx, params.OrderID)
		if err != nil {
			return ErrOrderNotFound
		}

		if params.Role != "admin" && order.UserID.String() != params.UserID {
			return ErrOrderForbidden
		}

		if order.Status != OrderStatusPendingPayment && order.Status != OrderStatusPaymentFailed {
			return ErrOrderInvalidStatus
		}

		updatedOrder, err := txStore.UpdateOrderStatus(ctx, order.ID.String(), OrderStatusCancelled)
		if err != nil {
			return err
		}

		eventPayload, err := json.Marshal(orderCancelledEventPayload{
			OrderID: updatedOrder.ID.String(),
			UserID:  updatedOrder.UserID.String(),
			Status:  updatedOrder.Status,
		})
		if err != nil {
			return err
		}

		if _, err := txStore.CreateOutboxEvent(ctx, store.CreateOutboxEventParams{
			ID:            uuid.NewString(),
			AggregateType: AggregateTypeOrder,
			AggregateID:   updatedOrder.ID.String(),
			EventType:     EventTypeOrderCancelled,
			Payload:       eventPayload,
		}); err != nil {
			return err
		}

		cancelledOrder = updatedOrder
		return nil
	}); err != nil {
		return sqlc.Order{}, err
	}

	return cancelledOrder, nil
}

func mockPayment(paymentID string) (status string, providerRef string, failureReason string) {
	// Deterministic-ish mock: most payment attempts succeed, some fail.
	if strings.HasSuffix(paymentID, "0") || strings.HasSuffix(paymentID, "f") {
		return PaymentStatusFailed, "", "mock payment declined"
	}

	return PaymentStatusSucceeded, "mock_" + paymentID, ""
}
