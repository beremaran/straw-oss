package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

var (
	// ErrEndpointNotFound is returned when an endpoint is not found.
	ErrEndpointNotFound = errors.New("endpoint not found")
	// ErrCommandNotFound is returned when a command is not found.
	ErrCommandNotFound = errors.New("command not found")
)

// EndpointRepository persists and retrieves endpoints.
type EndpointRepository struct {
	client *Client
}

// NewEndpointRepository creates a new EndpointRepository backed by the given client.
func NewEndpointRepository(client *Client) *EndpointRepository {
	return &EndpointRepository{client: client}
}

// Create inserts a new endpoint.
func (r *EndpointRepository) Create(ctx context.Context, ep *domain.Endpoint) error {
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
		if err != nil {
			return fmt.Errorf("failed to insert endpoint: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create endpoint: %w", err)
	}

	return nil
}

// GetByID returns the endpoint with the given ID.
func (r *EndpointRepository) GetByID(ctx context.Context, id string) (*domain.Endpoint, error) {
	query := `
		SELECT id, tags, last_heartbeat, is_healthy, metadata,
		       desired_state, is_registered, deleted_at, created_at, updated_at
		FROM endpoints
		WHERE id = $1
	`

	var (
		ep              domain.Endpoint
		tagsJSON        []byte
		metadataJSON    []byte
		desiredStateStr string
	)

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

// Update modifies an existing endpoint.
func (r *EndpointRepository) Update(ctx context.Context, ep *domain.Endpoint) error {
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
			return fmt.Errorf("failed to execute update: %w", err)
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

// Delete soft-deletes an endpoint.
func (r *EndpointRepository) Delete(ctx context.Context, id string) error {
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
			return fmt.Errorf("failed to execute delete: %w", err)
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

// List returns a paginated list of endpoints.
func (r *EndpointRepository) List(ctx context.Context, limit, offset int, includeDeleted bool) ([]domain.Endpoint, int, error) {
	var (
		total      int
		countQuery string
		query      string
	)

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
		var (
			ep              domain.Endpoint
			tagsJSON        []byte
			metadataJSON    []byte
			desiredStateStr string
		)

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

// EndpointCommandRepository persists and retrieves endpoint commands.
type EndpointCommandRepository struct {
	client *Client
}

// NewEndpointCommandRepository creates a new EndpointCommandRepository backed by the given client.
func NewEndpointCommandRepository(client *Client) *EndpointCommandRepository {
	return &EndpointCommandRepository{client: client}
}

// Create inserts a new endpoint command.
func (r *EndpointCommandRepository) Create(ctx context.Context, cmd *domain.EndpointCommand) error {
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
		if err != nil {
			return fmt.Errorf("failed to insert command: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	return nil
}

// GetByID returns the endpoint command with the given ID.
func (r *EndpointCommandRepository) GetByID(ctx context.Context, id string) (*domain.EndpointCommand, error) {
	query := `
		SELECT id, endpoint_id, command, status, payload, requested_by,
		       requested_at, accepted_at, completed_at, error
		FROM endpoint_commands
		WHERE id = $1
	`

	var (
		cmd         domain.EndpointCommand
		statusStr   string
		payloadJSON []byte
	)

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

// Update modifies an existing endpoint command.
func (r *EndpointCommandRepository) Update(ctx context.Context, cmd *domain.EndpointCommand) error {
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
			return fmt.Errorf("failed to execute update: %w", err)
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

// ListByEndpointID returns a paginated list of commands for the given endpoint.
func (r *EndpointCommandRepository) ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]domain.EndpointCommand, int, error) {
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

	commands, err := scanEndpointCommands(rows)
	if err != nil {
		return nil, 0, err
	}

	return commands, total, nil
}

func scanEndpointCommands(rows pgx.Rows) ([]domain.EndpointCommand, error) {
	var commands []domain.EndpointCommand

	var err error

	for rows.Next() {
		var (
			cmd         domain.EndpointCommand
			statusStr   string
			payloadJSON []byte
		)

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

// ListPending returns commands that are accepted, acknowledged, or running and were requested before the given time.
func (r *EndpointCommandRepository) ListPending(ctx context.Context, before time.Time) ([]domain.EndpointCommand, error) {
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
		var (
			cmd         domain.EndpointCommand
			statusStr   string
			payloadJSON []byte
		)

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

// EndpointLogRepository persists and retrieves endpoint log entries.
type EndpointLogRepository struct {
	client *Client
}

// NewEndpointLogRepository creates a new EndpointLogRepository backed by the given client.
func NewEndpointLogRepository(client *Client) *EndpointLogRepository {
	return &EndpointLogRepository{client: client}
}

// Create inserts a new log entry.
func (r *EndpointLogRepository) Create(ctx context.Context, entry *domain.EndpointLogEntry) error {
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

// ListByEndpointID returns log entries for the given endpoint, optionally before a given ID.
func (r *EndpointLogRepository) ListByEndpointID(ctx context.Context, endpointID string, beforeID int64, limit int) ([]domain.EndpointLogEntry, error) {
	var (
		query string
		rows  pgx.Rows
		err   error
	)

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
		var (
			entry     domain.EndpointLogEntry
			attrsJSON []byte
		)

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

// Query returns log entries for the given endpoint filtered by the provided LogFilter.
func (r *EndpointLogRepository) Query(ctx context.Context, endpointID string, f domain.LogFilter) ([]domain.EndpointLogEntry, error) {
	query, args := buildLogQuery(endpointID, f)

	rows, err := r.client.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
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

func buildLogQuery(endpointID string, f domain.LogFilter) (string, []any) {
	query := `
		SELECT id, endpoint_id, observed_at, level, message, attrs, trace_id, request_id
		FROM endpoint_log_entries
		WHERE endpoint_id = $1
	`
	args := []any{endpointID}
	placeholderIndex := 2
	params := []struct {
		cond   string
		val    any
		active bool
	}{
		{"observed_at >= $%d", timeOrZero(f.Start), f.Start != nil},
		{"observed_at <= $%d", timeOrZero(f.End), f.End != nil},
		{"level = $%d", f.Level, f.Level != ""},
		{"message ILIKE $%d", "%" + f.Q + "%", f.Q != ""},
		{"trace_id = $%d", f.TraceID, f.TraceID != ""},
		{"request_id = $%d", f.RequestID, f.RequestID != ""},
		{"id < $%d", f.Cursor, f.Cursor > 0},
	}

	var sb strings.Builder

	for _, p := range params {
		if p.active {
			sb.WriteString(" AND " + fmt.Sprintf(p.cond, placeholderIndex))
			args = append(args, p.val)
			placeholderIndex++
		}
	}

	query += sb.String()
	query += fmt.Sprintf(" ORDER BY observed_at DESC, id DESC LIMIT $%d", placeholderIndex)

	args = append(args, sanitizeLimit(f.Limit))

	return query, args
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}

const maxLogQueryLimit = 500

func sanitizeLimit(l int) int {
	if l <= 0 || l > maxLogQueryLimit {
		return maxLogQueryLimit
	}

	return l
}

// Cleanup removes log entries older than maxAge and trims the table to within maxSizeBytes.
func (r *EndpointLogRepository) Cleanup(ctx context.Context, maxAge time.Duration, maxSizeBytes int64) error {
	cutoff := time.Now().UTC().Add(-maxAge)
	deleteAgeQuery := `DELETE FROM endpoint_log_entries WHERE observed_at < $1`

	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, deleteAgeQuery, cutoff)
		if err != nil {
			return fmt.Errorf("failed to delete by age: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete log entries by age: %w", err)
	}

	return trimLogTableBySize(ctx, r.client, maxSizeBytes)
}

func trimLogTableBySize(ctx context.Context, c *Client, maxSizeBytes int64) error {
	for {
		var size int64

		sizeQuery := `SELECT COALESCE(pg_total_relation_size('endpoint_log_entries'), 0)`

		err := c.Execute(func() error {
			return c.Pool.QueryRow(ctx, sizeQuery).Scan(&size)
		})
		if err != nil {
			return fmt.Errorf("failed to check log table size: %w", err)
		}

		if size <= maxSizeBytes {
			break
		}

		deleteSizeQuery := `
			DELETE FROM endpoint_log_entries
			WHERE id IN (
				SELECT id FROM endpoint_log_entries
				ORDER BY observed_at ASC, id ASC
				LIMIT 10000
			)
		`

		var chunkDeleted int64

		err = c.Execute(func() error {
			res, err := c.Pool.Exec(ctx, deleteSizeQuery)
			if err != nil {
				return fmt.Errorf("failed to delete chunk: %w", err)
			}

			chunkDeleted = res.RowsAffected()

			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to delete log entries chunk by size: %w", err)
		}

		if chunkDeleted == 0 {
			break
		}
	}

	return nil
}
