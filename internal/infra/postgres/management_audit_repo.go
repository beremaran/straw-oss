package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ManagementAuditRepository struct {
	db *pgxpool.Pool
}

func NewManagementAuditRepository(db *pgxpool.Pool) *ManagementAuditRepository {
	return &ManagementAuditRepository{db: db}
}

func (r *ManagementAuditRepository) Create(ctx context.Context, event *domain.ManagementAuditEvent) error {
	var oldValBytes, newValBytes []byte
	var err error

	if event.OldValue != nil {
		oldValBytes, err = json.Marshal(event.OldValue)
		if err != nil {
			return fmt.Errorf("failed to marshal old_value: %w", err)
		}
	}

	if event.NewValue != nil {
		newValBytes, err = json.Marshal(event.NewValue)
		if err != nil {
			return fmt.Errorf("failed to marshal new_value: %w", err)
		}
	}

	const query = `
		INSERT INTO management_audit_events (
			actor_type, actor_id, actor_display, action, entity_type, entity_id,
			old_value, new_value, request_id, trace_id, ip, user_agent
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING id, occurred_at
	`

	err = r.db.QueryRow(ctx, query,
		event.ActorType,
		event.ActorID,
		event.ActorDisplay,
		event.Action,
		event.EntityType,
		event.EntityID,
		oldValBytes,
		newValBytes,
		event.RequestID,
		event.TraceID,
		event.IP,
		event.UserAgent,
	).Scan(&event.ID, &event.OccurredAt)

	if err != nil {
		return fmt.Errorf("failed to insert management audit event: %w", err)
	}

	return nil
}
