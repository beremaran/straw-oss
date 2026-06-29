package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var ErrRoutingRuleNotFound = errors.New("routing rule not found")

type RoutingRuleRepository struct {
	client *Client
	tracer trace.Tracer
}

func NewRoutingRuleRepository(client *Client) *RoutingRuleRepository {
	return &RoutingRuleRepository{
		client: client,
		tracer: otel.Tracer("infra/postgres/routing_rules"),
	}
}

func scanRoutingRule(scan func(dest ...interface{}) error, strictTags bool) (domain.RoutingRule, error) {
	var (
		id           string
		name         string
		priority     int
		reqTagsJSON  []byte
		exclTagsJSON []byte
		configJSON   []byte
		isActive     bool
		version      int
		createdAt    time.Time
		updatedAt    time.Time
	)

	err := scan(
		&id, &name, &priority, &reqTagsJSON, &exclTagsJSON, &configJSON,
		&isActive, &version, &createdAt, &updatedAt,
	)
	if err != nil {
		return domain.RoutingRule{}, err
	}

	var rule domain.RoutingRule
	err = json.Unmarshal(configJSON, &rule)
	if err != nil {
		return domain.RoutingRule{}, fmt.Errorf("failed to unmarshal routing rule config for rule %s: %w", id, err)
	}

	rule.ID = id
	rule.Name = name
	rule.Priority = priority
	rule.IsActive = isActive
	rule.Version = version
	rule.CreatedAt = createdAt
	rule.UpdatedAt = updatedAt

	err = applyRoutingRuleTags(&rule, reqTagsJSON, exclTagsJSON, strictTags)
	if err != nil {
		return domain.RoutingRule{}, err
	}

	return rule, nil
}

func applyRoutingRuleTags(rule *domain.RoutingRule, reqTagsJSON, exclTagsJSON []byte, strict bool) error {
	var reqTags []string
	err := json.Unmarshal(reqTagsJSON, &reqTags)
	if err != nil && strict {
		return fmt.Errorf("failed to unmarshal required tags for rule %s: %w", rule.ID, err)
	}
	rule.RequiredTags = reqTags

	var exclTags []string
	err = json.Unmarshal(exclTagsJSON, &exclTags)
	if err != nil && strict {
		return fmt.Errorf("failed to unmarshal excluded tags for rule %s: %w", rule.ID, err)
	}
	rule.ExcludedTags = exclTags

	return nil
}

func (r *RoutingRuleRepository) GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error) {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "select"),
		attribute.String("db.table", "routing_rules"),
	))
	defer span.End()

	query := `
		SELECT 
			id, name, priority, required_tags, excluded_tags, config, 
			is_active, version, created_at, updated_at
		FROM routing_rules
		WHERE is_active = true
		ORDER BY priority DESC, created_at DESC
	`

	var rows pgx.Rows
	var err error
	err = r.client.Execute(func() error {
		rows, err = r.client.Pool.Query(ctx, query)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query active routing rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.RoutingRule

	for rows.Next() {
		rule, err := scanRoutingRule(rows.Scan, true)
		if err != nil {
			return nil, fmt.Errorf("failed to scan routing rule row: %w", err)
		}

		rules = append(rules, rule)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating routing rules: %w", err)
	}

	return rules, nil
}

func (r *RoutingRuleRepository) ListActiveRulesReferencingFingerprintPreset(ctx context.Context, presetID string) ([]domain.RoutingRuleReference, error) {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "select"),
		attribute.String("db.table", "routing_rules"),
		attribute.String("fingerprint_preset", presetID),
	))
	defer span.End()

	query := `
		SELECT id, name
		FROM routing_rules
		WHERE is_active = true
			AND (
				config->>'fingerprint_preset' = $1
				OR EXISTS (
					SELECT 1
					FROM jsonb_array_elements(
						CASE
							WHEN jsonb_typeof(config->'fingerprint_ab_test'->'variants') = 'array'
							THEN config->'fingerprint_ab_test'->'variants'
							ELSE '[]'::jsonb
						END
					) AS variant
					WHERE variant->>'fingerprint' = $1
				)
			)
		ORDER BY priority DESC, created_at DESC
	`

	var rows pgx.Rows
	var err error
	err = r.client.Execute(func() error {
		rows, err = r.client.Pool.Query(ctx, query, presetID)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query referencing routing rules: %w", err)
	}
	defer rows.Close()

	var refs []domain.RoutingRuleReference
	for rows.Next() {
		var ref domain.RoutingRuleReference
		err := rows.Scan(&ref.ID, &ref.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan referencing routing rule: %w", err)
		}

		refs = append(refs, ref)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating referencing routing rules: %w", err)
	}

	return refs, nil
}

