package dto

// EndpointMetadataDTO contains metadata about an endpoint.
type EndpointMetadataDTO struct {
	Version        string `json:"version,omitempty"`
	IP             string `json:"ip,omitempty"`
	ActiveTasks    int    `json:"active_tasks"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// CreateEndpointRequest is the request body for creating an endpoint.
type CreateEndpointRequest struct {
	ID           string               `json:"id"`
	Tags         []string             `json:"tags,omitempty"`
	Metadata     *EndpointMetadataDTO `json:"metadata,omitempty"`
	DesiredState string               `json:"desired_state,omitempty"`
}

// PatchEndpointRequest is the request body for patching an endpoint.
type PatchEndpointRequest struct {
	Tags         *[]string            `json:"tags,omitempty"`
	Metadata     *EndpointMetadataDTO `json:"metadata,omitempty"`
	DesiredState *string              `json:"desired_state,omitempty"`
	IsRegistered *bool                `json:"is_registered,omitempty"`
}

// EndpointResponse represents an endpoint in API responses.
type EndpointResponse struct {
	ID             string               `json:"id"`
	Tags           []string             `json:"tags"`
	LastHeartbeat  string               `json:"last_heartbeat,omitempty"`
	IsHealthy      bool                 `json:"is_healthy"`
	Metadata       EndpointMetadataDTO  `json:"metadata"`
	DesiredState   string               `json:"desired_state"`
	IsRegistered   bool                 `json:"is_registered"`
	DeletedAt      *string              `json:"deleted_at,omitempty"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
	Health         *EndpointHealthDTO   `json:"health,omitempty"`
	RecentCommands []EndpointCommandDTO `json:"recent_commands,omitempty"`
}

// EndpointHealthDTO contains health information for an endpoint.
type EndpointHealthDTO struct {
	EndpointID  string   `json:"endpoint_id"`
	State       string   `json:"state"`
	Tags        []string `json:"tags,omitempty"`
	Version     string   `json:"version,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
	LastSeen    string   `json:"last_seen"`
}

// EndpointCommandDTO represents a command sent to an endpoint.
type EndpointCommandDTO struct {
	ID          string         `json:"id"`
	EndpointID  string         `json:"endpoint_id"`
	Command     string         `json:"command"`
	Status      string         `json:"status"`
	Payload     map[string]any `json:"payload"`
	RequestedBy *string        `json:"requested_by,omitempty"`
	RequestedAt string         `json:"requested_at"`
	AcceptedAt  *string        `json:"accepted_at,omitempty"`
	CompletedAt *string        `json:"completed_at,omitempty"`
	Error       *string        `json:"error,omitempty"`
}

// EndpointDrainResponse is the response for draining an endpoint.
type EndpointDrainResponse struct {
	EndpointID   string `json:"endpoint_id"`
	DesiredState string `json:"desired_state"`
	CommandID    string `json:"command_id,omitempty"`
}

// EndpointCommandListResponse is a paginated list of endpoint commands.
type EndpointCommandListResponse struct {
	Data  []EndpointCommandDTO `json:"data"`
	Total int                  `json:"total"`
	Page  int                  `json:"page"`
	Limit int                  `json:"limit"`
}

// EndpointListResponse is a paginated list of endpoints.
type EndpointListResponse struct {
	Data  []EndpointResponse `json:"data"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

// EndpointLogDTO represents a log entry from an endpoint.
type EndpointLogDTO struct {
	ID         int64          `json:"id"`
	EndpointID string         `json:"endpoint_id"`
	ObservedAt string         `json:"observed_at"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attrs      map[string]any `json:"attrs"`
	TraceID    *string        `json:"trace_id,omitempty"`
	RequestID  *string        `json:"request_id,omitempty"`
}

// EndpointLogListResponse is a paginated list of endpoint logs.
type EndpointLogListResponse struct {
	Data       []EndpointLogDTO `json:"data"`
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
}
