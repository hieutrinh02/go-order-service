-- +goose Up
ALTER TABLE outbox_events
ADD COLUMN partition_key UUID;

UPDATE outbox_events
SET partition_key = CASE
    WHEN aggregate_type = 'order' THEN aggregate_id
    WHEN aggregate_type = 'payment' THEN (payload->>'order_id')::UUID
END;

ALTER TABLE outbox_events
ALTER COLUMN partition_key SET NOT NULL;

-- +goose Down
ALTER TABLE outbox_events
DROP COLUMN partition_key;