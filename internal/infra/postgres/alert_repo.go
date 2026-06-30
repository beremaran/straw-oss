package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// AlertRuleRepository persists alert rules.
type AlertRuleRepository struct {
	client *Client
}

// NewAlertRuleRepository creates an AlertRuleRepository.
func NewAlertRuleRepository(client *Client) *AlertRuleRepository {
	return &AlertRuleRepository{client: client}
}

// Create inserts an alert rule.
func (r *AlertRuleRepository) Create(ctx context.Context, rule *domain.AlertRule) error {
	filters, err := json.Marshal(rule.Filters)
	if err != nil {
		return fmt.Errorf("marshal alert filters: %w", err)
	}

	err = r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, `
			INSERT INTO alert_rules (
				id, name, description, metric, condition, threshold, evaluation_window, filters,
				severity, is_active, cooldown, notification_channel_ids, created_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid[], $13, $14, $15)
		`,
			rule.ID,
			rule.Name,
			rule.Description,
			rule.Metric,
			rule.Condition,
			rule.Threshold,
			rule.Window,
			filters,
			rule.Severity,
			rule.IsActive,
			rule.Cooldown,
			rule.NotificationChannelIDs,
			nullableString(rule.CreatedBy),
			rule.CreatedAt,
			rule.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("insert alert rule: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create alert rule: %w", err)
	}

	return nil
}

// Update modifies an alert rule.
func (r *AlertRuleRepository) Update(ctx context.Context, rule *domain.AlertRule) error {
	filters, err := json.Marshal(rule.Filters)
	if err != nil {
		return fmt.Errorf("marshal alert filters: %w", err)
	}

	var rows int64

	err = r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `
			UPDATE alert_rules
			SET name = $2,
				description = $3,
				metric = $4,
				condition = $5,
				threshold = $6,
				evaluation_window = $7,
				filters = $8,
				severity = $9,
				is_active = $10,
				cooldown = $11,
				notification_channel_ids = $12::uuid[],
				updated_at = $13
			WHERE id = $1
		`,
			rule.ID,
			rule.Name,
			rule.Description,
			rule.Metric,
			rule.Condition,
			rule.Threshold,
			rule.Window,
			filters,
			rule.Severity,
			rule.IsActive,
			rule.Cooldown,
			rule.NotificationChannelIDs,
			rule.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("update alert rule: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update alert rule: %w", err)
	}

	if rows == 0 {
		return domain.ErrAlertRuleNotFound
	}

	return nil
}

// Disable marks an alert rule inactive.
func (r *AlertRuleRepository) Disable(ctx context.Context, id string) error {
	var rows int64

	err := r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `UPDATE alert_rules SET is_active = false, updated_at = now() WHERE id = $1`, id)
		if execErr != nil {
			return fmt.Errorf("disable alert rule: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("disable alert rule: %w", err)
	}

	if rows == 0 {
		return domain.ErrAlertRuleNotFound
	}

	return nil
}

