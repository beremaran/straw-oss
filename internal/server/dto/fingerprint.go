package dto

import "time"

// CreateFingerprintRequest is the request to create/update a fingerprint preset.
//
//	@Description Request body for creating or updating a fingerprint preset
type CreateFingerprintRequest struct {
	// ID is the unique identifier for this preset (required)
	ID string `json:"id" validate:"required"`

	// Name is a human-readable name for the preset (required)
	Name string `json:"name" validate:"required"`

	// Config is the detailed configuration of the fingerprint
	Config map[string]interface{} `json:"config"`
}

// FingerprintResponse is the API response for a fingerprint preset.
//
//	@Description Fingerprint preset data returned by the API
type FingerprintResponse struct {
	// ID is the unique identifier for this preset
	ID string `json:"id"`

	// Name is a human-readable name for the preset
	Name string `json:"name"`

	// Config is the detailed configuration of the fingerprint
	Config map[string]interface{} `json:"config"`

	// CreatedAt is when the preset was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the preset was last modified
	UpdatedAt time.Time `json:"updated_at"`
}

// ListFingerprintsResponse is the list of fingerprint presets
type ListFingerprintsResponse []FingerprintResponse
