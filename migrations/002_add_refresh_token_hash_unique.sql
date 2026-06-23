-- +goose Up
ALTER TABLE refresh_tokens
ADD CONSTRAINT refresh_tokens_token_hash_unique UNIQUE (token_hash);

-- +goose Down
ALTER TABLE refresh_tokens
DROP CONSTRAINT IF EXISTS refresh_tokens_token_hash_unique;