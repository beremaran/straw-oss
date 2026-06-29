package dto

type EndpointMetadataDTO struct {
	Version        string `json:"version,omitempty"`
	IP             string `json:"ip,omitempty"`
	ActiveTasks    int    `json:"active_tasks"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

type CreateEndpointRequest struct {
	ID           string               `json:"id"`
	Tags         []string             `json:"tags,omitempty"`
	Metadata     *EndpointMetadataDTO `json:"metadata,omitempty"`
	DesiredState string               `json:"desired_state,omitempty"`
}

type PatchEndpointRequest struct {
	Tags         *[]string            `json:"tags,omitempty"`
	Metadata     *EndpointMetadataDTO `json:"metadata,omitempty"`
	DesiredState *string              `json:"desired_state,omitempty"`
	IsRegistered *bool                `json:"is_registered,omitempty"`
}

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

type EndpointHealthDTO struct {
	EndpointID  string   `json:"endpoint_id"`
	State       string   `json:"state"`
	Tags        []string `json:"tags,omitempty"`
	Version     string   `json:"version,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
	LastSeen    string   `json:"last_seen"`
}

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

type EndpointDrainResponse struct {
	EndpointID   string `json:"endpoint_id"`
	DesiredState string `json:"desired_state"`
	CommandID    string `json:"command_id,omitempty"`
}

type EndpointCommandListResponse struct {
	Data  []EndpointCommandDTO `json:"data"`
	Total int                  `json:"total"`
	Page  int                  `json:"page"`
	Limit int                  `json:"limit"`
}

type EndpointListResponse struct {
	Data  []EndpointResponse `json:"data"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}
