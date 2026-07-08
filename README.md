<h1 align="center">Go Order Service</h1>

<p align="center">
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/License-MIT-green" />
  </a>
  <a href="https://github.com/hieutrinh02/go-order-service/actions/workflows/deploy.yml">
    <img src="https://github.com/hieutrinh02/go-order-service/actions/workflows/deploy.yml/badge.svg" />
  </a>
  <img src="https://img.shields.io/badge/Backend-Go-00ADD8" />
  <img src="https://img.shields.io/badge/Deploy-AWS_EC2-FF9900" />
  <img src="https://img.shields.io/badge/Runtime-Docker_Compose-2496ED" />
  <img src="https://img.shields.io/badge/HTTPS-Let's_Encrypt-003A70" />
  <img src="https://img.shields.io/badge/Observability-Prometheus%20%2B%20Grafana-F46800" />
</p>

A production-inspired order and payment backend written in Go, backed by PostgreSQL, Redis, and NATS.

The project demonstrates authentication, authorization, order lifecycle management, payment simulation, request idempotency, transactional outbox publishing, asynchronous event consumption, graceful shutdown, structured logging, Redis-backed coordination, Prometheus metrics, Grafana dashboards, and AWS EC2 deployment with CI/CD.

## Production Endpoints

- Frontend: `https://go-order-service.hieutrinh02.dev`
- API: `https://api.go-order-service.hieutrinh02.dev`
- Grafana: EC2 port `3000`, restricted by security group source IP

Nginx serves the React frontend from `FE_DIST_DIR` and reverse proxies API traffic from the API subdomain to the Go API replicas.

## Features

- User registration, login, refresh token, logout, and authenticated `/me`
- JWT access tokens and HttpOnly refresh token cookies
- Customer/admin authorization for order access
- Order create, list, get, pay, and cancel endpoints
- Idempotent order creation and payment requests with `Idempotency-Key`
- Payment retry support for failed payments
- PostgreSQL transactions for order, payment, outbox, and idempotency writes
- Transactional outbox table for reliable event creation
- Outbox publisher that publishes events to NATS
- Event consumer that records processed events and notification deliveries
- Idempotent consumer processing with `processed_events`
- Safe outbox claiming with `FOR UPDATE SKIP LOCKED`
- Redis-backed IP rate limiting
- Redis-backed distributed lock for order pay/cancel flows
- Graceful shutdown for API, publisher, consumer, and metrics servers
- Prometheus metrics for HTTP, orders, payments, rate limiting, outbox, and consumer events
- Provisioned Grafana dashboard for production-style service visibility
- Docker Compose setup for PostgreSQL, Redis, NATS, Prometheus, Grafana, Nginx, and Certbot
- Production Compose deployment on AWS EC2 with Nginx HTTPS, Let's Encrypt, GHCR images, and GitHub Actions CI/CD

## Architecture

```text
Client
  |
  v
Nginx HTTPS reverse proxy
  |
  v
API server replicas
  |
  +--> PostgreSQL
        |
        +--> users
        +--> refresh_tokens
        +--> orders
        +--> payments
        +--> idempotency_keys
        +--> outbox_events
  |
  +--> Redis
        |
        +--> rate limit counters
        +--> order locks
  |
  v
Outbox publisher
  |
  v
NATS
  |
  v
Event consumer
  |
  v
PostgreSQL
  |
  +--> processed_events
  +--> notification_deliveries
```

Order creation, payment, cancellation, idempotency records, and outbox events are written inside PostgreSQL transactions. The publisher later claims unpublished outbox rows and publishes them to NATS.

Publisher instances claim outbox events with:

```sql
FOR UPDATE SKIP LOCKED
```

This allows multiple publisher instances to safely share the same outbox table without publishing the same row at the same time.

## Tech Stack

- Go
- chi
- PostgreSQL
- pgx/v5
- sqlc
- goose
- NATS
- Redis
- JWT
- bcrypt
- slog
- Prometheus Go client
- Grafana
- Nginx
- Certbot / Let's Encrypt
- GitHub Actions
- AWS EC2
- Docker Compose

## Project Structure