// GetByID returns an alert rule by ID.
func (r *AlertRuleRepository) GetByID(ctx context.Context, id string) (*domain.AlertRule, error) {
	var rule domain.AlertRule

	err := r.client.Execute(func() error {
		return scanAlertRule(r.client.Pool.QueryRow(ctx, alertRuleSelectSQL()+` WHERE id = $1`, id), &rule)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get alert rule: %w", err)
	}

	return &rule, nil
}

// List returns alert rules.
func (r *AlertRuleRepository) List(ctx context.Context, limit, offset int) ([]domain.AlertRule, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_rules`).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count alert rules: %w", err)
	}

	rows, err := r.client.Pool.Query(ctx, alertRuleSelectSQL()+` ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query alert rules: %w", err)
	}
	defer rows.Close()

	rules, err := scanAlertRules(rows)
	if err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

// ListActive returns active alert rules.
func (r *AlertRuleRepository) ListActive(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := r.client.Pool.Query(ctx, alertRuleSelectSQL()+` WHERE is_active = true ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query active alert rules: %w", err)
	}
	defer rows.Close()

	return scanAlertRules(rows)
}

// AlertEventRepository persists alert events.
type AlertEventRepository struct {
	client *Client
}

// NewAlertEventRepository creates an AlertEventRepository.
func NewAlertEventRepository(client *Client) *AlertEventRepository {
	return &AlertEventRepository{client: client}
}

// Create inserts an alert event.
func (r *AlertEventRepository) Create(ctx context.Context, event *domain.AlertEvent) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("marshal alert details: %w", err)
	}

	err = r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, `
			INSERT INTO alert_events (
				id, alert_rule_id, status, value, started_at, resolved_at, last_notified_at, details
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`,
			event.ID,
			event.AlertRuleID,
			event.Status,
			event.Value,
			event.StartedAt,
			event.ResolvedAt,
			event.LastNotifiedAt,
			details,
		)
		if execErr != nil {
			return fmt.Errorf("insert alert event: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create alert event: %w", err)
	}

	return nil
}

// Update modifies an alert event.
func (r *AlertEventRepository) Update(ctx context.Context, event *domain.AlertEvent) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("marshal alert details: %w", err)
	}

	var rows int64

	err = r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `
			UPDATE alert_events
			SET status = $2,
				value = $3,
				resolved_at = $4,
				last_notified_at = $5,
				details = $6
			WHERE id = $1
		`,
			event.ID,
			event.Status,
			event.Value,
			event.ResolvedAt,
			event.LastNotifiedAt,
			details,
		)
		if execErr != nil {
			return fmt.Errorf("update alert event: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update alert event: %w", err)
	}

	if rows == 0 {
		return domain.ErrAlertEventNotFound
	}

	return nil
}

// GetByID returns an alert event by ID.
func (r *AlertEventRepository) GetByID(ctx context.Context, id string) (*domain.AlertEvent, error) {
	var event domain.AlertEvent

	err := r.client.Execute(func() error {
		return scanAlertEvent(r.client.Pool.QueryRow(ctx, alertEventSelectSQL()+` WHERE id = $1`, id), &event)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get alert event: %w", err)
	}

	return &event, nil
}

// List returns alert events.
func (r *AlertEventRepository) List(ctx context.Context, limit, offset int) ([]domain.AlertEvent, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_events`).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count alert events: %w", err)
	}

	rows, err := r.client.Pool.Query(ctx, alertEventSelectSQL()+` ORDER BY started_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query alert events: %w", err)
	}
	defer rows.Close()

	events, err := scanAlertEvents(rows)
	if err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// ActiveForRule returns the active firing/acknowledged event for a rule.
func (r *AlertEventRepository) ActiveForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error) {
	var event domain.AlertEvent

	err := r.client.Execute(func() error {
		return scanAlertEvent(r.client.Pool.QueryRow(ctx, alertEventSelectSQL()+`
			WHERE alert_rule_id = $1 AND status IN ($2, $3)
			ORDER BY started_at DESC
			LIMIT 1
		`, ruleID, domain.AlertStatusFiring, domain.AlertStatusAcknowledged), &event)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get active alert event: %w", err)
	}

	return &event, nil
}

func alertRuleSelectSQL() string {
	return `
		SELECT id, name, COALESCE(description, ''), metric, condition, threshold, evaluation_window,
		       filters, severity, is_active, cooldown, notification_channel_ids,
		       COALESCE(created_by::text, ''), created_at, updated_at
		FROM alert_rules
	`
}

func scanAlertRules(rows pgx.Rows) ([]domain.AlertRule, error) {
	var rules []domain.AlertRule

	for rows.Next() {
		var rule domain.AlertRule

		err := scanAlertRule(rows, &rule)
		if err != nil {
			return nil, err
		}

		rules = append(rules, rule)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate alert rules: %w", err)
	}

	return rules, nil
}

func scanAlertRule(row scanner, rule *domain.AlertRule) error {
	var filters []byte

	err := row.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Description,
		&rule.Metric,
		&rule.Condition,
		&rule.Threshold,
		&rule.Window,
		&filters,
		&rule.Severity,
		&rule.IsActive,
		&rule.Cooldown,
		&rule.NotificationChannelIDs,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("scan alert rule: %w", err)
	}

	if len(filters) > 0 {
		err = json.Unmarshal(filters, &rule.Filters)
		if err != nil {
			return fmt.Errorf("unmarshal alert filters: %w", err)
		}
	}

	if rule.Filters == nil {
		rule.Filters = domain.ConfigMap{}
	}

	return nil
}

func alertEventSelectSQL() string {
	return `
		SELECT id, alert_rule_id, status, value, started_at, resolved_at, last_notified_at, details
		FROM alert_events
	`
}

func scanAlertEvents(rows pgx.Rows) ([]domain.AlertEvent, error) {
	var events []domain.AlertEvent

	for rows.Next() {
		var event domain.AlertEvent

		err := scanAlertEvent(rows, &event)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate alert events: %w", err)
	}

	return events, nil
}

func scanAlertEvent(row scanner, event *domain.AlertEvent) error {
	var (
		resolvedAt     sql.NullTime
		lastNotifiedAt sql.NullTime
		details        []byte
	)

	err := row.Scan(
		&event.ID,
		&event.AlertRuleID,
		&event.Status,
		&event.Value,
		&event.StartedAt,
		&resolvedAt,
		&lastNotifiedAt,
		&details,
	)
	if err != nil {
		return fmt.Errorf("scan alert event: %w", err)
	}

	if resolvedAt.Valid {
		event.ResolvedAt = &resolvedAt.Time
	}

	if lastNotifiedAt.Valid {
		event.LastNotifiedAt = &lastNotifiedAt.Time
	}

	if len(details) > 0 {
		err = json.Unmarshal(details, &event.Details)
		if err != nil {
			return fmt.Errorf("unmarshal alert details: %w", err)
		}
	}

	if event.Details == nil {
		event.Details = domain.ConfigMap{}
	}

	return nil
}
