-- name: CreateNotificationDelivery :one
INSERT INTO notification_deliveries (
    id,
    event_id,
    channel,
    recipient,
    status
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;