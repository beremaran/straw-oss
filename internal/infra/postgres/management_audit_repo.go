package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
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
	whereClause, args := buildEventFilterQuery(filter)

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM management_audit_events %s", whereClause)
	var totalCount int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, occurred_at, actor_type, actor_id, actor_display, action, entity_type, entity_id,
		       old_value, new_value, request_id, trace_id, ip, user_agent
		FROM management_audit_events
		%s
			ORDER BY occurred_at DESC
		`, whereClause)

	query, args = appendPagination(query, args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []*domain.ManagementAuditEvent
	for rows.Next() {
		event, err := scanManagementAuditEvent(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit event: %w", err)
		}
		events = append(events, event)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return events, totalCount, nil
}

func (r *ManagementAuditRepository) ListRequests(ctx context.Context, filter domain.AuditEventFilter, includeBody bool) ([]*domain.ManagementAuditRequest, int, error) {
	whereClause, args := buildRequestFilterQuery(filter)

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM admin_audit_log %s", whereClause)
	var totalCount int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
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

	query, args = appendPagination(query, args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit requests: %w", err)
	}
	defer rows.Close()

	var reqs []*domain.ManagementAuditRequest
	for rows.Next() {
		req, err := scanManagementAuditRequest(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit request: %w", err)
		}
		reqs = append(reqs, req)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return reqs, totalCount, nil
}

func appendPagination(query string, args []interface{}, limit, offset int) (string, []interface{}) {
	argID := len(args) + 1

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, limit)
		argID++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, offset)
	}

	return query, args
}

func scanManagementAuditEvent(rows pgx.Rows) (*domain.ManagementAuditEvent, error) {
	var event domain.ManagementAuditEvent
	var oldValBytes, newValBytes []byte
	err := rows.Scan(
		&event.ID, &event.OccurredAt, &event.ActorType, &event.ActorID, &event.ActorDisplay,
		&event.Action, &event.EntityType, &event.EntityID,
		&oldValBytes, &newValBytes, &event.RequestID, &event.TraceID, &event.IP, &event.UserAgent,
	)
	if err != nil {
		return nil, err
	}

	if len(oldValBytes) > 0 {
		_ = json.Unmarshal(oldValBytes, &event.OldValue)
	}
	if len(newValBytes) > 0 {
		_ = json.Unmarshal(newValBytes, &event.NewValue)
	}

	return &event, nil
}

func scanManagementAuditRequest(rows pgx.Rows) (*domain.ManagementAuditRequest, error) {
	var req domain.ManagementAuditRequest
	err := rows.Scan(
		&req.ID, &req.Timestamp, &req.Method, &req.Path, &req.Query, &req.Body, &req.IP,
		&req.UserAgent, &req.Status, &req.Error, &req.ActorType, &req.ActorID,
		&req.ActorDisplayName, &req.SessionID, &req.RequestID, &req.TraceID,
	)
	if err != nil {
		return nil, err
	}

	return &req, nil
}

func buildRequestFilterQuery(filter domain.AuditEventFilter) (string, []interface{}) {
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
	}

	return whereClause, args
}

func buildEventFilterQuery(filter domain.AuditEventFilter) (string, []interface{}) {
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
	}

	return whereClause, args
}
