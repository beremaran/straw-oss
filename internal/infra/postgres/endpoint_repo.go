package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrEndpointNotFound = errors.New("endpoint not found")
	ErrCommandNotFound  = errors.New("command not found")
)

type PostgresEndpointRepository struct {
	client *Client
}

func NewPostgresEndpointRepository(client *Client) *PostgresEndpointRepository {
	return &PostgresEndpointRepository{client: client}
}

func (r *PostgresEndpointRepository) Create(ctx context.Context, ep *domain.Endpoint) error {
	tagsJSON, err := json.Marshal(ep.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}
	metadataJSON, err := json.Marshal(ep.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO endpoints (
			id, tags, last_heartbeat, is_healthy, metadata,
			desired_state, is_registered, deleted_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	err = r.client.Execute(func() error {
		_, err = r.client.Pool.Exec(ctx, query,
			ep.ID,
			tagsJSON,
			ep.LastHeartbeat,
			ep.IsHealthy,
			metadataJSON,
			string(ep.DesiredState),
			ep.IsRegistered,
			ep.DeletedAt,
			ep.CreatedAt,
			ep.UpdatedAt,
		)

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to create endpoint: %w", err)
	}

	return nil
}

func (r *PostgresEndpointRepository) GetByID(ctx context.Context, id string) (*domain.Endpoint, error) {
	query := `
		SELECT id, tags, last_heartbeat, is_healthy, metadata,
		       desired_state, is_registered, deleted_at, created_at, updated_at
		FROM endpoints
		WHERE id = $1
	`

	var ep domain.Endpoint
	var tagsJSON []byte
	var metadataJSON []byte
	var desiredStateStr string

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&ep.ID,
			&tagsJSON,
			&ep.LastHeartbeat,
			&ep.IsHealthy,
			&metadataJSON,
			&desiredStateStr,
			&ep.IsRegistered,
			&ep.DeletedAt,
			&ep.CreatedAt,
			&ep.UpdatedAt,
		)
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get endpoint by id: %w", err)
	}

	err = json.Unmarshal(tagsJSON, &ep.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	err = json.Unmarshal(metadataJSON, &ep.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	ep.DesiredState = domain.DesiredState(desiredStateStr)

	return &ep, nil
}

func (r *PostgresEndpointRepository) Update(ctx context.Context, ep *domain.Endpoint) error {
	tagsJSON, err := json.Marshal(ep.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}
	metadataJSON, err := json.Marshal(ep.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	ep.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE endpoints
		SET tags = $2,
			last_heartbeat = $3,
			is_healthy = $4,
			metadata = $5,
			desired_state = $6,
			is_registered = $7,
			deleted_at = $8,
			updated_at = $9
		WHERE id = $1
	`

	var rows int64
	err = r.client.Execute(func() error {
		res, err := r.client.Pool.Exec(ctx, query,
			ep.ID,
			tagsJSON,
			ep.LastHeartbeat,
			ep.IsHealthy,
			metadataJSON,
			string(ep.DesiredState),
			ep.IsRegistered,
			ep.DeletedAt,
			ep.UpdatedAt,
		)
		if err != nil {
			return err
		}
		rows = res.RowsAffected()

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to update endpoint: %w", err)
	}
	if rows == 0 {
		return ErrEndpointNotFound
	}

	return nil
}

func (r *PostgresEndpointRepository) Delete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `
		UPDATE endpoints
		SET deleted_at = $2,
			desired_state = $3,
			is_registered = false,
			updated_at = $4
		WHERE id = $1
	`

	var rows int64
	err := r.client.Execute(func() error {
		res, err := r.client.Pool.Exec(ctx, query, id, now, string(domain.DesiredStateDeleted), now)
		if err != nil {
			return err
		}
		rows = res.RowsAffected()

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}
	if rows == 0 {
		return ErrEndpointNotFound
	}

	return nil
}

func (r *PostgresEndpointRepository) List(ctx context.Context, limit, offset int, includeDeleted bool) ([]domain.Endpoint, int, error) {
	var total int
	var countQuery string
	var query string

	if includeDeleted {
		countQuery = `SELECT COUNT(*) FROM endpoints`
		query = `
			SELECT id, tags, last_heartbeat, is_healthy, metadata,
			       desired_state, is_registered, deleted_at, created_at, updated_at
			FROM endpoints
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
	} else {
		countQuery = `SELECT COUNT(*) FROM endpoints WHERE deleted_at IS NULL`
		query = `
			SELECT id, tags, last_heartbeat, is_healthy, metadata,
			       desired_state, is_registered, deleted_at, created_at, updated_at
			FROM endpoints
			WHERE deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
	}

	err := r.client.Pool.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count endpoints: %w", err)
	}

	rows, err := r.client.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query endpoints: %w", err)
	}
	defer rows.Close()

	endpoints, err := scanEndpoints(rows)
	if err != nil {
		return nil, 0, err
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return endpoints, total, nil
}

func scanEndpoints(rows pgx.Rows) ([]domain.Endpoint, error) {
	var endpoints []domain.Endpoint
	for rows.Next() {
		var ep domain.Endpoint
		var tagsJSON []byte
		var metadataJSON []byte
		var desiredStateStr string

		err := rows.Scan(
			&ep.ID,
			&tagsJSON,
			&ep.LastHeartbeat,
			&ep.IsHealthy,
			&metadataJSON,
			&desiredStateStr,
			&ep.IsRegistered,
			&ep.DeletedAt,
			&ep.CreatedAt,
			&ep.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan endpoint: %w", err)
		}

		err = json.Unmarshal(tagsJSON, &ep.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		err = json.Unmarshal(metadataJSON, &ep.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		ep.DesiredState = domain.DesiredState(desiredStateStr)

		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

type PostgresEndpointCommandRepository struct {
	client *Client
}

func NewPostgresEndpointCommandRepository(client *Client) *PostgresEndpointCommandRepository {
	return &PostgresEndpointCommandRepository{client: client}
}

func (r *PostgresEndpointCommandRepository) Create(ctx context.Context, cmd *domain.EndpointCommand) error {
	payloadJSON, err := json.Marshal(cmd.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	query := `
		INSERT INTO endpoint_commands (
			id, endpoint_id, command, status, payload, requested_by,
			requested_at, accepted_at, completed_at, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	err = r.client.Execute(func() error {
		_, err = r.client.Pool.Exec(ctx, query,
			cmd.ID,
			cmd.EndpointID,
			cmd.Command,
			string(cmd.Status),
			payloadJSON,
			cmd.RequestedBy,
			cmd.RequestedAt,
			cmd.AcceptedAt,
			cmd.CompletedAt,
			cmd.Error,
		)

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	return nil
}

func (r *PostgresEndpointCommandRepository) GetByID(ctx context.Context, id string) (*domain.EndpointCommand, error) {
	query := `
		SELECT id, endpoint_id, command, status, payload, requested_by,
		       requested_at, accepted_at, completed_at, error
		FROM endpoint_commands
		WHERE id = $1
	`

	var cmd domain.EndpointCommand
	var statusStr string
	var payloadJSON []byte

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&cmd.ID,
			&cmd.EndpointID,
			&cmd.Command,
			&statusStr,
			&payloadJSON,
			&cmd.RequestedBy,
			&cmd.RequestedAt,
			&cmd.AcceptedAt,
			&cmd.CompletedAt,
			&cmd.Error,
		)
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get command by id: %w", err)
	}

	err = json.Unmarshal(payloadJSON, &cmd.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	cmd.Status = domain.CommandStatus(statusStr)

	return &cmd, nil
}

func (r *PostgresEndpointCommandRepository) Update(ctx context.Context, cmd *domain.EndpointCommand) error {
	payloadJSON, err := json.Marshal(cmd.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	query := `
		UPDATE endpoint_commands
		SET status = $2,
			payload = $3,
			accepted_at = $4,
			completed_at = $5,
			error = $6
		WHERE id = $1
	`

	var rows int64
	err = r.client.Execute(func() error {
		res, err := r.client.Pool.Exec(ctx, query,
			cmd.ID,
			string(cmd.Status),
			payloadJSON,
			cmd.AcceptedAt,
			cmd.CompletedAt,
			cmd.Error,
		)
		if err != nil {
			return err
		}
		rows = res.RowsAffected()

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to update command: %w", err)
	}
	if rows == 0 {
		return ErrCommandNotFound
	}

	return nil
}

func (r *PostgresEndpointCommandRepository) ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]domain.EndpointCommand, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM endpoint_commands WHERE endpoint_id = $1`
	err := r.client.Pool.QueryRow(ctx, countQuery, endpointID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count commands: %w", err)
	}

	query := `
		SELECT id, endpoint_id, command, status, payload, requested_by,
		       requested_at, accepted_at, completed_at, error
		FROM endpoint_commands
		WHERE endpoint_id = $1
		ORDER BY requested_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.client.Pool.Query(ctx, query, endpointID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query commands: %w", err)
	}
	defer rows.Close()

	var commands []domain.EndpointCommand
	for rows.Next() {
		var cmd domain.EndpointCommand
		var statusStr string
		var payloadJSON []byte

		err = rows.Scan(
			&cmd.ID,
			&cmd.EndpointID,
			&cmd.Command,
			&statusStr,
			&payloadJSON,
			&cmd.RequestedBy,
			&cmd.RequestedAt,
			&cmd.AcceptedAt,
			&cmd.CompletedAt,
			&cmd.Error,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan command: %w", err)
		}

		err = json.Unmarshal(payloadJSON, &cmd.Payload)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		cmd.Status = domain.CommandStatus(statusStr)

		commands = append(commands, cmd)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return commands, total, nil
}

func (r *PostgresEndpointCommandRepository) ListPending(ctx context.Context, before time.Time) ([]domain.EndpointCommand, error) {
	query := `
		SELECT id, endpoint_id, command, status, payload, requested_by,
		       requested_at, accepted_at, completed_at, error
		FROM endpoint_commands
		WHERE status IN ('accepted', 'acknowledged', 'running') AND requested_at < $1
	`

	rows, err := r.client.Pool.Query(ctx, query, before)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending commands: %w", err)
	}
	defer rows.Close()

	var commands []domain.EndpointCommand
	for rows.Next() {
		var cmd domain.EndpointCommand
		var statusStr string
		var payloadJSON []byte

		err = rows.Scan(
			&cmd.ID,
			&cmd.EndpointID,
			&cmd.Command,
			&statusStr,
			&payloadJSON,
			&cmd.RequestedBy,
			&cmd.RequestedAt,
			&cmd.AcceptedAt,
			&cmd.CompletedAt,
			&cmd.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan command: %w", err)
		}

		err = json.Unmarshal(payloadJSON, &cmd.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		cmd.Status = domain.CommandStatus(statusStr)

		commands = append(commands, cmd)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return commands, nil
}

type PostgresEndpointLogRepository struct {
	client *Client
}

func NewPostgresEndpointLogRepository(client *Client) *PostgresEndpointLogRepository {
	return &PostgresEndpointLogRepository{client: client}
}

func (r *PostgresEndpointLogRepository) Create(ctx context.Context, entry *domain.EndpointLogEntry) error {
	attrsJSON, err := json.Marshal(entry.Attrs)
	if err != nil {
		return fmt.Errorf("failed to marshal attrs: %w", err)
	}

	query := `
		INSERT INTO endpoint_log_entries (
			endpoint_id, observed_at, level, message, attrs, trace_id, request_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	err = r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query,
			entry.EndpointID,
			entry.ObservedAt,
			entry.Level,
			entry.Message,
			attrsJSON,
			entry.TraceID,
			entry.RequestID,
		).Scan(&entry.ID)
	})

	if err != nil {
		return fmt.Errorf("failed to create log entry: %w", err)
	}

	return nil
}

func (r *PostgresEndpointLogRepository) ListByEndpointID(ctx context.Context, endpointID string, beforeID int64, limit int) ([]domain.EndpointLogEntry, error) {
	var query string
	var rows pgx.Rows
	var err error

	if beforeID > 0 {
		query = `
			SELECT id, endpoint_id, observed_at, level, message, attrs, trace_id, request_id
			FROM endpoint_log_entries
			WHERE endpoint_id = $1 AND id < $2
			ORDER BY observed_at DESC, id DESC
			LIMIT $3
		`
		rows, err = r.client.Pool.Query(ctx, query, endpointID, beforeID, limit)
	} else {
		query = `
			SELECT id, endpoint_id, observed_at, level, message, attrs, trace_id, request_id
			FROM endpoint_log_entries
			WHERE endpoint_id = $1
			ORDER BY observed_at DESC, id DESC
			LIMIT $2
		`
		rows, err = r.client.Pool.Query(ctx, query, endpointID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query log entries: %w", err)
	}
	defer rows.Close()

	entries, err := scanLogEntries(rows)
	if err != nil {
		return nil, err
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

func scanLogEntries(rows pgx.Rows) ([]domain.EndpointLogEntry, error) {
	var entries []domain.EndpointLogEntry
	for rows.Next() {
		var entry domain.EndpointLogEntry
		var attrsJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.EndpointID,
			&entry.ObservedAt,
			&entry.Level,
			&entry.Message,
			&attrsJSON,
			&entry.TraceID,
			&entry.RequestID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log entry: %w", err)
		}

		err = json.Unmarshal(attrsJSON, &entry.Attrs)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal attrs: %w", err)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
