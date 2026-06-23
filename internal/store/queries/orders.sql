-- name: CreateOrder :one
INSERT INTO orders (
    id,
    user_id,
    status,
    amount_cents,
    currency,
    description
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetOrder :one
SELECT *
FROM orders
WHERE id = $1;

-- name: ListOrdersByUser :many
SELECT *
FROM orders
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListOrders :many
SELECT *
FROM orders
ORDER BY created_at DESC;