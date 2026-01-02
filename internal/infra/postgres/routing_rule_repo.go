package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RoutingRuleRepository implements the repository for routing rules using PostgreSQL.
type RoutingRuleRepository struct {
	client *Client
	tracer trace.Tracer
}

// NewRoutingRuleRepository creates a new RoutingRuleRepository.
func NewRoutingRuleRepository(client *Client) *RoutingRuleRepository {
	return &RoutingRuleRepository{
		client: client,
		tracer: otel.Tracer("infra/postgres/routing_rules"),
	}
}

// GetActiveRules retrieves all active routing rules, ordered by priority (highest first).
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

		if err := rows.Scan(
			&id, &name, &priority, &reqTagsJSON, &exclTagsJSON, &configJSON,
			&isActive, &version, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan routing rule row: %w", err)
		}

		// Unmarshal the config JSON first to get the base fields
		var rule domain.RoutingRule
		if err := json.Unmarshal(configJSON, &rule); err != nil {
			return nil, fmt.Errorf("failed to unmarshal routing rule config for ruled %s: %w", id, err)
		}

		// Overwrite with the indexed column values to ensure consistency
		rule.ID = id
		rule.Name = name
		rule.Priority = priority
		rule.IsActive = isActive
		rule.Version = version
		rule.CreatedAt = createdAt
		rule.UpdatedAt = updatedAt

		// Parse tags from the strongly typed JSON columns
		// The columns are stored as JSON strings array ["key:val", "key2:val2"]
		var reqTags []string
		if err := json.Unmarshal(reqTagsJSON, &reqTags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal required tags for rule %s: %w", id, err)
		}
		rule.RequiredTags = reqTags

		var exclTags []string
		if err := json.Unmarshal(exclTagsJSON, &exclTags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal excluded tags for rule %s: %w", id, err)
		}
		rule.ExcludedTags = exclTags

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating routing rules: %w", err)
	}

	return rules, nil
}

// CreateRule creates a new routing rule.
// This is primarily for testing and admin API usage.
func (r *RoutingRuleRepository) CreateRule(ctx context.Context, rule *domain.RoutingRule) error {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "insert"),
		attribute.String("db.table", "routing_rules"),
	))
	defer span.End()

	// Marshal the full rule to config
	configJSON, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule config: %w", err)
	}

	// Marshal tags
	reqTagsJSON, err := json.Marshal(rule.RequiredTags)
	if err != nil {
		return fmt.Errorf("failed to marshal required tags: %w", err)
	}
	// Defaults to empty JSON array if nil
	if rule.RequiredTags == nil {
		reqTagsJSON = []byte("[]")
	}

	exclTagsJSON, err := json.Marshal(rule.ExcludedTags)
	if err != nil {
		return fmt.Errorf("failed to marshal excluded tags: %w", err)
	}
	if rule.ExcludedTags == nil {
		exclTagsJSON = []byte("[]")
	}

	query := `
		INSERT INTO routing_rules (
			id, name, priority, required_tags, excluded_tags, config, 
			is_active, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	// Ensure timestamps are set
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

// GetRuleByID fetches a single rule by ID.
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

	var (
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

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&id, &name, &priority, &reqTagsJSON, &exclTagsJSON, &configJSON,
			&isActive, &version, &createdAt, &updatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get routing rule %s: %w", id, err)
	}

	var rule domain.RoutingRule
	if err := json.Unmarshal(configJSON, &rule); err != nil {
		return nil, fmt.Errorf("failed to unmarshal routing rule config: %w", err)
	}

	rule.ID = id
	rule.Name = name
	rule.Priority = priority
	rule.IsActive = isActive
	rule.Version = version
	rule.CreatedAt = createdAt
	rule.UpdatedAt = updatedAt

	var reqTags []string
	if err := json.Unmarshal(reqTagsJSON, &reqTags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal required tags: %w", err)
	}
	rule.RequiredTags = reqTags

	var exclTags []string
	if err := json.Unmarshal(exclTagsJSON, &exclTags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal excluded tags: %w", err)
	}
	rule.ExcludedTags = exclTags

	return &rule, nil
}

// UpdateRule updates an existing routing rule.
// It also handles optimistic locking via the version field.
func (r *RoutingRuleRepository) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "update"),
		attribute.String("db.table", "routing_rules"),
		attribute.String("rule.id", rule.ID),
	))
	defer span.End()

	// Marshal fields
	configJSON, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule config: %w", err)
	}

	reqTagsJSON, err := json.Marshal(rule.RequiredTags)
	if err != nil {
		return fmt.Errorf("failed to marshal required tags: %w", err)
	}
	if rule.RequiredTags == nil {
		reqTagsJSON = []byte("[]")
	}

	exclTagsJSON, err := json.Marshal(rule.ExcludedTags)
	if err != nil {
		return fmt.Errorf("failed to marshal excluded tags: %w", err)
	}
	if rule.ExcludedTags == nil {
		exclTagsJSON = []byte("[]")
	}

	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = time.Now()
	}

	// Increment version for optimistic locking
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
		return fmt.Errorf("routing rule not found or version mismatch (concurrency conflict)")
	}

	// Update the struct with new version
	rule.Version = newVersion

	return nil
}

// DeleteRule soft deletes a routing rule by setting is_active = false.
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
		return errors.New("routing rule not found")
	}

	return nil
}

// ListRules retrieves a paginated list of all routing rules, regardless of active status.
func (r *RoutingRuleRepository) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	ctx, span := r.tracer.Start(ctx, "db.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "select"),
		attribute.String("db.table", "routing_rules"),
	))
	defer span.End()

	// Get total count
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

		if err := rows.Scan(
			&id, &name, &priority, &reqTagsJSON, &exclTagsJSON, &configJSON,
			&isActive, &version, &createdAt, &updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan routing rule row: %w", err)
		}

		var rule domain.RoutingRule
		if err := json.Unmarshal(configJSON, &rule); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal routing rule config for ruled %s: %w", id, err)
		}

		rule.ID = id
		rule.Name = name
		rule.Priority = priority
		rule.IsActive = isActive
		rule.Version = version
		rule.CreatedAt = createdAt
		rule.UpdatedAt = updatedAt

		// Parse tags just in case
		var reqTags []string
		_ = json.Unmarshal(reqTagsJSON, &reqTags)
		rule.RequiredTags = reqTags

		var exclTags []string
		_ = json.Unmarshal(exclTagsJSON, &exclTags)
		rule.ExcludedTags = exclTags

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating routing rules: %w", err)
	}

	return rules, total, nil
}
