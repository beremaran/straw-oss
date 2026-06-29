-- +goose Up
-- +goose StatementBegin
-- Migration to Bearer token authentication
-- This renames key_hash to token_hash and clears existing keys
-- since bcrypt hashes are incompatible with the new SHA256 lookup scheme

-- Rename key_hash column to token_hash
ALTER TABLE api_keys RENAME COLUMN key_hash TO token_hash;

-- Update column comment
COMMENT
ON COLUMN api_keys.token_hash IS 'SHA256 hash of the Bearer token';

-- Clear existing keys (bcrypt hashes won't work with new SHA256 lookup)
-- This is safe since there is no production deployment
TRUNCATE TABLE api_keys CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore key_hash column name
ALTER TABLE api_keys RENAME COLUMN token_hash TO key_hash;
COMMENT
ON COLUMN api_keys.key_hash IS 'bcrypt hash of the API key';
-- +goose StatementEnd
