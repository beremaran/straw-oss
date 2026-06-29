package dto

import "time"

type CreateFingerprintRequest struct {
	ID string `json:"id" validate:"required"`

	Name string `json:"name" validate:"required"`

	Config map[string]interface{} `json:"config"`
}

type FingerprintResponse struct {
	ID string `json:"id"`

	Name string `json:"name"`

	Config map[string]interface{} `json:"config"`

	CreatedAt time.Time `json:"created_at"`

	UpdatedAt time.Time `json:"updated_at"`
}

type FingerprintDeleteResponse struct {
	ID string `json:"id"`

	Deleted bool `json:"deleted"`

	BroadcastRequested bool `json:"broadcast_requested"`
}

type RoutingRuleReferenceResponse struct {
	ID string `json:"id"`

	Name string `json:"name"`
}

type FingerprintDeleteConflictDetails struct {
	ReferencingRules []RoutingRuleReferenceResponse `json:"referencing_rules"`
}

type FingerprintDeleteConflictResponse struct {
	Error string `json:"error"`

	Code string `json:"code,omitempty"`

	Details FingerprintDeleteConflictDetails `json:"details"`
}

type ListFingerprintsResponse []FingerprintResponse