```text
cmd/api                  HTTP API entrypoint
cmd/publisher            outbox publisher entrypoint
cmd/consumer             NATS event consumer entrypoint
internal/api             HTTP router, handlers, middleware, and response helpers
internal/appstart        startup dependency retry logic
internal/auth            password hashing, JWT, and refresh token helpers
internal/broker          NATS wrapper
internal/cache           Redis client setup
internal/config          environment configuration
internal/consumer        event consumer logic
internal/db              PostgreSQL pool setup
internal/distributedlock Redis-backed lock manager
internal/metrics         Prometheus metrics and metrics server
internal/publisher       outbox publisher logic
internal/ratelimit       Redis-backed fixed-window rate limiter
internal/service         auth, order, payment, and idempotency business logic
internal/store           data access wrapper around sqlc
deploy/grafana           Grafana datasource and dashboard provisioning
deploy/nginx             Nginx reverse proxy configuration
migrations               Goose database migrations
prometheus.yml           Prometheus scrape configuration
```

## Getting Started

### Prerequisites

- Go
- Docker and Docker Compose
- Goose CLI
- sqlc CLI

### Environment

Copy the example environment file:

```bash
cp .env.example .env
```

Default local values:

```env
PORT=8080
DATABASE_URL=postgres://orderservice:orderservice@localhost:5434/order_service?sslmode=disable
NATS_URL=nats://localhost:4223
JWT_SECRET=dev-secret-change-me
COOKIE_SECURE=false
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=168h
PUBLISHER_BATCH_SIZE=10
PUBLISHER_POLL_INTERVAL=2s
PUBLISHER_METRICS_PORT=8081
CONSUMER_METRICS_PORT=8082
```

`JWT_SECRET=dev-secret-change-me` is only for local development. Use a strong secret in production-like deployments.

### Start Infrastructure

```bash
docker compose up -d
```

This starts:

```text
PostgreSQL  localhost:5434
NATS        localhost:4223
NATS monitor localhost:8223
Prometheus  localhost:9091
```

### Run Migrations

```bash
goose -dir migrations postgres "postgres://orderservice:orderservice@localhost:5434/order_service?sslmode=disable" up
```

Check migration status:

```bash
goose -dir migrations postgres "postgres://orderservice:orderservice@localhost:5434/order_service?sslmode=disable" status
```

### Run the Processes

Run the API:

```bash
go run ./cmd/api
```

Run the outbox publisher:

```bash
go run ./cmd/publisher
```

Run the event consumer:

```bash
go run ./cmd/consumer
```

The API listens on:

```text
http://localhost:8080
```

Publisher metrics listen on:

```text
http://localhost:8081/metrics
```

Consumer metrics listen on:

```text
http://localhost:8082/metrics
```

## API

### Health Check

```bash
curl http://localhost:8080/healthz
```

### Readiness Check

```bash
curl http://localhost:8080/readyz
```

### Register

```bash
curl -i -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "flowtest@example.com",
    "password": "secret123",
    "role": "customer"
  }'
```

For demo purposes, registration accepts a `role` field so customer/admin authorization flows are easy to test. In production, public signup should create customer users only, and admin accounts should be provisioned separately.

### Login

```bash
curl -i -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "flowtest@example.com",
    "password": "secret123"
  }'
```

Example response:

```json
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "user": {
    "id": "e395ed6b-3414-4727-9514-fd634fba59eb",
    "email": "flowtest@example.com",
    "role": "customer",
    "created_at": "2026-06-23T04:49:14.389566Z",
    "updated_at": "2026-06-23T04:49:14.389566Z"
  }
}
```

The login endpoint returns an access token in the JSON response and sets a refresh token in an HttpOnly cookie.

### Authenticated User

```bash
curl http://localhost:8080/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

### Refresh Access Token

```bash
curl -i -X POST http://localhost:8080/auth/refresh \
  --cookie "refresh_token=<refresh_token>"
```

### Logout

```bash
curl -i -X POST http://localhost:8080/auth/logout \
  --cookie "refresh_token=<refresh_token>"
```

### Create Order

```bash
curl -i -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Idempotency-Key: order-create-001" \
  -d '{
    "amount_cents": 1999,
    "currency": "usd",
    "description": "test order"
  }'
```

Example response:

```json
{
  "id": "f4533cc1-d0ff-4de3-bbcd-899583bf7462",
  "user_id": "e395ed6b-3414-4727-9514-fd634fba59eb",
  "status": "pending_payment",
  "amount_cents": 1999,
  "currency": "USD",
  "description": "test order",
  "created_at": "2026-06-23T08:52:08.353282Z",
  "updated_at": "2026-06-23T08:52:08.353282Z"
}
```

### List Orders

```bash
curl http://localhost:8080/orders \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Customers see their own orders. Admins can list all orders.

