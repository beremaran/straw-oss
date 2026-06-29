-- +goose Up
-- +goose StatementBegin
CREATE TABLE saved_reports
(
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT,
    type        TEXT        NOT NULL,
    filters     JSONB       NOT NULL DEFAULT '{}',
    format      TEXT        NOT NULL DEFAULT 'csv',
    created_by  UUID REFERENCES admin_users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE report_schedules
(
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id              UUID        NOT NULL REFERENCES saved_reports (id) ON DELETE CASCADE,
    cron                   TEXT        NOT NULL,
    timezone               TEXT        NOT NULL DEFAULT 'UTC',
    destination_channel_id UUID,
    is_active              BOOLEAN     NOT NULL DEFAULT true,
    next_run_at            TIMESTAMPTZ,
    last_run_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE report_runs
(
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id    UUID        NOT NULL REFERENCES saved_reports (id) ON DELETE CASCADE,
    schedule_id  UUID REFERENCES report_schedules (id) ON DELETE SET NULL,
    status       TEXT        NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    artifact_url TEXT,
    error        TEXT
);

CREATE INDEX idx_report_runs_report_id ON report_runs (report_id, started_at DESC);
CREATE INDEX idx_report_schedules_report_id ON report_schedules (report_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_report_schedules_report_id;
DROP INDEX IF EXISTS idx_report_runs_report_id;
DROP TABLE IF EXISTS report_runs;
DROP TABLE IF EXISTS report_schedules;
DROP TABLE IF EXISTS saved_reports;
-- +goose StatementEnd