func (r *RoutingRuleRepository) CreateRule(ctx context.Context, rule *domain.RoutingRule) error {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "insert"),
		attribute.String("db.table", "routing_rules"),
	))
	defer span.End()

	configJSON, reqTagsJSON, exclTagsJSON, err := routingRuleJSON(rule)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO routing_rules (
			id, name, priority, required_tags, excluded_tags, config, 
			is_active, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = time.Now()
	}
	if rule.Version == 0 {
		rule.Version = 1
	}

	err = r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			rule.ID, rule.Name, rule.Priority, reqTagsJSON, exclTagsJSON, configJSON,
			rule.IsActive, rule.Version, rule.CreatedAt, rule.UpdatedAt,
		)

		return err
	})

	return err
}

func routingRuleJSON(rule *domain.RoutingRule) ([]byte, []byte, []byte, error) {
	configJSON, err := json.Marshal(rule)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal rule config: %w", err)
	}

	reqTagsJSON, err := tagListJSON(rule.RequiredTags, "required tags")
	if err != nil {
		return nil, nil, nil, err
	}

	exclTagsJSON, err := tagListJSON(rule.ExcludedTags, "excluded tags")
	if err != nil {
		return nil, nil, nil, err
	}

	return configJSON, reqTagsJSON, exclTagsJSON, nil
}

func tagListJSON(tags []string, name string) ([]byte, error) {
	data, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s: %w", name, err)
	}
	if tags == nil {
		return []byte("[]"), nil
	}

	return data, nil
}

func (r *RoutingRuleRepository) GetRuleByID(ctx context.Context, id string) (*domain.RoutingRule, error) {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "select"),
		attribute.String("db.table", "routing_rules"),
		attribute.String("rule.id", id),
	))
	defer span.End()

	query := `
		SELECT 
			id, name, priority, required_tags, excluded_tags, config, 
			is_active, version, created_at, updated_at
		FROM routing_rules
		WHERE id = $1
	`

	var rule domain.RoutingRule
	err := r.client.Execute(func() error {
		var scanErr error
		rule, scanErr = scanRoutingRule(r.client.Pool.QueryRow(ctx, query, id).Scan, true)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get routing rule %s: %w", id, err)
	}

	return &rule, nil
}

func (r *RoutingRuleRepository) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "update"),
		attribute.String("db.table", "routing_rules"),
		attribute.String("rule.id", rule.ID),
	))
	defer span.End()

	configJSON, reqTagsJSON, exclTagsJSON, err := routingRuleJSON(rule)
	if err != nil {
		return err
	}

	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = time.Now()
	}

	newVersion := rule.Version + 1

	query := `
		UPDATE routing_rules
		SET name = $1, 
			priority = $2, 
			required_tags = $3, 
			excluded_tags = $4, 
			config = $5, 
			is_active = $6, 
			version = $7, 
			updated_at = $8
		WHERE id = $9 AND version = $10
	`

	var res pgconn.CommandTag
	err = r.client.Execute(func() error {
		res, err = r.client.Pool.Exec(ctx, query,
			rule.Name, rule.Priority, reqTagsJSON, exclTagsJSON, configJSON,
			rule.IsActive, newVersion, rule.UpdatedAt,
			rule.ID, rule.Version,
		)

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to update routing rule: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrRoutingRuleNotFound)
	}

	rule.Version = newVersion

	return nil
}

func (r *RoutingRuleRepository) DeleteRule(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "update"),
		attribute.String("db.table", "routing_rules"),
		attribute.String("rule.id", id),
	))
	defer span.End()

	query := `
		UPDATE routing_rules
		SET is_active = false
		WHERE id = $1
	`
	var res pgconn.CommandTag
	err := r.client.Execute(func() error {
		var err error
		res, err = r.client.Pool.Exec(ctx, query, id)

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to delete routing rule: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrRoutingRuleNotFound
	}

	return nil
}

func (r *RoutingRuleRepository) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "select"),
		attribute.String("db.table", "routing_rules"),
	))
	defer span.End()

	var total int
	countQuery := `SELECT COUNT(*) FROM routing_rules`
	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, countQuery).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count routing rules: %w", err)
	}

	query := `
		SELECT 
			id, name, priority, required_tags, excluded_tags, config, 
			is_active, version, created_at, updated_at
		FROM routing_rules
		ORDER BY priority DESC, created_at DESC
		LIMIT $1 OFFSET $2
	`

	var rows pgx.Rows
	err = r.client.Execute(func() error {
		rows, err = r.client.Pool.Query(ctx, query, limit, offset)

		return err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list routing rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.RoutingRule

	for rows.Next() {
		rule, err := scanRoutingRule(rows.Scan, false)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan routing rule row: %w", err)
		}

		rules = append(rules, rule)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("error iterating routing rules: %w", err)
	}

	return rules, total, nil
}
