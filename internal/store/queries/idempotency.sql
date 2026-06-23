-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (
    id,
    user_id,
    key,
    method,
    path,
    request_hash,
    response_body,
    status_code,
    resource_type,
    resource_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetIdempotencyKey :one
SELECT *
FROM idempotency_keys
WHERE user_id = $1
  AND key = $2;