package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hieutrinh02/go-order-service/internal/distributedlock"
	"github.com/hieutrinh02/go-order-service/internal/service"
	"github.com/hieutrinh02/go-order-service/internal/store/sqlc"
)

type createOrderRequest struct {
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Description string `json:"description"`
}

type orderResponse struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Status      string    `json:"status"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type paymentResponse struct {
	ID            string    `json:"id"`
	OrderID       string    `json:"order_id"`
	Status        string    `json:"status"`
	AmountCents   int64     `json:"amount_cents"`
	Provider      string    `json:"provider"`
	ProviderRef   string    `json:"provider_ref,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type payOrderResponse struct {
	Order   orderResponse   `json:"order"`
	Payment paymentResponse `json:"payment"`
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, err := s.orderService.CreateOrder(r.Context(), service.CreateOrderParams{
		UserID:         claims.UserID,
		IdempotencyKey: idempotencyKey,
		Method:         r.Method,
		Path:           r.URL.Path,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		Description:    req.Description,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrIdempotencyKeyRequired):
			writeJSONError(w, http.StatusBadRequest, "idempotency key is required")
		case errors.Is(err, service.ErrInvalidInput):
			writeJSONError(w, http.StatusBadRequest, "invalid order input")
		case errors.Is(err, service.ErrIdempotencyConflict):
			writeJSONError(w, http.StatusConflict, "idempotency key reused with different request")
		default:
			s.logger.Error("failed to create order", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to create order")
		}
		return
	}

	writeJSON(w, result.StatusCode, newOrderResponse(result.Order))
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orders, err := s.orderService.ListOrders(r.Context(), service.ListOrdersParams{
		UserID: claims.UserID,
		Role:   claims.Role,
	})
	if err != nil {
		s.logger.Error("failed to list orders", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	response := make([]orderResponse, 0, len(orders))
	for _, order := range orders {
		response = append(response, newOrderResponse(order))
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeJSONError(w, http.StatusBadRequest, "order id is required")
		return
	}

	order, err := s.orderService.GetOrder(r.Context(), service.GetOrderParams{
		UserID:  claims.UserID,
		Role:    claims.Role,
		OrderID: orderID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			writeJSONError(w, http.StatusNotFound, "order not found")
		case errors.Is(err, service.ErrOrderForbidden):
			writeJSONError(w, http.StatusForbidden, "order forbidden")
		default:
			s.logger.Error("failed to get order", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to get order")
		}
		return
	}

	writeJSON(w, http.StatusOK, newOrderResponse(order))
}

func (s *Server) handlePayOrder(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeJSONError(w, http.StatusBadRequest, "order id is required")
		return
	}

	lock, ok := s.acquireOrderLock(w, r, orderID)
	if !ok {
		return
	}
	defer s.releaseOrderLock(r, lock, orderID)

	result, err := s.orderService.PayOrder(r.Context(), service.PayOrderParams{
		UserID:         claims.UserID,
		Role:           claims.Role,
		OrderID:        orderID,
		IdempotencyKey: idempotencyKey,
		Method:         r.Method,
		Path:           r.URL.Path,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeJSONError(w, http.StatusBadRequest, "invalid payment input")
		case errors.Is(err, service.ErrIdempotencyKeyRequired):
			writeJSONError(w, http.StatusBadRequest, "idempotency key is required")
		case errors.Is(err, service.ErrIdempotencyConflict):
			writeJSONError(w, http.StatusConflict, "idempotency key reused with different request")
		case errors.Is(err, service.ErrOrderNotFound):
			writeJSONError(w, http.StatusNotFound, "order not found")
		case errors.Is(err, service.ErrOrderForbidden):
			writeJSONError(w, http.StatusForbidden, "order forbidden")
		case errors.Is(err, service.ErrOrderInvalidStatus):
			writeJSONError(w, http.StatusConflict, "order cannot be paid in current status")
		default:
			s.logger.Error("failed to pay order", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to pay order")
		}
		return
	}

	writeJSON(w, result.StatusCode, payOrderResponse{
		Order:   newOrderResponse(result.Order),
		Payment: newPaymentResponse(result.Payment),
	})
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeJSONError(w, http.StatusBadRequest, "order id is required")
		return
	}

	lock, ok := s.acquireOrderLock(w, r, orderID)
	if !ok {
		return
	}
	defer s.releaseOrderLock(r, lock, orderID)

	order, err := s.orderService.CancelOrder(r.Context(), service.CancelOrderParams{
		UserID:  claims.UserID,
		Role:    claims.Role,
		OrderID: orderID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeJSONError(w, http.StatusBadRequest, "invalid cancel input")
		case errors.Is(err, service.ErrOrderNotFound):
			writeJSONError(w, http.StatusNotFound, "order not found")
		case errors.Is(err, service.ErrOrderForbidden):
			writeJSONError(w, http.StatusForbidden, "order forbidden")
		case errors.Is(err, service.ErrOrderInvalidStatus):
			writeJSONError(w, http.StatusConflict, "order cannot be cancelled in current status")
		default:
			s.logger.Error("failed to cancel order", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to cancel order")
		}
		return
	}

	writeJSON(w, http.StatusOK, newOrderResponse(order))
}

func (s *Server) acquireOrderLock(w http.ResponseWriter, r *http.Request, orderID string) (*distributedlock.Lock, bool) {
	if s.lockManager == nil {
		return nil, true
	}

	lock, err := s.lockManager.Acquire(r.Context(), orderLockKey(orderID))
	if err != nil {
		if errors.Is(err, distributedlock.ErrLockNotAcquired) {
			writeJSONError(w, http.StatusConflict, "order is being processed")
			return nil, false
		}

		s.logger.Error("failed to acquire order lock", "order_id", orderID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to acquire order lock")
		return nil, false
	}

	return lock, true
}

func (s *Server) releaseOrderLock(r *http.Request, lock *distributedlock.Lock, orderID string) {
	if lock == nil {
		return
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lock.Release(releaseCtx); err != nil {
		s.logger.Error("failed to release order lock", "order_id", orderID, "error", err)
	}
}

func orderLockKey(orderID string) string {
	return fmt.Sprintf("lock:order:%s", orderID)
}

func newOrderResponse(order sqlc.Order) orderResponse {
	return orderResponse{
		ID:          order.ID.String(),
		UserID:      order.UserID.String(),
		Status:      order.Status,
		AmountCents: order.AmountCents,
		Currency:    order.Currency,
		Description: order.Description.String,
		CreatedAt:   order.CreatedAt.Time.UTC(),
		UpdatedAt:   order.UpdatedAt.Time.UTC(),
	}
}

func newPaymentResponse(payment sqlc.Payment) paymentResponse {
	return paymentResponse{
		ID:            payment.ID.String(),
		OrderID:       payment.OrderID.String(),
		Status:        payment.Status,
		AmountCents:   payment.AmountCents,
		Provider:      payment.Provider,
		ProviderRef:   payment.ProviderRef.String,
		FailureReason: payment.FailureReason.String,
		CreatedAt:     payment.CreatedAt.Time.UTC(),
		UpdatedAt:     payment.UpdatedAt.Time.UTC(),
	}
}
