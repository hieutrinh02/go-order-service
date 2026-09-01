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
  <img src="https://img.shields.io/badge/Kubernetes-K3s-326CE5" />
  <img src="https://img.shields.io/badge/Local-Docker_Compose-2496ED" />
  <img src="https://img.shields.io/badge/HTTPS-Let's_Encrypt-003A70" />
  <img src="https://img.shields.io/badge/Observability-Prometheus%20%2B%20Grafana-F46800" />
</p>

A production-inspired order and payment backend written in Go, backed by PostgreSQL, Redis, and Apache Kafka.

The project demonstrates authentication, authorization, order lifecycle management, payment simulation, request idempotency, transactional outbox publishing, asynchronous event consumption, graceful shutdown, structured logging, Redis-backed coordination, Prometheus metrics, Grafana dashboards, and a production-inspired single-node K3s deployment on AWS EC2.

## Production Endpoints

- Frontend: `https://go-order-service.hieutrinh02.dev`
- API: `https://api.go-order-service.hieutrinh02.dev`
- Grafana: internal Kubernetes Service, accessed through SSH and `kubectl port-forward`

K3s Traefik terminates HTTPS and routes the frontend domain to an Nginx Pod and the API subdomain to the Go API Service. The frontend Pod serves the React build from `/home/ubuntu/go-order-service-fe/dist` through a read-only host mount.

## Features

- User registration, login, refresh token, logout, and authenticated `/me`
- JWT access tokens and HttpOnly refresh token cookies
- Customer/admin authorization for order access
- Order create, list, get, pay, and cancel endpoints
- Idempotent order creation and payment requests with `Idempotency-Key`
- Payment retry support for failed payments
- PostgreSQL transactions for order, payment, outbox, and idempotency writes
- Transactional outbox table for reliable event creation
- Outbox publisher that publishes keyed events to Kafka
- Event consumer that records processed events and notification deliveries
- Idempotent consumer processing with `processed_events`
- Idempotent Kafka producer with `acks=all` and delivery-report handling
- Manual Kafka consumer offset commits after successful event handling
- Safe outbox claiming with `FOR UPDATE SKIP LOCKED`
- Redis-backed IP rate limiting
- Redis-backed distributed lock for order pay/cancel flows
- Graceful shutdown for API, publisher, consumer, and metrics servers
- Prometheus metrics for HTTP, orders, payments, rate limiting, outbox, and consumer events
- Provisioned Grafana dashboard for production-style service visibility
- Docker Compose setup for PostgreSQL, Redis, Kafka, Prometheus, Grafana, Nginx, and Certbot
- Kubernetes manifests organized with Kustomize bases and EC2-specific overlays
- Stateful workloads with persistent volumes, health probes, resource controls, Services, Jobs, and Ingresses
- Staged K3s bootstrap that waits for infrastructure, Kafka topic initialization, and database migration before starting application workloads
- Single-node AWS EC2 deployment with Traefik HTTPS, immutable GHCR images, Prometheus, and Grafana
- GitHub Actions CI/CD for validation, image publishing, and EC2 delivery

## Architecture

```text
Client
  |
  v
K3s Traefik ingress
  |-- frontend domain -> frontend Service -> Nginx Pod
  |
  `-- API domain -> API Service -> API server replicas
                                    |-- Redis
                                    |     |-- rate limit counters
                                    |     `-- order locks
                                    |
                                    `-- PostgreSQL
                                          |-- users and refresh tokens
                                          |-- orders and payments
                                          |-- idempotency keys
                                          `-- outbox events
                                                |
                                                v
                                          Outbox publisher
                                                |
                                                | topic: order.events.v1
                                                | key: order ID
                                                v
                                          Apache Kafka
                                                |
                                                | consumer group:
                                                | notification-consumer-v1
                                                v
                                          Event consumer
                                                |
                                                v
                                          PostgreSQL
                                                |-- processed events
                                                `-- notification deliveries
```

Order creation, payment, cancellation, idempotency records, and outbox events are written inside PostgreSQL transactions. The publisher later claims unpublished outbox rows and publishes them to Kafka. Events for the same order use the order ID as their Kafka key, so they are routed to the same partition and retain per-order ordering.

Publisher instances claim outbox events with:

```sql
FOR UPDATE SKIP LOCKED
```

This allows multiple publisher instances to safely share the same outbox table without publishing the same row at the same time.

## Network and Ports

| Component | External access | Kubernetes endpoint | Used by |
| --- | --- | --- | --- |
| Traefik | EC2 `:80` / `:443` | - | Internet traffic |
| Frontend | `https://go-order-service.hieutrinh02.dev` | `frontend:8080` | Traefik |
| API | `https://api.go-order-service.hieutrinh02.dev` | `api:8080` | Traefik, Prometheus |
| Publisher | None | `publisher:8081` metrics | Prometheus |
| Consumer | None | `consumer:8082` metrics | Prometheus |
| PostgreSQL | None | `postgres:5432` | API, publisher, consumer, migration Job |
| Redis | None | `redis:6379` | API |
| Kafka | None | `kafka:29092` | Publisher, consumer, topic-init Job |
| Prometheus | Port-forward only | `prometheus:9090` | Grafana, operators |
| Grafana | Port-forward only | `grafana:3000` | Operators |

