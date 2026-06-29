-- +goose Up
-- +goose StatementBegin
CREATE TABLE fingerprint_presets
(
    id         TEXT PRIMARY KEY,
    name       TEXT  NOT NULL,
    config     JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

COMMENT
ON TABLE fingerprint_presets IS 'Pre-configured TLS fingerprints';
COMMENT
ON COLUMN fingerprint_presets.id IS 'Unique fingerprint ID, e.g. "chrome-130"';
COMMENT
ON COLUMN fingerprint_presets.config IS 'Fingerprint configuration (JA3, HTTP/2 settings, etc.)';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS fingerprint_presets;
-- +goose StatementEnd