### Get Order

```bash
curl http://localhost:8080/orders/<order_id> \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

### Pay Order

```bash
curl -i -X POST http://localhost:8080/orders/<order_id>/pay \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Idempotency-Key: order-pay-001"
```

The payment provider is mocked. Payment attempts can succeed or fail. Failed payments can be retried with a new idempotency key while the order is still in `payment_failed`.

### Cancel Order

```bash
curl -i -X POST http://localhost:8080/orders/<order_id>/cancel \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Only orders in `pending_payment` or `payment_failed` can be cancelled.

## Order Statuses

```text
pending_payment  created and waiting for payment
paid             payment succeeded
payment_failed   payment failed and can be retried
cancelled        cancelled before successful payment
```

## Payment Statuses

```text
succeeded  payment succeeded
failed     payment failed
```

## Idempotency

Clients must provide an `Idempotency-Key` header when creating or paying an order.

The API stores:

```text
user_id
key
method
path
request_hash
response_body
status_code
resource_type
resource_id
```

If the same user reuses the same key for the same method, path, and request hash, the API returns the original resource instead of creating a duplicate.

```text
First create request      -> 201 Created
Repeated create request   -> 201 Created with existing order
Same key, different body  -> 409 Conflict
```

The unique constraint on `(user_id, key)` prevents duplicate idempotency records for the same user.

## Outbox and Messaging

Order and payment operations write events to `outbox_events` inside the same transaction as the domain change.

Published event types:

```text
order.created
order.cancelled
payment.succeeded
payment.failed
```

The publisher:

1. Claims unpublished events with `FOR UPDATE SKIP LOCKED`.
2. Publishes an event envelope to NATS.
3. Marks the row as published with `published_at`.
4. Records `attempt` and `last_error` if publishing fails.

The consumer:

1. Subscribes to NATS events.
2. Inserts `(event_id, consumer_name)` into `processed_events`.
3. Skips duplicates with `ON CONFLICT DO NOTHING`.
4. Creates a `notification_deliveries` row.

The current implementation uses core NATS pub/sub. The outbox guarantees events are not lost before publishing. Consumer-side durable redelivery can be upgraded with NATS JetStream.

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

```text
API        stops accepting new HTTP requests and waits for in-flight requests
Publisher  stops polling, finishes the current loop, drains NATS, and closes resources
Consumer   stops waiting for new messages, drains NATS, and closes resources
Metrics    shuts down each metrics HTTP server
```

## Metrics

Metrics are exposed at:

```text
API        http://localhost:8080/metrics
Publisher  http://localhost:8081/metrics
Consumer   http://localhost:8082/metrics
```

Prometheus is available at:

```text
http://localhost:9091
```

Check scrape targets:

```text
http://localhost:9091/targets
```

Custom metrics:

```text
order_service_http_requests_total
order_service_http_request_duration_seconds_bucket
order_service_http_request_duration_seconds_sum
order_service_http_request_duration_seconds_count
order_service_orders_created_total
order_service_payments_total
order_service_rate_limit_allowed_total
order_service_rate_limit_blocked_total
order_service_outbox_events_published_total
order_service_outbox_events_failed_total
order_service_consumer_events_processed_total
order_service_consumer_events_duplicate_total
```

## Useful Commands

### Deploy to AWS EC2

See [docs/aws-ec2-deploy.md](docs/aws-ec2-deploy.md) for the single-instance Docker Compose deployment guide, including GHCR-based CI/CD, Nginx HTTPS and Let's Encrypt.

### Local Commands

Run tests:

```bash
go test ./...
```

Regenerate sqlc code:

```bash
sqlc generate
```

Open psql:

```bash
docker compose exec postgres psql -U orderservice -d order_service
```

Stop Docker services while keeping data:

```bash
docker compose down
```

Stop Docker services and remove volumes:

```bash
docker compose down -v
```

## Resume Bullet

Built a production-inspired order and payment backend in Go with JWT authentication, refresh tokens, role-based authorization, idempotent order/payment APIs, PostgreSQL transactions, Redis-backed rate limiting and distributed locks, transactional outbox, NATS-based asynchronous messaging, graceful shutdown, Prometheus metrics, Grafana dashboards, Nginx HTTPS with Let's Encrypt, and GitHub Actions CI/CD deployment to AWS EC2 using GHCR images.

## Disclaimer

This code is for educational purposes only, has not been audited, and is provided without any warranties or guarantees.

## License

This project is licensed under the MIT License.
