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

func (r *ManagementAuditRepository) GetEventByID(ctx context.Context, id int64) (*domain.ManagementAuditEvent, error) {
	const query = `
		SELECT id, occurred_at, actor_type, actor_id, actor_display, action, entity_type, entity_id,
		       old_value, new_value, request_id, trace_id, ip, user_agent
		FROM management_audit_events
		WHERE id = $1
	`
	var event domain.ManagementAuditEvent
	var oldValBytes, newValBytes []byte
	err := r.db.QueryRow(ctx, query, id).Scan(
		&event.ID, &event.OccurredAt, &event.ActorType, &event.ActorID, &event.ActorDisplay,
		&event.Action, &event.EntityType, &event.EntityID,
		&oldValBytes, &newValBytes, &event.RequestID, &event.TraceID, &event.IP, &event.UserAgent,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit event by id: %w", err)
	}

	if len(oldValBytes) > 0 {
		_ = json.Unmarshal(oldValBytes, &event.OldValue)
	}
	if len(newValBytes) > 0 {
		_ = json.Unmarshal(newValBytes, &event.NewValue)
	}

	return &event, nil
}

func (r *ManagementAuditRepository) ListEvents(ctx context.Context, filter domain.AuditEventFilter) ([]*domain.ManagementAuditEvent, int, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argID := 1

	if filter.StartDate != nil {
		whereClause += fmt.Sprintf(" AND occurred_at >= $%d", argID)
		args = append(args, *filter.StartDate)
		argID++
	}
	if filter.EndDate != nil {
		whereClause += fmt.Sprintf(" AND occurred_at <= $%d", argID)
		args = append(args, *filter.EndDate)
		argID++
	}
	if filter.ActorID != nil {
		whereClause += fmt.Sprintf(" AND actor_id = $%d", argID)
		args = append(args, *filter.ActorID)
		argID++
	}
	if filter.Action != nil {
		whereClause += fmt.Sprintf(" AND action = $%d", argID)
		args = append(args, *filter.Action)
		argID++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM management_audit_events %s", whereClause)
	var totalCount int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, occurred_at, actor_type, actor_id, actor_display, action, entity_type, entity_id,
		       old_value, new_value, request_id, trace_id, ip, user_agent
		FROM management_audit_events
		%s
		ORDER BY occurred_at DESC
	`, whereClause)

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, filter.Limit)
		argID++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, filter.Offset)
		argID++
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []*domain.ManagementAuditEvent
	for rows.Next() {
		var event domain.ManagementAuditEvent
		var oldValBytes, newValBytes []byte
		if err := rows.Scan(
			&event.ID, &event.OccurredAt, &event.ActorType, &event.ActorID, &event.ActorDisplay,
			&event.Action, &event.EntityType, &event.EntityID,
			&oldValBytes, &newValBytes, &event.RequestID, &event.TraceID, &event.IP, &event.UserAgent,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit event: %w", err)
		}
		if len(oldValBytes) > 0 {
			_ = json.Unmarshal(oldValBytes, &event.OldValue)
		}
		if len(newValBytes) > 0 {
			_ = json.Unmarshal(newValBytes, &event.NewValue)
		}
		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return events, totalCount, nil
}

func (r *ManagementAuditRepository) ListRequests(ctx context.Context, filter domain.AuditEventFilter, includeBody bool) ([]*domain.ManagementAuditRequest, int, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argID := 1

	if filter.StartDate != nil {
		whereClause += fmt.Sprintf(" AND timestamp >= $%d", argID)
		args = append(args, *filter.StartDate)
		argID++
	}
	if filter.EndDate != nil {
		whereClause += fmt.Sprintf(" AND timestamp <= $%d", argID)
		args = append(args, *filter.EndDate)
		argID++
	}
	if filter.ActorID != nil {
		whereClause += fmt.Sprintf(" AND actor_id = $%d", argID)
		args = append(args, *filter.ActorID)
		argID++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM admin_audit_log %s", whereClause)
	var totalCount int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to get total request count: %w", err)
	}

	bodyCol := "'' as body"
	if includeBody {
		bodyCol = "body"
	}

	query := fmt.Sprintf(`
		SELECT id, timestamp, method, path, query, %s, ip, user_agent, status, error,
		       COALESCE(actor_type, ''), COALESCE(actor_id, ''), COALESCE(actor_display_name, ''),
		       COALESCE(session_id, ''), COALESCE(request_id, ''), COALESCE(trace_id, '')
		FROM admin_audit_log
		%s
		ORDER BY timestamp DESC
	`, bodyCol, whereClause)

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, filter.Limit)
		argID++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, filter.Offset)
		argID++
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit requests: %w", err)
	}
	defer rows.Close()

	var reqs []*domain.ManagementAuditRequest
	for rows.Next() {
		var req domain.ManagementAuditRequest
		if err := rows.Scan(
			&req.ID, &req.Timestamp, &req.Method, &req.Path, &req.Query, &req.Body, &req.IP,
			&req.UserAgent, &req.Status, &req.Error, &req.ActorType, &req.ActorID,
			&req.ActorDisplayName, &req.SessionID, &req.RequestID, &req.TraceID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit request: %w", err)
		}
		reqs = append(reqs, &req)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return reqs, totalCount, nil
}
