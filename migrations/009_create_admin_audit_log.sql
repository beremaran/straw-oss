-- +goose Up
-- +goose StatementBegin
CREATE TABLE admin_audit_log (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    query TEXT,
    body TEXT,
    ip TEXT,
    user_agent TEXT,
    status INT,
    error TEXT
);

CREATE INDEX idx_admin_audit_log_timestamp ON admin_audit_log(timestamp);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS admin_audit_log;
-- +goose StatementEnd
