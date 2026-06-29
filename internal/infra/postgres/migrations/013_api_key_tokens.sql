-- +goose Up
-- +goose StatementBegin
CREATE TABLE api_key_tokens
(
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id UUID NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL    DEFAULT 'active', -- active, grace, revoked
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ      DEFAULT now()
);

CREATE INDEX idx_api_key_tokens_api_key_id ON api_key_tokens (api_key_id);
CREATE INDEX idx_api_key_tokens_status ON api_key_tokens (status);

COMMENT
ON TABLE api_key_tokens IS 'History of tokens for each API key';

-- Copy existing tokens
INSERT INTO api_key_tokens (api_key_id, token_hash, status, created_at)
SELECT id, token_hash, 'active', created_at
FROM api_keys
WHERE token_hash IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS api_key_tokens;
-- +goose StatementEnd
