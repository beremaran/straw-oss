package dto

import "time"

// CreateCostMultiplierRequest creates a cost multiplier.
type CreateCostMultiplierRequest struct {
	EndpointTag string  `json:"endpoint_tag"`
	Multiplier  float64 `json:"multiplier"`
	Description string  `json:"description,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// UpdateCostMultiplierRequest updates a cost multiplier.
type UpdateCostMultiplierRequest struct {
	EndpointTag string  `json:"endpoint_tag"`
	Multiplier  float64 `json:"multiplier"`
	Description string  `json:"description,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	Version     int     `json:"version"`
}

// CostMultiplierResponse represents a cost multiplier.
type CostMultiplierResponse struct {
	ID          string    `json:"id"`
	EndpointTag string    `json:"endpoint_tag"`
	Multiplier  float64   `json:"multiplier"`
	Description string    `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListCostMultipliersResponse is a paginated list of cost multipliers.
type ListCostMultipliersResponse = PaginatedResponse[CostMultiplierResponse]
