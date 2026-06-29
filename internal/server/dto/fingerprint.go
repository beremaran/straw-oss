package dto

import "time"

// CreateFingerprintRequest is the request body for creating a fingerprint preset.
type CreateFingerprintRequest struct {
	ID     string         `json:"id"     validate:"required"`
	Name   string         `json:"name"   validate:"required"`
	Config map[string]any `json:"config"`
}

// FingerprintResponse represents a fingerprint preset in API responses.
type FingerprintResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FingerprintDeleteResponse is the response body for deleting a fingerprint preset.
type FingerprintDeleteResponse struct {
	ID                 string `json:"id"`
	Deleted            bool   `json:"deleted"`
	BroadcastRequested bool   `json:"broadcast_requested"`
}

// RoutingRuleReferenceResponse represents a routing rule that references a fingerprint.
type RoutingRuleReferenceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FingerprintDeleteConflictDetails contains information about conflicts when deleting a fingerprint.
type FingerprintDeleteConflictDetails struct {
	ReferencingRules []RoutingRuleReferenceResponse `json:"referencing_rules"`
}

// FingerprintDeleteConflictResponse is returned when a fingerprint has referencing rules.
type FingerprintDeleteConflictResponse struct {
	Error   string                           `json:"error"`
	Code    string                           `json:"code,omitempty"`
	Details FingerprintDeleteConflictDetails `json:"details"`
}

// ListFingerprintsResponse is a list of fingerprint presets.
type ListFingerprintsResponse []FingerprintResponse
