-- name: CreatePayment :one
INSERT INTO payments (
    id,
    order_id,
    status,
    amount_cents,
    provider,
    provider_ref,
    failure_reason
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetPayment :one
SELECT *
FROM payments
WHERE id = $1;