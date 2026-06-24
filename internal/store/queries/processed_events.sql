-- name: TryCreateProcessedEvent :one
INSERT INTO processed_events (
    event_id,
    consumer_name
) VALUES (
    $1, $2
)
ON CONFLICT DO NOTHING
RETURNING *;