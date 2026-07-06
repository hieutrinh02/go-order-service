-- +goose Up
ALTER TABLE payments
DROP CONSTRAINT payments_status_check;

ALTER TABLE payments
ADD CONSTRAINT payments_status_check CHECK (
    status IN ('succeeded', 'failed')
);

-- +goose Down
ALTER TABLE payments
DROP CONSTRAINT payments_status_check;

ALTER TABLE payments
ADD CONSTRAINT payments_status_check CHECK (
    status IN ('processing', 'succeeded', 'failed')
);