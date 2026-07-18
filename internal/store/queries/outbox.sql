-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (
    id,
    aggregate_type,
    aggregate_id,
    partition_key,
    event_type,
    payload
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: ClaimOutboxEvents :many
SELECT *
FROM outbox_events
WHERE published_at IS NULL
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventPublished :one
UPDATE outbox_events
SET published_at = NOW(),
    last_error = NULL
WHERE id = $1
RETURNING *;

-- name: MarkOutboxEventFailed :one
UPDATE outbox_events
SET attempt = attempt + 1,
    last_error = $2
WHERE id = $1
RETURNING *;