-- +goose Up
-- +goose StatementBegin
CREATE TABLE api_keys
(
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash            TEXT NOT NULL UNIQUE,          -- bcrypt hash
    name                TEXT NOT NULL,
    scopes              JSONB            DEFAULT '[]', -- allowed tag patterns, e.g. ["target:*", "type:search"]
    rate_limit_override INT,                           -- optional per-key rate limit
    is_active           BOOLEAN          DEFAULT true,
    created_at          TIMESTAMPTZ      DEFAULT now(),
    expires_at          TIMESTAMPTZ
);

COMMENT
ON TABLE api_keys IS 'API keys for client authentication';
COMMENT
ON COLUMN api_keys.key_hash IS 'bcrypt hash of the API key';
COMMENT
ON COLUMN api_keys.scopes IS 'Allowed tag patterns for this key, e.g. ["target:*", "type:search"]';
COMMENT
ON COLUMN api_keys.rate_limit_override IS 'Optional per-key rate limit override';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS api_keys;
-- +goose StatementEnd
