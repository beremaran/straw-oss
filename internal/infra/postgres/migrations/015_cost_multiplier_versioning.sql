-- +goose Up
-- +goose StatementBegin
ALTER TABLE cost_multipliers
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN version INT NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE cost_multipliers
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at;
-- +goose StatementEnd
