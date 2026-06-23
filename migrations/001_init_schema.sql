-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT users_role_check CHECK (
        role IN ('customer', 'admin')
    )
);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT orders_status_check CHECK (
        status IN ('pending_payment', 'paid', 'payment_failed', 'cancelled')
    ),
    CONSTRAINT orders_amount_cents_check CHECK (amount_cents > 0)
);

CREATE TABLE payments (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    provider TEXT NOT NULL,
    provider_ref TEXT,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT payments_status_check CHECK (
        status IN ('processing', 'succeeded', 'failed')
    ),
    CONSTRAINT payments_amount_cents_check CHECK (amount_cents > 0)
);

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_body JSONB,
    status_code INT,
    resource_type TEXT,
    resource_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT idempotency_keys_user_key_unique UNIQUE (user_id, key)
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    attempt INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT outbox_events_attempt_check CHECK (attempt >= 0),
    CONSTRAINT outbox_events_event_aggregate_check CHECK (
        (aggregate_type = 'order' AND event_type IN ('order.created', 'order.cancelled'))
        OR
        (aggregate_type = 'payment' AND event_type IN ('payment.succeeded', 'payment.failed'))
    )
);

CREATE TABLE processed_events (
    event_id UUID NOT NULL,
    consumer_name TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT processed_events_pkey PRIMARY KEY (event_id, consumer_name)
);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL,
    channel TEXT NOT NULL,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT notification_deliveries_status_check CHECK (
        status IN ('pending', 'sent', 'failed')
    ),
    CONSTRAINT notification_deliveries_attempt_check CHECK (attempt >= 0)
);

CREATE INDEX idx_refresh_tokens_user_id
ON refresh_tokens (user_id);

CREATE INDEX idx_orders_user_id
ON orders (user_id);

CREATE INDEX idx_orders_status
ON orders (status);

CREATE INDEX idx_payments_order_id
ON payments (order_id);

CREATE INDEX idx_outbox_events_unpublished
ON outbox_events (created_at)
WHERE published_at IS NULL;

CREATE INDEX idx_notification_deliveries_event_id
ON notification_deliveries (event_id);

-- +goose Down
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;