Kafka also exposes `kafka:9093` inside the cluster for its KRaft controller quorum; application clients do not use that port.

## Tech Stack

- Go
- chi
- PostgreSQL
- pgx/v5
- sqlc
- goose
- Apache Kafka
- confluent-kafka-go
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
- Kubernetes
- Kustomize
- K3s
- Traefik
- Docker Compose

## Project Structure

```text
cmd/api                  HTTP API entrypoint
cmd/publisher            outbox publisher entrypoint
cmd/consumer             Kafka event consumer entrypoint
internal/api             HTTP router, handlers, middleware, and response helpers
internal/appstart        startup dependency retry logic
internal/auth            password hashing, JWT, and refresh token helpers
internal/cache           Redis client setup
internal/config          environment configuration
internal/consumer        event consumer logic
internal/db              PostgreSQL pool setup
internal/distributedlock Redis-backed lock manager
internal/kafka           Kafka producer and consumer clients
internal/metrics         Prometheus metrics and metrics server
internal/publisher       outbox publisher logic
internal/ratelimit       Redis-backed fixed-window rate limiter
internal/service         auth, order, payment, and idempotency business logic
internal/store           data access wrapper around sqlc
deploy/grafana           Grafana datasource and dashboard provisioning
deploy/k8s               Kubernetes bases and EC2 deployment overlays
deploy/nginx             Nginx reverse proxy configuration
migrations               Goose database migrations
prometheus.yml           Prometheus scrape configuration
scripts/deploy-k8s.sh     staged K3s deployment script
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
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
KAFKA_TOPIC=order.events.v1
KAFKA_CONSUMER_GROUP=notification-consumer-v1
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
Redis       localhost:6379
Kafka       localhost:9092
Prometheus  localhost:9091
```

The one-shot `kafka-init` service creates `order.events.v1` with three partitions and replication factor one after Kafka becomes healthy. An `Exited (0)` status for `kafka-init` is expected.

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
2. Publishes an event envelope to `order.events.v1`, keyed by the order ID.
3. Waits for the Kafka delivery report and marks the row as published with `published_at` only after successful delivery.
4. Records `attempt` and `last_error` if publishing fails.

The consumer:

1. Subscribes to `order.events.v1` as part of `notification-consumer-v1`.
2. Inserts `(event_id, consumer_name)` into `processed_events`.
3. Skips duplicates with `ON CONFLICT DO NOTHING`.
4. Creates a `notification_deliveries` row.
5. Commits the Kafka offset only after the handler succeeds.

The producer uses `acks=all` and idempotent production. The consumer disables automatic offset commits and provides at-least-once processing: a record can be delivered again if the database transaction succeeds but the offset commit does not. The `processed_events` table makes this redelivery safe. Invalid records are logged and committed so they do not block a partition; a dead-letter topic can be added in a later hardening phase.

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

```text
API        stops accepting new HTTP requests and waits for in-flight requests
Publisher  stops polling, flushes queued Kafka deliveries up to its close timeout, and closes resources
Consumer   stops polling, closes the Kafka consumer, and leaves its consumer group
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

### Deploy to AWS EC2 with K3s

See [docs/aws-ec2-k3s-deploy.md](docs/aws-ec2-k3s-deploy.md) for the single-node K3s deployment guide, including the staged bootstrap, Kubernetes Secrets, Traefik HTTPS, verification, operations, and rollback.

The deployment uses Kustomize manifests from `deploy/k8s`, while `scripts/deploy-k8s.sh` applies them in dependency-safe stages with an immutable GHCR image tag.

### Legacy Docker Compose Deployment

See [docs/aws-ec2-deploy.md](docs/aws-ec2-deploy.md) for the previous single-instance Docker Compose deployment. Its volumes are retained during the K3s cutover for rollback or later data migration.

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

Built a production-inspired order and payment backend in Go with JWT authentication, refresh tokens, role-based authorization, idempotent order/payment APIs, PostgreSQL transactions, Redis-backed rate limiting and distributed locks, a transactional outbox, keyed Kafka event publishing, manual consumer offset commits, idempotent event processing, graceful shutdown, Prometheus metrics, Grafana dashboards, and a Kustomize-managed single-node K3s deployment on AWS EC2 with Traefik HTTPS and immutable GHCR images.

## Disclaimer

This code is for educational purposes only, has not been audited, and is provided without any warranties or guarantees.

## License

This project is licensed under the MIT License.
