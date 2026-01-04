// Package dto contains Data Transfer Objects for the API layer.
// DTOs separate the API contract from internal domain models.
package dto

// PaginatedResponse wraps paginated list responses
type PaginatedResponse[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

// HeaderDTO represents an HTTP header for API transport
type HeaderDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
